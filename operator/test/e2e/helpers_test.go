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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
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
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/test/utils"
)

// namespace where the project is deployed in
const namespace = "flokoa-system"

// serviceAccountName created for the project
const serviceAccountName = "flokoa-controller"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "flokoa-controller-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "flokoa-metrics-binding"

// a2aPluginImage is the name of the A2A plugin image for testing
const a2aPluginImage = "localhost/a2a-plugin:test"

// argoNamespace is the namespace where Argo Workflows is deployed
const argoNamespace = "argo"

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
	defer logs.Close()

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

// loadManifestsFromFile loads Kubernetes manifests from a YAML file and returns unstructured objects
func loadManifestsFromFile(path string) ([]*unstructured.Unstructured, error) {
	projectDir, err := utils.GetProjectDir()
	if err != nil {
		return nil, err
	}

	fullPath := filepath.Join(projectDir, path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", fullPath, err)
	}

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
				if err := wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 30*time.Second, true, func(ctx context.Context) (bool, error) {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), &corev1.Pod{})
					return apierrors.IsNotFound(err), nil
				}); err != nil {
					return fmt.Errorf("failed waiting for %s/%s deletion: %w", obj.GetKind(), obj.GetName(), err)
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

// createNamespace creates a namespace with optional labels
func createNamespace(name string, labels map[string]string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
	err := k8sClient.Create(ctx, ns)
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// deleteNamespace deletes a namespace
func deleteNamespace(name string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	_ = k8sClient.Delete(ctx, ns)
}

// waitForDeploymentReady waits for a deployment to be ready
func waitForDeploymentReady(name, ns string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx2 context.Context) (bool, error) {
		deploy := &appsv1.Deployment{}
		err := k8sClient.Get(ctx2, types.NamespacedName{Name: name, Namespace: ns}, deploy)
		if err != nil {
			return false, nil
		}
		return deploy.Status.ReadyReplicas == *deploy.Spec.Replicas, nil
	})
}

// waitForPodRunning waits for a pod to be in Running phase
func waitForPodRunning(name, ns string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx2 context.Context) (bool, error) {
		pod := &corev1.Pod{}
		err := k8sClient.Get(ctx2, types.NamespacedName{Name: name, Namespace: ns}, pod)
		if err != nil {
			return false, nil
		}
		return pod.Status.Phase == corev1.PodRunning, nil
	})
}

// waitForPodPhase waits for a pod to reach a specific phase
func waitForPodPhase(name, ns string, phase corev1.PodPhase, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx2 context.Context) (bool, error) {
		pod := &corev1.Pod{}
		err := k8sClient.Get(ctx2, types.NamespacedName{Name: name, Namespace: ns}, pod)
		if err != nil {
			return false, nil
		}
		return pod.Status.Phase == phase, nil
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

// waitForWorkflowPhase waits for a workflow to reach a specific phase
func waitForWorkflowPhase(name, ns string, phase wfv1.WorkflowPhase, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx2 context.Context) (bool, error) {
		wf := &wfv1.Workflow{}
		err := k8sClient.Get(ctx2, types.NamespacedName{Name: name, Namespace: ns}, wf)
		if err != nil {
			return false, nil
		}
		return wf.Status.Phase == phase, nil
	})
}

// getWorkflow retrieves a workflow by name
func getWorkflow(name, ns string) (*wfv1.Workflow, error) {
	wf := &wfv1.Workflow{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, wf)
	return wf, err
}

// createWorkflow creates a workflow from a YAML file and returns the created workflow name
func createWorkflow(path string) (string, error) {
	objects, err := loadManifestsFromFile(path)
	if err != nil {
		return "", err
	}

	for _, obj := range objects {
		if obj.GetKind() != "Workflow" {
			continue
		}

		if obj.GetNamespace() == "" {
			obj.SetNamespace(namespace)
		}

		// Convert to Workflow type
		wf := &wfv1.Workflow{}
		data, err := obj.MarshalJSON()
		if err != nil {
			return "", err
		}
		if err := json.Unmarshal(data, wf); err != nil {
			return "", err
		}

		// Create the workflow
		if err := k8sClient.Create(ctx, wf); err != nil {
			return "", err
		}

		return wf.Name, nil
	}

	return "", fmt.Errorf("no Workflow found in %s", path)
}

// deleteWorkflow deletes a workflow by name
func deleteWorkflow(name, ns string) {
	wf := &wfv1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
	_ = k8sClient.Delete(ctx, wf)
}

// createConfigMap creates a ConfigMap
func createConfigMap(name, ns string, data map[string]string, labels map[string]string) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Data: data,
	}
	err := k8sClient.Create(ctx, cm)
	if apierrors.IsAlreadyExists(err) {
		existing := &corev1.ConfigMap{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, existing); err != nil {
			return err
		}
		existing.Data = data
		existing.Labels = labels
		return k8sClient.Update(ctx, existing)
	}
	return err
}

// deleteConfigMap deletes a ConfigMap
func deleteConfigMap(name, ns string) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
	_ = k8sClient.Delete(ctx, cm)
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
	defer logs.Close()

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
