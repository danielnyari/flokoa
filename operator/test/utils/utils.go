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

package utils

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	prometheusOperatorVersion = "v0.77.1"
	prometheusOperatorURL     = "https://github.com/prometheus-operator/prometheus-operator/" +
		"releases/download/%s/bundle.yaml"

	certmanagerVersion = "v1.16.3"
	certmanagerURLTmpl = "https://github.com/cert-manager/cert-manager/releases/download/%s/cert-manager.yaml"

	argoWorkflowsVersion    = "3.7.9"
	argoWorkflowsQuickStart = "https://github.com/argoproj/argo-workflows/releases/download/v%s/quick-start-minimal.yaml"

	argoNamespace = "argo"
)

func warnError(err error) {
	_, _ = fmt.Fprintf(GinkgoWriter, "warning: %v\n", err)
}

// Run executes the provided command within this context
func Run(cmd *exec.Cmd) (string, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "chdir dir: %q\n", err)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %q\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%q failed with error %q: %w", command, string(output), err)
	}

	return string(output), nil
}

// InstallPrometheusOperator installs the prometheus Operator to be used to export the enabled metrics.
func InstallPrometheusOperator() error {
	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
	cmd := exec.Command("kubectl", "create", "-f", url)
	_, err := Run(cmd)
	return err
}

// UninstallPrometheusOperator uninstalls the prometheus
func UninstallPrometheusOperator() {
	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
	cmd := exec.Command("kubectl", "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// IsPrometheusCRDsInstalled checks if any Prometheus CRDs are installed
// by verifying the existence of key CRDs related to Prometheus.
func IsPrometheusCRDsInstalled() bool {
	// List of common Prometheus CRDs
	prometheusCRDs := []string{
		"prometheuses.monitoring.coreos.com",
		"prometheusrules.monitoring.coreos.com",
		"prometheusagents.monitoring.coreos.com",
	}

	cmd := exec.Command("kubectl", "get", "crds", "-o", "custom-columns=NAME:.metadata.name")
	output, err := Run(cmd)
	if err != nil {
		return false
	}
	crdList := GetNonEmptyLines(output)
	for _, crd := range prometheusCRDs {
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				return true
			}
		}
	}

	return false
}

// UninstallCertManager uninstalls the cert manager
func UninstallCertManager() {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// InstallCertManager installs the cert manager bundle.
func InstallCertManager() error {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "apply", "-f", url)
	if _, err := Run(cmd); err != nil {
		return err
	}
	// Wait for cert-manager-webhook to be ready, which can take time if cert-manager
	// was re-installed after uninstalling on a cluster.
	cmd = exec.Command("kubectl", "wait", "deployment.apps/cert-manager-webhook",
		"--for", "condition=Available",
		"--namespace", "cert-manager",
		"--timeout", "5m",
	)

	_, err := Run(cmd)
	return err
}

// IsCertManagerCRDsInstalled checks if any Cert Manager CRDs are installed
// by verifying the existence of key CRDs related to Cert Manager.
func IsCertManagerCRDsInstalled() bool {
	// List of common Cert Manager CRDs
	certManagerCRDs := []string{
		"certificates.cert-manager.io",
		"issuers.cert-manager.io",
		"clusterissuers.cert-manager.io",
		"certificaterequests.cert-manager.io",
		"orders.acme.cert-manager.io",
		"challenges.acme.cert-manager.io",
	}

	// Execute the kubectl command to get all CRDs
	cmd := exec.Command("kubectl", "get", "crds")
	output, err := Run(cmd)
	if err != nil {
		return false
	}

	// Check if any of the Cert Manager CRDs are present
	crdList := GetNonEmptyLines(output)
	for _, crd := range certManagerCRDs {
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				return true
			}
		}
	}

	return false
}

// LoadImageToKindClusterWithName loads a local docker image to the kind cluster
func LoadImageToKindClusterWithName(name string) error {
	kindOptions := []string{"load", "docker-image", name, "--name", kindClusterName()}
	cmd := exec.Command("kind", kindOptions...)
	_, err := Run(cmd)
	return err
}

// kindClusterName returns the Kind cluster targeted by the e2e run.
func kindClusterName() string {
	if v, ok := os.LookupEnv("KIND_CLUSTER"); ok {
		return v
	}
	return "kind"
}

// LoadImageAndGetDigest loads a locally-built docker image into the Kind
// cluster and returns a digest-pinned reference (repo@sha256:…) that pods —
// and, in particular, Capability CRs, whose admission requires digest
// pinning — can use without any registry.
//
// Locally-built images have no RepoDigest on the docker side (a manifest
// digest only exists once an OCI manifest does), so the digest is read back
// from the node's containerd store after `kind load` imports the archive.
// This is the one place the e2e suite couples to Kind node internals
// (`docker exec <node> crictl/ctr`); everything else consumes the returned
// reference like any registry ref. If this ever proves brittle against a
// Kind/containerd upgrade, the documented fallback is the standard
// local-registry pattern (https://kind.sigs.k8s.io/docs/user/local-registry/):
// run a registry container next to the cluster, `docker push` the fixture
// images, and read the digest from the push output instead.
func LoadImageAndGetDigest(name string) (string, error) {
	if err := LoadImageToKindClusterWithName(name); err != nil {
		return "", fmt.Errorf("loading %s into kind: %w", name, err)
	}

	nodesOut, err := Run(exec.Command("kind", "get", "nodes", "--name", kindClusterName()))
	if err != nil {
		return "", fmt.Errorf("listing kind nodes: %w", err)
	}
	nodes := GetNonEmptyLines(nodesOut)
	if len(nodes) == 0 {
		return "", fmt.Errorf("kind cluster %q has no nodes", kindClusterName())
	}

	// Read the containerd-recorded manifest digest via CRI on the first node
	// (kind load imports the image on every node; the archive is identical,
	// so the digest is too).
	inspectOut, err := Run(exec.Command(
		"docker", "exec", nodes[0], "crictl", "inspecti", "--output", "json", name))
	if err != nil {
		return "", fmt.Errorf("inspecting %s on node %s: %w", name, nodes[0], err)
	}
	var inspect struct {
		Status struct {
			RepoTags    []string `json:"repoTags"`
			RepoDigests []string `json:"repoDigests"`
		} `json:"status"`
	}
	if err := json.Unmarshal([]byte(inspectOut), &inspect); err != nil {
		return "", fmt.Errorf("parsing crictl inspecti output for %s: %w", name, err)
	}
	if len(inspect.Status.RepoTags) == 0 {
		return "", fmt.Errorf(
			"no repoTags recorded for %s on node %s (crictl output: %s)",
			name, nodes[0], inspectOut)
	}
	tagRef := inspect.Status.RepoTags[0]

	var digestRef string
	if len(inspect.Status.RepoDigests) > 0 {
		digestRef = inspect.Status.RepoDigests[0]
	} else {
		// `kind load` imports a docker-archive: containerd computes and stores
		// the manifest digest but records no repo@digest reference, so CRI
		// reports empty repoDigests. Read the digest from containerd's image
		// store instead and synthesize the canonical reference.
		digest, err := nodeImageDigest(nodes[0], tagRef)
		if err != nil {
			return "", err
		}
		repo := tagRef
		if i := strings.LastIndex(tagRef, ":"); i > strings.LastIndex(tagRef, "/") {
			repo = tagRef[:i]
		}
		digestRef = repo + "@" + digest
	}

	// `kind load` only records the tag reference. Register the canonical
	// repo@digest reference too, on every node, so the kubelet's
	// by-digest lookup (IfNotPresent) resolves without a registry.
	for _, node := range nodes {
		if _, err := Run(exec.Command(
			"docker", "exec", node, "ctr", "--namespace", "k8s.io",
			"images", "tag", "--force", tagRef, digestRef)); err != nil {
			return "", fmt.Errorf("registering digest ref %s on node %s: %w", digestRef, node, err)
		}
	}

	return digestRef, nil
}

// nodeImageDigest reads an image's manifest digest from a Kind node's
// containerd store (`ctr images ls` prints a table whose DIGEST column is
// always populated, unlike CRI repoDigests for archive-imported images).
func nodeImageDigest(node, tagRef string) (string, error) {
	lsOut, err := Run(exec.Command(
		"docker", "exec", node, "ctr", "--namespace", "k8s.io",
		"images", "ls", "name=="+tagRef))
	if err != nil {
		return "", fmt.Errorf("listing %s in containerd on node %s: %w", tagRef, node, err)
	}
	for _, line := range GetNonEmptyLines(lsOut) {
		if !strings.HasPrefix(line, tagRef+" ") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "sha256:") {
				return field, nil
			}
		}
	}
	return "", fmt.Errorf(
		"no containerd digest found for %s on node %s (ctr output: %s)", tagRef, node, lsOut)
}

// GetNonEmptyLines converts given command output string into individual objects
// according to line breakers, and ignores the empty elements in it.
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.Split(output, "\n")
	for _, element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// GetProjectDir will return the directory where the project is
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, fmt.Errorf("failed to get current working directory: %w", err)
	}
	wd = strings.ReplaceAll(wd, "/test/e2e", "")
	return wd, nil
}

// InstallArgoWorkflows installs Argo Workflows with executor plugins enabled
func InstallArgoWorkflows(ctx context.Context, k8sClient client.Client, managedNamespaces ...string) error {
	// Create argo namespace using client-go
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: argoNamespace,
		},
	}
	if err := k8sClient.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create argo namespace: %w", err)
	}

	// Apply quick-start-minimal.yaml - this still uses kubectl as it's a remote URL
	// and requires multi-document YAML parsing that's complex to do with client-go
	url := fmt.Sprintf(argoWorkflowsQuickStart, argoWorkflowsVersion)
	cmd := exec.Command("kubectl", "apply", "-n", argoNamespace, "-f", url)
	if _, err := Run(cmd); err != nil {
		return fmt.Errorf("failed to apply Argo Workflows manifests: %w", err)
	}

	// Patch workflow-controller deployment to enable executor plugins
	// and optionally configure additional managed namespaces.
	// Use retry to handle optimistic concurrency conflicts.
	patchErr := wait.PollUntilContextTimeout(
		ctx, time.Second, 30*time.Second, true,
		func(ctx context.Context) (bool, error) {
			deploy := &appsv1.Deployment{}
			nn := types.NamespacedName{Name: "workflow-controller", Namespace: argoNamespace}
			if err := k8sClient.Get(ctx, nn, deploy); err != nil {
				return false, fmt.Errorf("failed to get workflow-controller deployment: %w", err)
			}

			container := &deploy.Spec.Template.Spec.Containers[0]

			// Add ARGO_EXECUTOR_PLUGINS env var
			envExists := false
			for i, env := range container.Env {
				if env.Name == "ARGO_EXECUTOR_PLUGINS" {
					container.Env[i].Value = "true"
					envExists = true
					break
				}
			}
			if !envExists {
				container.Env = append(container.Env, corev1.EnvVar{
					Name:  "ARGO_EXECUTOR_PLUGINS",
					Value: "true",
				})
			}

			// If additional namespaces are specified, remove --namespaced flag
			// and add --managed-namespace for each namespace so the controller
			// watches workflows outside the argo namespace.
			if len(managedNamespaces) > 0 {
				var filteredArgs []string
				for _, arg := range container.Args {
					if arg != "--namespaced" {
						filteredArgs = append(filteredArgs, arg)
					}
				}
				// Always include the argo namespace since we removed --namespaced
				filteredArgs = append(filteredArgs, fmt.Sprintf("--managed-namespace=%s", argoNamespace))
				for _, managedNs := range managedNamespaces {
					filteredArgs = append(filteredArgs, fmt.Sprintf("--managed-namespace=%s", managedNs))
				}
				container.Args = filteredArgs
			}

			if err := k8sClient.Update(ctx, deploy); err != nil {
				if apierrors.IsConflict(err) {
					return false, nil // retry on conflict
				}
				return false, fmt.Errorf("failed to update workflow-controller deployment: %w", err)
			}
			return true, nil
		})
	if patchErr != nil {
		return fmt.Errorf("failed to patch workflow-controller: %w", patchErr)
	}

	// Wait for workflow-controller to be ready (5 minutes to allow for image pulls)
	err := wait.PollUntilContextTimeout(
		ctx, 2*time.Second, 5*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			d := &appsv1.Deployment{}
			nn := types.NamespacedName{
				Name: "workflow-controller", Namespace: argoNamespace,
			}
			if err := k8sClient.Get(ctx, nn, d); err != nil {
				return false, nil
			}
			if d.Spec.Replicas == nil {
				return false, nil
			}
			return d.Status.ReadyReplicas == *d.Spec.Replicas && d.Status.UpdatedReplicas == *d.Spec.Replicas, nil
		})
	if err != nil {
		return fmt.Errorf("workflow-controller not ready: %w", err)
	}

	return nil
}

// UninstallArgoWorkflows uninstalls Argo Workflows
func UninstallArgoWorkflows(ctx context.Context, k8sClient client.Client) {
	// Delete using kubectl since the manifests are from a URL
	url := fmt.Sprintf(argoWorkflowsQuickStart, argoWorkflowsVersion)
	cmd := exec.Command("kubectl", "delete", "-n", argoNamespace, "-f", url, "--ignore-not-found=true")
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}

	// Delete namespace using client-go
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: argoNamespace,
		},
	}
	if err := k8sClient.Delete(ctx, ns); err != nil && !apierrors.IsNotFound(err) {
		warnError(err)
	}
}

// IsArgoWorkflowsInstalled checks if Argo Workflows is installed
func IsArgoWorkflowsInstalled(ctx context.Context, k8sClient client.Client) bool {
	deploy := &appsv1.Deployment{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: "workflow-controller", Namespace: argoNamespace}, deploy)
	return err == nil
}

// InstallA2AExecutorPlugin installs the A2A executor plugin to Argo using client-go
func InstallA2AExecutorPlugin(ctx context.Context, k8sClient client.Client, pluginImage string) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "a2a-executor-plugin",
			Namespace: argoNamespace,
			Labels: map[string]string{
				"workflows.argoproj.io/configmap-type": "ExecutorPlugin",
			},
		},
		Data: map[string]string{
			"sidecar.automountServiceAccountToken": "true",
			"sidecar.container": fmt.Sprintf(`command:
- /a2a-plugin
image: %s
name: a2a-executor-plugin
ports:
- containerPort: 4355
  protocol: TCP
resources:
  limits:
    cpu: 500m
    memory: 128Mi
  requests:
    cpu: 100m
    memory: 64Mi
securityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop:
    - ALL
  readOnlyRootFilesystem: true
  runAsNonRoot: true
  runAsUser: 65532
`, pluginImage),
		},
	}

	// Try to create, if exists update
	err := k8sClient.Create(ctx, cm)
	if apierrors.IsAlreadyExists(err) {
		existing := &corev1.ConfigMap{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, existing); err != nil {
			return err
		}
		existing.Data = cm.Data
		existing.Labels = cm.Labels
		return k8sClient.Update(ctx, existing)
	}
	return err
}

// UninstallA2AExecutorPlugin uninstalls the A2A executor plugin from Argo
func UninstallA2AExecutorPlugin(ctx context.Context, k8sClient client.Client) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "a2a-executor-plugin",
			Namespace: argoNamespace,
		},
	}
	if err := k8sClient.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
		warnError(err)
	}
}

// UncommentCode searches for target in the file and remove the comment prefix
// of the target content. The target content may span multiple lines.
func UncommentCode(filename, target, prefix string) error {
	// false positive
	// nolint:gosec
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", filename, err)
	}
	strContent := string(content)

	idx := strings.Index(strContent, target)
	if idx < 0 {
		return fmt.Errorf("unable to find the code %q to be uncomment", target)
	}

	out := new(bytes.Buffer)
	_, err = out.Write(content[:idx])
	if err != nil {
		return fmt.Errorf("failed to write to output: %w", err)
	}

	scanner := bufio.NewScanner(bytes.NewBufferString(target))
	if !scanner.Scan() {
		return nil
	}
	for {
		if _, err = out.WriteString(strings.TrimPrefix(scanner.Text(), prefix)); err != nil {
			return fmt.Errorf("failed to write to output: %w", err)
		}
		// Avoid writing a newline in case the previous line was the last in target.
		if !scanner.Scan() {
			break
		}
		if _, err = out.WriteString("\n"); err != nil {
			return fmt.Errorf("failed to write to output: %w", err)
		}
	}

	if _, err = out.Write(content[idx+len(target):]); err != nil {
		return fmt.Errorf("failed to write to output: %w", err)
	}

	// false positive
	// nolint:gosec
	if err = os.WriteFile(filename, out.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write file %q: %w", filename, err)
	}

	return nil
}
