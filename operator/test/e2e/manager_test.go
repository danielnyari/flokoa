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
	"fmt"
	"sort"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// After all tests in this describe block, clean up resources specific to manager tests
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		deletePod("curl-metrics", managerNamespace)

		By("cleaning up metrics role binding")
		deleteClusterRoleBinding(metricsRoleBindingName)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			controllerLogs, err := getPodLogs(controllerPodName, managerNamespace)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			eventList := &corev1.EventList{}
			if err := k8sClient.List(ctx, eventList, client.InNamespace(managerNamespace)); err == nil {
				// Sort events by last timestamp
				sort.Slice(eventList.Items, func(i, j int) bool {
					return eventList.Items[i].LastTimestamp.Before(&eventList.Items[j].LastTimestamp)
				})
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n")
				for _, event := range eventList.Items {
					_, _ = fmt.Fprintf(GinkgoWriter, "  %s %s %s: %s\n",
						event.LastTimestamp.Format(time.RFC3339),
						event.InvolvedObject.Name,
						event.Reason,
						event.Message)
				}
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			metricsOutput, err := getPodLogs("curl-metrics", namespace)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			pod := &corev1.Pod{}
			nn := types.NamespacedName{Name: controllerPodName, Namespace: managerNamespace}
			if err := k8sClient.Get(ctx, nn, pod); err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Pod description:\n  Name: %s\n  Phase: %s\n  Conditions:\n",
					pod.Name, pod.Status.Phase)
				for _, cond := range pod.Status.Conditions {
					_, _ = fmt.Fprintf(GinkgoWriter, "    %s: %s (%s)\n", cond.Type, cond.Status, cond.Message)
				}
				_, _ = fmt.Fprintf(GinkgoWriter, "  Container Statuses:\n")
				for _, cs := range pod.Status.ContainerStatuses {
					_, _ = fmt.Fprintf(GinkgoWriter, "    %s: Ready=%v, RestartCount=%d\n",
						cs.Name, cs.Ready, cs.RestartCount)
				}
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to describe controller pod: %s\n", err)
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Controller Pod", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the controller-manager pods
				podList := &corev1.PodList{}
				err := k8sClient.List(ctx, podList,
					client.InNamespace(managerNamespace),
					client.MatchingLabels{
						"app.kubernetes.io/name":      "flokoa",
						"app.kubernetes.io/component": "controller",
					})
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")

				// Filter out pods with deletion timestamp
				var runningPods []corev1.Pod
				for _, pod := range podList.Items {
					if pod.DeletionTimestamp == nil {
						runningPods = append(runningPods, pod)
					}
				}

				g.Expect(runningPods).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = runningPods[0].Name
				g.Expect(controllerPodName).To(ContainSubstring("flokoa-controller"))
				g.Expect(runningPods[0].Status.Phase).To(Equal(corev1.PodRunning), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})
	})

	Context("Metrics", func() {
		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			err := createClusterRoleBinding(metricsRoleBindingName, "metrics-reader", []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      serviceAccountName,
					Namespace: managerNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			svc := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: metricsServiceName, Namespace: managerNamespace}, svc)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("waiting for the metrics endpoint to be ready")
			verifyMetricsEndpointReady := func(g Gomega) {
				endpoints := &corev1.Endpoints{} //nolint:staticcheck // TODO: migrate to discoveryv1.EndpointSlice
				err := k8sClient.Get(ctx, types.NamespacedName{Name: metricsServiceName, Namespace: managerNamespace}, endpoints)
				g.Expect(err).NotTo(HaveOccurred())
				// Check that we have at least one ready address
				hasReadyAddress := false
				for _, subset := range endpoints.Subsets {
					if len(subset.Addresses) > 0 {
						hasReadyAddress = true
						break
					}
				}
				g.Expect(hasReadyAddress).To(BeTrue(), "Metrics endpoint is not ready")
			}
			Eventually(verifyMetricsEndpointReady).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				// Get the controller pod name first
				podList := &corev1.PodList{}
				err := k8sClient.List(ctx, podList,
					client.InNamespace(managerNamespace),
					client.MatchingLabels{
						"app.kubernetes.io/name":      "flokoa",
						"app.kubernetes.io/component": "controller",
					})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(podList.Items).NotTo(BeEmpty())

				for _, pod := range podList.Items {
					if pod.DeletionTimestamp == nil {
						controllerPodName = pod.Name
						break
					}
				}

				output, err := getPodLogs(controllerPodName, managerNamespace)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("controller-runtime.metrics\tServing metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted).Should(Succeed())

			By("creating the curl-metrics pod to access the metrics endpoint")
			curlPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "curl-metrics",
					Namespace: managerNamespace,
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: serviceAccountName,
					Containers: []corev1.Container{
						{
							Name:    "curl",
							Image:   "curlimages/curl:latest",
							Command: []string{"/bin/sh", "-c"},
							Args: []string{
								fmt.Sprintf("curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics",
									token, metricsServiceName, managerNamespace),
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								RunAsNonRoot: ptr(true),
								RunAsUser:    ptr(int64(1000)),
								SeccompProfile: &corev1.SeccompProfile{
									Type: corev1.SeccompProfileTypeRuntimeDefault,
								},
							},
						},
					},
				},
			}
			err = createPod(curlPod)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				pod := &corev1.Pod{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "curl-metrics", Namespace: managerNamespace}, pod)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(pod.Status.Phase).To(Equal(corev1.PodSucceeded), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			metricsOutput := getMetricsOutput()
			Expect(metricsOutput).To(ContainSubstring(
				"controller_runtime_reconcile_total",
			))
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks
	})
})
