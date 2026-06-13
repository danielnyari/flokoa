/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/test/utils"
)

const (
	// Fixture artifact tags produced by `make build-e2e-capability-artifacts`
	// (test/e2e/fixtures/capabilities/build.sh).
	echoArtifactImage  = "flokoa-cap-echo:test"
	upperArtifactImage = "flokoa-cap-upper:test"

	// tamperedArtifactImage is built in this suite FROM echoArtifactImage
	// with the wheel bytes corrupted after manifest assembly — the exact
	// supply-chain window the runner's sha256 verification closes.
	tamperedArtifactImage = "flokoa-cap-echo-tampered:test"

	// capabilityDeliveryAnnotation mirrors
	// builder.CapabilityDeliveryAnnotation (e2e asserts the product surface,
	// not the internal package).
	capabilityDeliveryAnnotation = "flokoa.ai/capability-delivery"

	// deliveryStateConfigMapName mirrors delivery.StateConfigMapName.
	deliveryStateConfigMapName = "flokoa-capability-delivery"

	// capabilitiesMountPath mirrors builder.CapabilitiesMountPath /
	// DEFAULT_CAPABILITIES_ROOT in flokoa_runner/capabilities.py.
	capabilitiesMountPath = "/opt/flokoa/capabilities"
)

// expectedDeliveryMode is how this run expects capability artifacts to reach
// runner pods. The default CI job runs the operator's default (initContainer);
// the advisory ImageVolume job sets CAPABILITY_DELIVERY_EXPECT=imageVolume on
// a feature-gated cluster, and this suite then switches the operator to
// `auto` and asserts the probe lands on imageVolume — one test file serves
// both jobs.
func expectedDeliveryMode() string {
	if v := os.Getenv("CAPABILITY_DELIVERY_EXPECT"); v != "" {
		return v
	}
	return "initContainer"
}

var _ = Describe("Capability delivery", Ordered, func() {
	SetDefaultEventuallyTimeout(3 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	var (
		echoRef     string
		upperRef    string
		tamperedRef string
	)

	// capabilityManifests are the static manifests this context applies,
	// deleted in reverse order by AfterAll.
	capabilityManifests := []string{
		"test/e2e/testdata/modelprovider.yaml",
		"test/e2e/testdata/model.yaml",
		"test/e2e/testdata/capability-instruction.yaml",
		"test/e2e/testdata/capability-agent.yaml",
		"test/e2e/testdata/capability-tampered-agent.yaml",
	}

	BeforeAll(func() {
		skipIfNoOpenAIKey()

		if expectedDeliveryMode() == "imageVolume" {
			By("switching the operator to capability delivery mode 'auto'")
			Expect(setOperatorCapabilityDeliveryMode("auto")).To(Succeed(),
				"Failed to configure --capability-delivery-mode=auto on the controller")
		}

		By("building the reference capability artifacts (echo, upper)")
		_, err := utils.Run(exec.Command("make", "build-e2e-capability-artifacts",
			"CAPABILITY_IMAGE_PLATFORM="+capabilityImagePlatform()))
		Expect(err).NotTo(HaveOccurred(), "Failed to build the capability artifact fixtures")

		By("building the tampered echo artifact")
		Expect(buildTamperedArtifact(echoArtifactImage, tamperedArtifactImage)).To(Succeed(),
			"Failed to build the tampered artifact image")

		By("loading the artifacts into Kind and resolving their digests")
		echoRef, err = utils.LoadImageAndGetDigest(echoArtifactImage)
		Expect(err).NotTo(HaveOccurred(), "Failed to load the echo artifact")
		upperRef, err = utils.LoadImageAndGetDigest(upperArtifactImage)
		Expect(err).NotTo(HaveOccurred(), "Failed to load the upper artifact")
		tamperedRef, err = utils.LoadImageAndGetDigest(tamperedArtifactImage)
		Expect(err).NotTo(HaveOccurred(), "Failed to load the tampered artifact")
		_, _ = fmt.Fprintf(GinkgoWriter, "Artifact refs:\n  echo:     %s\n  upper:    %s\n  tampered: %s\n",
			echoRef, upperRef, tamperedRef)

		By("creating/updating the OpenAI API key secret")
		Expect(ensureOpenAIAPIKeySecret(namespace)).To(Succeed())

		By("applying the ModelProvider, Model, and Instruction")
		for _, m := range capabilityManifests[:3] {
			Expect(applyManifestFile(m)).To(Succeed(), "Failed to apply %s", m)
		}

		By("creating the Capability CRs (digest-pinned, mirroring the artifact manifests)")
		Expect(ensureCapability("flokoa-cap-echo", echoRef, echoCapabilitySpec)).To(Succeed())
		Expect(ensureCapability("flokoa-cap-upper", upperRef, upperCapabilitySpec)).To(Succeed())
		Expect(ensureCapability("flokoa-cap-echo-tampered", tamperedRef, echoCapabilitySpec)).To(Succeed())
	})

	It("records the effective delivery mode in the state ConfigMap", func() {
		Eventually(func(g Gomega) {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      deliveryStateConfigMapName,
				Namespace: namespace,
			}, cm)
			g.Expect(err).NotTo(HaveOccurred(), "state ConfigMap should exist in the operator namespace")
			g.Expect(cm.Data["effectiveMode"]).To(Equal(expectedDeliveryMode()),
				"probe outcome: %+v", cm.Data)
		}).Should(Succeed())
	})

	It("delivers two capability wheelhouses into a Ready agent pod", func() {
		By("applying the two-capability Agent")
		Expect(applyManifestFile("test/e2e/testdata/capability-agent.yaml")).To(Succeed())

		By("waiting for the Agent to reach Ready")
		Expect(waitForAgentReady("capability-agent", namespace, 3*time.Minute)).To(Succeed(),
			"capability-agent did not become Ready; pod diagnostics:\n%s",
			describeAgentPods("capability-agent"))

		By("inspecting the agent pod's delivery shape")
		pod := getAgentPod("capability-agent")
		Expect(pod.Annotations).To(HaveKeyWithValue(capabilityDeliveryAnnotation, expectedDeliveryMode()))

		switch expectedDeliveryMode() {
		case "imageVolume":
			By("verifying image volumes and zero cap-* initContainers")
			Expect(capInitContainerNames(pod)).To(BeEmpty(),
				"imageVolume mode must not emit copy initContainers")
			for capName, ref := range map[string]string{
				"flokoa-cap-echo":  echoRef,
				"flokoa-cap-upper": upperRef,
			} {
				volume := findVolume(pod, "cap-"+capName)
				Expect(volume).NotTo(BeNil(), "image volume cap-%s should exist", capName)
				Expect(volume.Image).NotTo(BeNil(), "cap-%s must be an image volume", capName)
				Expect(volume.Image.Reference).To(Equal(ref))

				mount := findContainerMountAt(pod, "agent", capabilitiesMountPath+"/"+capName)
				Expect(mount).NotTo(BeNil(), "runner mount for %s should exist", capName)
				Expect(mount.ReadOnly).To(BeTrue())
				Expect(mount.SubPath).To(Equal("wheelhouse"))
			}
		default: // initContainer
			By("verifying two cap-* copy initContainers and the shared emptyDir")
			Expect(capInitContainerNames(pod)).To(ConsistOf(
				"cap-flokoa-cap-echo", "cap-flokoa-cap-upper"))
			for capName, ref := range map[string]string{
				"flokoa-cap-echo":  echoRef,
				"flokoa-cap-upper": upperRef,
			} {
				initContainer := findInitContainer(pod, "cap-"+capName)
				Expect(initContainer).NotTo(BeNil())
				Expect(initContainer.Image).To(Equal(ref), "initContainer pulls the digest-pinned artifact")
			}

			volume := findVolume(pod, "flokoa-capabilities")
			Expect(volume).NotTo(BeNil(), "shared capabilities emptyDir should exist")
			Expect(volume.EmptyDir).NotTo(BeNil())

			mount := findContainerMountAt(pod, "agent", capabilitiesMountPath)
			Expect(mount).NotTo(BeNil(), "runner should mount the capabilities volume")
			Expect(mount.ReadOnly).To(BeTrue(), "runner's capabilities mount must be read-only")
		}

		By("invoking the agent over A2A and exercising the echo capability tool")
		httpClient, agentURL, err := agentA2AProxy("capability-agent")
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			body, err := sendA2AMessage(httpClient, agentURL,
				"Echo this exact text: capability delivery works")
			g.Expect(err).NotTo(HaveOccurred())
			_, _ = fmt.Fprintf(GinkgoWriter, "A2A response: %s\n", body)

			var rpc struct {
				Error  map[string]any `json:"error"`
				Result struct {
					Status struct {
						State string `json:"state"`
					} `json:"status"`
				} `json:"result"`
			}
			g.Expect(json.Unmarshal([]byte(body), &rpc)).To(Succeed())
			g.Expect(rpc.Error).To(BeNil(), "A2A error: %v", rpc.Error)
			g.Expect(rpc.Result.Status.State).To(Equal("completed"))
			// EchoCapability returns "<prefix>: <message>"; the attachment
			// config sets prefix=e2e-cap — observable proof that the
			// wheelhouse installed, the entrypoint hydrated, and the
			// per-agent config applied.
			g.Expect(body).To(ContainSubstring("e2e-cap:"))
		}, 4*time.Minute, 20*time.Second).Should(Succeed())
	})

	It("fails bootstrap with a structured integrity error for a tampered artifact", func() {
		By("applying the Agent pinned to the tampered artifact")
		Expect(applyManifestFile("test/e2e/testdata/capability-tampered-agent.yaml")).To(Succeed())

		By("waiting for the runner to reject the wheelhouse")
		Eventually(func(g Gomega) {
			podList := &corev1.PodList{}
			err := k8sClient.List(ctx, podList,
				client.InNamespace(namespace),
				client.MatchingLabels{"app.kubernetes.io/name": "capability-tampered-agent"})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(podList.Items).NotTo(BeEmpty(), "tampered agent pod should exist")

			pod := podList.Items[0]
			logs := agentContainerLogsAllAttempts(pod.Name)
			// BootstrapError emits single-line structured JSON on stderr
			// (flokoa_runner/errors.py); these substrings are the contract.
			g.Expect(logs).To(ContainSubstring("wheelhouse integrity check failed"),
				"runner should fail the sha256 integrity gate; logs:\n%s", logs)
			g.Expect(logs).To(ContainSubstring("expected_sha256"))
			g.Expect(logs).To(ContainSubstring("install_capabilities"))
		}, 5*time.Minute, 5*time.Second).Should(Succeed())

		By("verifying the tampered agent is never Ready (the readiness probe closes the flap window)")
		// The runner container's readiness probe gates on GET /health, which
		// the FastAPI app serves only once bootstrap reaches `serve`. A
		// tampered wheelhouse fails at `install_capabilities`, so the server
		// never starts and the probe never passes: across the whole crash
		// loop the pod is never Ready, no replica ever counts as available,
		// and the Agent's Ready condition can never flap True. The preceding
		// log assertion already proved bootstrap crashed at least once, so
		// this window observes the post-crash steady state — a hard
		// never-Ready assertion, not a "settles to not-Ready" one.
		Consistently(func(g Gomega) {
			agent := &agentv1alpha1.Agent{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "capability-tampered-agent", Namespace: namespace,
			}, agent)).To(Succeed())
			if ready := findCondition(agent.Status.Conditions, "Ready"); ready != nil {
				g.Expect(ready.Status).NotTo(Equal(metav1.ConditionTrue),
					"a tampered wheelhouse must never let the agent become Ready (reason: %s, message: %s)",
					ready.Reason, ready.Message)
			}
			g.Expect(agent.Status.AvailableReplicas).To(BeZero(),
				"no replica of a tampered agent may ever count as available")
		}, 60*time.Second, 3*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		By("cleaning up capability test resources")
		for i := len(capabilityManifests) - 1; i >= 0; i-- {
			deleteManifestFile(capabilityManifests[i])
		}
		for _, name := range []string{"flokoa-cap-echo", "flokoa-cap-upper", "flokoa-cap-echo-tampered"} {
			capability := &agentv1alpha1.Capability{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			}
			_ = k8sClient.Delete(ctx, capability)
		}
	})
})

// capabilitySpecParams carries the per-fixture Capability spec fields,
// mirroring the fixture's artifact.json (test/e2e/fixtures/capabilities).
type capabilitySpecParams struct {
	entrypoint   string
	configSchema string
	dependencies []string
}

var echoCapabilitySpec = capabilitySpecParams{
	entrypoint: "flokoa_cap_echo:EchoCapability",
	configSchema: `{
		"type": "object",
		"properties": {
			"prefix": {
				"type": "string",
				"default": "echo",
				"description": "Prefix prepended to every echoed message."
			}
		},
		"additionalProperties": false
	}`,
}

var upperCapabilitySpec = capabilitySpecParams{
	entrypoint: "flokoa_cap_upper:UpperCapability",
	configSchema: `{
		"type": "object",
		"properties": {
			"exclaim": {
				"type": "boolean",
				"default": false,
				"description": "Append an exclamation mark to shouted messages."
			}
		},
		"additionalProperties": false
	}`,
	dependencies: []string{"inflection==0.5.1"},
}

// ensureCapability creates (or updates) a digest-pinned Capability CR whose
// spec mirrors the fixture artifact manifest.
func ensureCapability(name, artifactRef string, params capabilitySpecParams) error {
	desired := &agentv1alpha1.Capability{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: agentv1alpha1.CapabilitySpec{
			Artifact:     artifactRef,
			Version:      "0.1.0",
			Entrypoint:   params.entrypoint,
			ConfigSchema: &apiextensionsv1.JSON{Raw: []byte(params.configSchema)},
			SchemaPolicy: agentv1alpha1.SchemaPolicyStrict,
			Requires: agentv1alpha1.CapabilityRequires{
				Python:       "3.13",
				PydanticAI:   ">=1.107,<2",
				FlokoaRunner: ">=0.2",
			},
			Dependencies: params.dependencies,
		},
	}

	err := k8sClient.Create(ctx, desired)
	if apierrors.IsAlreadyExists(err) {
		existing := &agentv1alpha1.Capability{}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(desired), existing); err != nil {
			return err
		}
		existing.Spec = desired.Spec
		return k8sClient.Update(ctx, existing)
	}
	return err
}

// buildTamperedArtifact corrupts the wheel bytes of a built artifact image
// after manifest assembly — the manifest's sha256s no longer match, which is
// exactly what the runner's integrity gate must catch.
func buildTamperedArtifact(baseImage, taggedAs string) error {
	tmpDir, err := os.MkdirTemp("", "cap-tamper-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	dockerfile := fmt.Sprintf(
		"FROM %s\nRUN for f in /wheelhouse/*.whl; do printf tampered >> \"$f\"; done\n",
		baseImage)
	if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0o600); err != nil {
		return err
	}

	// Pin the same platform the fixture artifacts were built for — without
	// it, docker resolves FROM against the host platform, which the local
	// base image may not have.
	_, err = utils.Run(exec.Command("docker", "build",
		"--platform", capabilityImagePlatform(), "-t", taggedAs, tmpDir))
	return err
}

// capabilityImagePlatform is the platform the capability fixture artifacts
// are built and run for. The host architecture is the default — the Kind
// node runs the host's architecture, and the artifacts' wheels are pure
// Python — overridable via CAPABILITY_IMAGE_PLATFORM (the same knob the
// Makefile honors).
func capabilityImagePlatform() string {
	if v := os.Getenv("CAPABILITY_IMAGE_PLATFORM"); v != "" {
		return v
	}
	return "linux/" + runtime.GOARCH
}

// setOperatorCapabilityDeliveryMode adds (or rewrites)
// --capability-delivery-mode on the controller Deployment and waits for the
// rollout; the operator re-resolves delivery at startup.
func setOperatorCapabilityDeliveryMode(mode string) error {
	deploy := &appsv1.Deployment{}
	nn := types.NamespacedName{Name: "flokoa-controller", Namespace: namespace}
	if err := k8sClient.Get(ctx, nn, deploy); err != nil {
		return fmt.Errorf("getting controller deployment: %w", err)
	}

	container := &deploy.Spec.Template.Spec.Containers[0]
	arg := "--capability-delivery-mode=" + mode
	replaced := false
	for i, a := range container.Args {
		if strings.HasPrefix(a, "--capability-delivery-mode=") {
			container.Args[i] = arg
			replaced = true
			break
		}
	}
	if !replaced {
		container.Args = append(container.Args, arg)
	}
	if err := k8sClient.Update(ctx, deploy); err != nil {
		return fmt.Errorf("updating controller deployment: %w", err)
	}

	return waitForDeploymentReady("flokoa-controller", namespace, 3*time.Minute)
}

// getAgentPod returns the single running pod of an agent Deployment.
func getAgentPod(agentName string) *corev1.Pod {
	podList := &corev1.PodList{}
	var pod *corev1.Pod
	Eventually(func(g Gomega) {
		err := k8sClient.List(ctx, podList,
			client.InNamespace(namespace),
			client.MatchingLabels{"app.kubernetes.io/name": agentName})
		g.Expect(err).NotTo(HaveOccurred())
		for i := range podList.Items {
			if podList.Items[i].Status.Phase == corev1.PodRunning {
				pod = &podList.Items[i]
				return
			}
		}
		g.Expect(pod).NotTo(BeNil(), "no running pod for agent %s", agentName)
	}, 2*time.Minute).Should(Succeed())
	return pod
}

func capInitContainerNames(pod *corev1.Pod) []string {
	var names []string
	for _, c := range pod.Spec.InitContainers {
		if strings.HasPrefix(c.Name, "cap-") {
			names = append(names, c.Name)
		}
	}
	return names
}

func findInitContainer(pod *corev1.Pod, name string) *corev1.Container {
	for i := range pod.Spec.InitContainers {
		if pod.Spec.InitContainers[i].Name == name {
			return &pod.Spec.InitContainers[i]
		}
	}
	return nil
}

func findVolume(pod *corev1.Pod, name string) *corev1.Volume {
	for i := range pod.Spec.Volumes {
		if pod.Spec.Volumes[i].Name == name {
			return &pod.Spec.Volumes[i]
		}
	}
	return nil
}

func findContainerMountAt(pod *corev1.Pod, containerName, mountPath string) *corev1.VolumeMount {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name != containerName {
			continue
		}
		for j := range pod.Spec.Containers[i].VolumeMounts {
			if pod.Spec.Containers[i].VolumeMounts[j].MountPath == mountPath {
				return &pod.Spec.Containers[i].VolumeMounts[j]
			}
		}
	}
	return nil
}

// agentA2AProxy returns an HTTP client and the agent's A2A base URL through
// the Kubernetes API server service proxy (no port-forwards needed).
func agentA2AProxy(agentName string) (*http.Client, string, error) {
	transport, err := rest.TransportFor(cfg)
	if err != nil {
		return nil, "", fmt.Errorf("creating transport from kubeconfig: %w", err)
	}
	url := fmt.Sprintf("%s/api/v1/namespaces/%s/services/http:%s:80/proxy/",
		cfg.Host, namespace, agentName)
	return &http.Client{Transport: transport, Timeout: 2 * time.Minute}, url, nil
}

// sendA2AMessage POSTs a JSON-RPC message/send and returns the raw response
// body (the same wire shape the integration suite asserts on).
func sendA2AMessage(httpClient *http.Client, url, text string) (string, error) {
	id := fmt.Sprintf("e2e-cap-%d", time.Now().UnixNano())
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "message/send",
		"params": map[string]any{
			"message": map[string]any{
				"role":      "user",
				"kind":      "message",
				"messageId": "msg-" + id,
				"parts":     []map[string]any{{"kind": "text", "text": text}},
			},
		},
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return string(body), fmt.Errorf("A2A POST returned %d: %s", resp.StatusCode, string(body))
	}
	return string(body), nil
}

// agentContainerLogsAllAttempts concatenates the agent container's current
// and previous-attempt logs (a bootstrap failure crash-loops, so the error
// line may only exist on the previous attempt).
func agentContainerLogsAllAttempts(podName string) string {
	var combined strings.Builder
	for _, previous := range []bool{false, true} {
		logs, err := getPodContainerLogsWithOptions(podName, namespace, "agent", previous)
		if err == nil {
			combined.WriteString(logs)
			combined.WriteString("\n")
		}
	}
	return combined.String()
}

// describeAgentPods renders pod diagnostics for failure messages.
func describeAgentPods(agentName string) string {
	podList := &corev1.PodList{}
	if err := k8sClient.List(ctx, podList,
		client.InNamespace(namespace),
		client.MatchingLabels{"app.kubernetes.io/name": agentName}); err != nil {
		return fmt.Sprintf("failed to list pods: %v", err)
	}
	var b strings.Builder
	for _, pod := range podList.Items {
		fmt.Fprintf(&b, "pod %s: phase=%s\n", pod.Name, pod.Status.Phase)
		for _, cs := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
			fmt.Fprintf(&b, "  container %s: ready=%v restarts=%d state=%+v\n",
				cs.Name, cs.Ready, cs.RestartCount, cs.State)
		}
		fmt.Fprintf(&b, "  logs:\n%s\n", agentContainerLogsAllAttempts(pod.Name))
	}
	return b.String()
}
