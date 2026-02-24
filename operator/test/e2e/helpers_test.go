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
	"bufio"
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/test/utils"
)

// namespace is the single namespace used for operator, server, and workloads in this test run.
// Initialized in BeforeSuite via initializeTestNamespace().
var namespace string

// serviceAccountName created for the project
const serviceAccountName = "flokoa-controller"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "flokoa-controller-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "flokoa-metrics-binding"

// a2aPluginImage is the name of the A2A plugin image for testing
const a2aPluginImage = "localhost/a2a-plugin:test"

func initializeTestNamespace() string {
	if explicit := os.Getenv("E2E_NAMESPACE"); explicit != "" {
		return explicit
	}

	randomBytes := make([]byte, 4)
	if _, err := crand.Read(randomBytes); err != nil {
		return fmt.Sprintf("flokoa-e2e-%d", time.Now().UnixNano())
	}

	return fmt.Sprintf("flokoa-e2e-%s", hex.EncodeToString(randomBytes))
}

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token.
func serviceAccountToken() (string, error) {
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", fmt.Errorf("failed to create clientset: %w", err)
	}

	tokenRequest := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: ptr(int64(3600)),
		},
	}

	var token string
	verifyTokenCreation := func(g Gomega) {
		result, err := clientset.CoreV1().ServiceAccounts(namespace).CreateToken(
			ctx,
			serviceAccountName,
			tokenRequest,
			metav1.CreateOptions{},
		)
		g.Expect(err).NotTo(HaveOccurred())
		token = result.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return token, nil
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() string {
	By("getting the curl-metrics logs")
	pod := &corev1.Pod{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: "curl-metrics", Namespace: namespace}, pod)
	Expect(err).NotTo(HaveOccurred(), "Failed to get curl-metrics pod")

	clientset, err := kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())

	req := clientset.CoreV1().Pods(namespace).GetLogs("curl-metrics", &corev1.PodLogOptions{})
	logs, err := req.Stream(ctx)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = logs.Close() }()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, logs)
	Expect(err).NotTo(HaveOccurred())

	metricsOutput := buf.String()
	Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
	return metricsOutput
}

// ptr returns a pointer to the given value
func ptr[T any](v T) *T {
	return &v
}

// kustomizeBin returns the path to the kustomize binary built by the Makefile.
func kustomizeBin() string {
	projectDir, _ := utils.GetProjectDir()
	return filepath.Join(projectDir, "bin", "kustomize")
}

// loadManifestsFromFile loads Kubernetes manifests from a YAML file and returns unstructured objects.
// It uses a temporary kustomize overlay to inject the test namespace into all resources.
func loadManifestsFromFile(path string) ([]*unstructured.Unstructured, error) {
	projectDir, err := utils.GetProjectDir()
	if err != nil {
		return nil, err
	}

	fullPath := filepath.Join(projectDir, path)

	// Create a temp overlay that wraps the single file with the test namespace.
	// Kustomize refuses resources outside its root, so we copy the file in.
	tmpDir, err := os.MkdirTemp("", "e2e-kustomize-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	localName := filepath.Base(fullPath)
	srcData, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, localName), srcData, 0o600); err != nil {
		return nil, fmt.Errorf("failed to copy %s: %w", path, err)
	}

	kustomization := fmt.Sprintf(
		"apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nnamespace: %s\nresources:\n  - %s\n",
		namespace, localName,
	)
	if err := os.WriteFile(filepath.Join(tmpDir, "kustomization.yaml"), []byte(kustomization), 0o600); err != nil {
		return nil, fmt.Errorf("failed to write temp kustomization: %w", err)
	}

	cmd := exec.Command(kustomizeBin(), "build", tmpDir)
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("kustomize build failed for %s: %w", path, err)
	}

	return parseYAMLDocuments(data)
}

// loadKustomizeDir builds all resources from a kustomize directory with the test namespace.
func loadKustomizeDir(dir string) ([]*unstructured.Unstructured, error) {
	projectDir, err := utils.GetProjectDir()
	if err != nil {
		return nil, err
	}

	baseDir := filepath.Join(projectDir, dir)

	tmpDir, err := os.MkdirTemp("", "e2e-kustomize-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	kustomization := fmt.Sprintf(
		"apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nnamespace: %s\nresources:\n  - %s\n",
		namespace, baseDir,
	)
	if err := os.WriteFile(filepath.Join(tmpDir, "kustomization.yaml"), []byte(kustomization), 0o600); err != nil {
		return nil, fmt.Errorf("failed to write temp kustomization: %w", err)
	}

	cmd := exec.Command(kustomizeBin(), "build", "--load-restrictor", "LoadRestrictionsNone", tmpDir)
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("kustomize build failed for %s: %w", dir, err)
	}

	return parseYAMLDocuments(data)
}

// parseYAMLDocuments parses multi-document YAML into unstructured objects.
func parseYAMLDocuments(data []byte) ([]*unstructured.Unstructured, error) {
	var objects []*unstructured.Unstructured
	reader := yaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))

	for {
		doc, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read YAML document: %w", err)
		}

		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}

		obj := &unstructured.Unstructured{}
		if err := yaml.Unmarshal(doc, obj); err != nil {
			return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
		}

		if obj.GetKind() == "" {
			continue
		}

		objects = append(objects, obj)
	}

	return objects, nil
}

// applyManifestFile applies all manifests from a YAML file
func applyManifestFile(path string) error {
	objects, err := loadManifestsFromFile(path)
	if err != nil {
		return err
	}

	for _, obj := range objects {
		// Set namespace if not specified and object is namespaced
		if obj.GetNamespace() == "" {
			obj.SetNamespace(namespace)
		}

		existing := obj.DeepCopy()
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), existing)
		if apierrors.IsNotFound(err) {
			if err := k8sClient.Create(ctx, obj); err != nil {
				return fmt.Errorf("failed to create %s/%s: %w", obj.GetKind(), obj.GetName(), err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to get %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		} else {
			// Pods have many immutable fields, so delete and recreate instead of update
			if obj.GetKind() == "Pod" {
				if err := k8sClient.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
					return fmt.Errorf("failed to delete %s/%s for recreation: %w", obj.GetKind(), obj.GetName(), err)
				}
				// Wait for pod to be deleted
				err := wait.PollUntilContextTimeout(
					ctx, 500*time.Millisecond, 30*time.Second, true,
					func(ctx context.Context) (bool, error) {
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), &corev1.Pod{})
						return apierrors.IsNotFound(err), nil
					})
				if err != nil {
					return fmt.Errorf("failed waiting for %s/%s deletion: %w",
						obj.GetKind(), obj.GetName(), err)
				}
				if err := k8sClient.Create(ctx, obj); err != nil {
					return fmt.Errorf("failed to recreate %s/%s: %w", obj.GetKind(), obj.GetName(), err)
				}
			} else {
				obj.SetResourceVersion(existing.GetResourceVersion())
				if err := k8sClient.Update(ctx, obj); err != nil {
					return fmt.Errorf("failed to update %s/%s: %w", obj.GetKind(), obj.GetName(), err)
				}
			}
		}
	}

	return nil
}

// deleteManifestFile deletes all manifests from a YAML file
func deleteManifestFile(path string) {
	objects, err := loadManifestsFromFile(path)
	if err != nil {
		return
	}

	for _, obj := range objects {
		if obj.GetNamespace() == "" {
			obj.SetNamespace(namespace)
		}
		_ = k8sClient.Delete(ctx, obj)
	}
}

// waitForDeploymentReady waits for a deployment to be ready
func waitForDeploymentReady(name, ns string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx2 context.Context) (bool, error) {
		deploy := &appsv1.Deployment{}
		err := k8sClient.Get(ctx2, types.NamespacedName{Name: name, Namespace: ns}, deploy)
		if err != nil {
			return false, nil
		}

		for _, condition := range deploy.Status.Conditions {
			if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
				return true, nil
			}
		}

		desiredReplicas := int32(1)
		if deploy.Spec.Replicas != nil {
			desiredReplicas = *deploy.Spec.Replicas
		}

		return deploy.Status.ReadyReplicas >= desiredReplicas, nil
	})
}

// waitForAgentReady waits for an agent to reach Ready condition
func waitForAgentReady(name, ns string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx2 context.Context) (bool, error) {
		agent := &agentv1alpha1.Agent{}
		err := k8sClient.Get(ctx2, types.NamespacedName{Name: name, Namespace: ns}, agent)
		if err != nil {
			return false, nil
		}
		for _, cond := range agent.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
}

// findCondition returns the condition with the given type, or nil if not found.
func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

// waitForAgentWorkflowReady waits for an AgentWorkflow to become ready.
// It fast-fails if the workflow has a failed Compiled condition.
func waitForAgentWorkflowReady(name, ns string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx2 context.Context) (bool, error) {
		awf := &agentv1alpha1.AgentWorkflow{}
		err := k8sClient.Get(ctx2, types.NamespacedName{Name: name, Namespace: ns}, awf)
		if err != nil {
			return false, nil
		}
		if awf.Status.Ready {
			return true, nil
		}
		// Fast-fail if compilation failed
		for _, c := range awf.Status.Conditions {
			if c.Type == "Compiled" && c.Status == metav1.ConditionFalse {
				return false, fmt.Errorf("AgentWorkflow %s/%s failed compilation: %s", ns, name, c.Message)
			}
		}
		return false, nil
	})
}

// createClusterRoleBinding creates a ClusterRoleBinding
func createClusterRoleBinding(name, clusterRole string, subjects []rbacv1.Subject) error {
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRole,
		},
		Subjects: subjects,
	}
	err := k8sClient.Create(ctx, crb)
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// deleteClusterRoleBinding deletes a ClusterRoleBinding
func deleteClusterRoleBinding(name string) {
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	_ = k8sClient.Delete(ctx, crb)
}

// getPodLogs retrieves logs from a pod
func getPodLogs(name, ns string) (string, error) {
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", err
	}

	req := clientset.CoreV1().Pods(ns).GetLogs(name, &corev1.PodLogOptions{})
	logs, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = logs.Close() }()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, logs)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// createPod creates a Pod
func createPod(pod *corev1.Pod) error {
	return k8sClient.Create(ctx, pod)
}

// deletePod deletes a Pod
func deletePod(name, ns string) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
	_ = k8sClient.Delete(ctx, pod)
}

// ensureOpenAIAPIKeySecret creates or updates the openai-api-key secret from OPENAI_API_KEY env var.
func ensureOpenAIAPIKeySecret(ns string) error {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is required for this e2e test")
	}

	nn := types.NamespacedName{Name: "openai-api-key", Namespace: ns}
	existing := &corev1.Secret{}
	err := k8sClient.Get(ctx, nn, existing)
	if apierrors.IsNotFound(err) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openai-api-key",
				Namespace: ns,
			},
			Type: corev1.SecretTypeOpaque,
			StringData: map[string]string{
				"api-key": apiKey,
			},
		}
		return k8sClient.Create(ctx, secret)
	}
	if err != nil {
		return err
	}

	existing.StringData = map[string]string{"api-key": apiKey}
	if existing.Data == nil {
		existing.Data = map[string][]byte{}
	}
	existing.Data["api-key"] = []byte(apiKey)
	return k8sClient.Update(ctx, existing)
}

// waitForWorkflowCompletion waits for an Argo Workflow to reach a terminal phase.
// Returns the final Workflow object for further assertions.
func waitForWorkflowCompletion(name, ns string, timeout time.Duration) (*wfv1.Workflow, error) {
	var wf wfv1.Workflow
	err := wait.PollUntilContextTimeout(ctx, 3*time.Second, timeout, true, func(ctx2 context.Context) (bool, error) {
		if err := k8sClient.Get(ctx2, types.NamespacedName{Name: name, Namespace: ns}, &wf); err != nil {
			return false, nil
		}
		return wf.Status.Phase.Completed(), nil
	})
	if err != nil {
		return &wf, fmt.Errorf("Workflow %s/%s did not complete (phase=%s): %w", ns, name, wf.Status.Phase, err)
	}
	return &wf, nil
}

// dumpAgentPodDiagnostics finds and logs the Argo agent pod for a workflow.
// Agent pods follow the naming pattern: <wf-name>-<hash>-agent.
func dumpAgentPodDiagnostics(wfName, ns string) {
	podList := &corev1.PodList{}
	err := k8sClient.List(ctx, podList,
		client.InNamespace(ns),
		client.MatchingLabels{"workflows.argoproj.io/workflow": wfName, "workflows.argoproj.io/component": "agent"})
	if err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "Failed to list agent pods: %v\n", err)
		return
	}
	if len(podList.Items) == 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "No agent pod found for workflow %s\n", wfName)
		return
	}
	for _, pod := range podList.Items {
		_, _ = fmt.Fprintf(GinkgoWriter, "Agent pod %s: phase=%s\n", pod.Name, pod.Status.Phase)
		for _, cs := range pod.Status.InitContainerStatuses {
			_, _ = fmt.Fprintf(GinkgoWriter, "  InitContainer %s: ready=%v state=%+v\n", cs.Name, cs.Ready, cs.State)
		}
		for _, cs := range pod.Status.ContainerStatuses {
			_, _ = fmt.Fprintf(GinkgoWriter, "  Container %s: ready=%v restarts=%d state=%+v\n", cs.Name, cs.Ready, cs.RestartCount, cs.State)
			if cs.LastTerminationState.Terminated != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "    Last termination: reason=%s exitCode=%d message=%s\n",
					cs.LastTerminationState.Terminated.Reason, cs.LastTerminationState.Terminated.ExitCode, cs.LastTerminationState.Terminated.Message)
			}
		}
		// Try to get logs from each container
		for _, c := range pod.Spec.Containers {
			logs, logErr := getPodContainerLogs(pod.Name, ns, c.Name)
			if logErr != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "  Logs(%s): error=%v\n", c.Name, logErr)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "  Logs(%s):\n%s\n", c.Name, logs)
			}
		}
	}
}

// getPodContainerLogs retrieves logs from a specific container in a pod
func getPodContainerLogs(podName, ns, containerName string) (string, error) {
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", err
	}
	req := clientset.CoreV1().Pods(ns).GetLogs(podName, &corev1.PodLogOptions{Container: containerName})
	logs, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = logs.Close() }()
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, logs)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// serverRESTProxy returns an HTTP client and base URL for making requests to
// the flokoa-server REST gateway through the Kubernetes API server proxy.
// No pods or port-forwards needed — uses the kubeconfig transport directly.
func serverRESTProxy() (*http.Client, string, error) {
	transport, err := rest.TransportFor(cfg)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create transport from kubeconfig: %w", err)
	}
	baseURL := fmt.Sprintf("%s/api/v1/namespaces/%s/services/http:flokoa-server:8080/proxy",
		cfg.Host, namespace)
	return &http.Client{Transport: transport}, baseURL, nil
}

// newCurlPod creates a restricted curl Pod spec for in-cluster HTTP calls.
// Uses -s (silent) without -f so the pod always succeeds on HTTP responses
// (even 4xx/5xx), allowing callers to inspect HTTP_STATUS in the logs.
func newCurlPod(name, ns, url string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "curl",
					Image:   "curlimages/curl:latest",
					Command: []string{"/bin/sh", "-c"},
					Args:    []string{fmt.Sprintf("curl -s --retry 10 --retry-delay 3 --retry-connrefused --connect-timeout 10 --max-time 30 -o /dev/stdout -w '\\nHTTP_STATUS:%%{http_code}' %s", url)},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: ptr(false),
						Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
						RunAsNonRoot:             ptr(true),
						RunAsUser:                ptr(int64(1000)),
						SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
				},
			},
		},
	}
}
