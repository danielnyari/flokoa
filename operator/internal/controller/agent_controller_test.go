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

package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

var _ = Describe("Agent Controller", func() {
	Context("When reconciling an Agent resource", func() {
		const (
			agentNamespace = "default"
			timeout        = time.Second * 10
			interval       = time.Millisecond * 250
		)

		var (
			ctx                context.Context
			agentName          string
			typeNamespacedName types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			// Use unique name per test to avoid conflicts
			agentName = fmt.Sprintf("test-agent-%d", time.Now().UnixNano())
			typeNamespacedName = types.NamespacedName{
				Name:      agentName,
				Namespace: agentNamespace,
			}
		})

		AfterEach(func() {
			// Cleanup the Agent resource
			agent := &agentv1alpha1.Agent{}
			err := k8sClient.Get(ctx, typeNamespacedName, agent)
			if err == nil {
				By("Cleaning up the Agent resource")

				// Remove finalizer if present to allow deletion
				if controllerutil.ContainsFinalizer(agent, agentFinalizer) {
					controllerutil.RemoveFinalizer(agent, agentFinalizer)
					Expect(k8sClient.Update(ctx, agent)).To(Succeed())
				}

				Expect(k8sClient.Delete(ctx, agent)).To(Succeed())

				// Wait for deletion to complete
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agent)
					return errors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			}
		})

		Context("Basic reconciliation", func() {
			It("should create Deployment and Service for a new Agent", func() {
				By("Creating a new Agent resource")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Runtime: agentv1alpha1.RuntimeSpec{
							Container: corev1.Container{
								Name:  "agent",
								Image: "nginx:latest",
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: 8080,
										Protocol:      corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Reconciling the Agent to add finalizer")
				controllerReconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				// First reconcile adds the finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				By("Reconciling the Agent again to create resources")
				// Second reconcile creates the Deployment and Service
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying the Deployment was created")
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, deployment)
				}, timeout, interval).Should(Succeed())

				Expect(deployment.Spec.Replicas).NotTo(BeNil())
				Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
				Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
				Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal("nginx:latest"))

				By("Verifying the Service was created")
				service := &corev1.Service{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, service)
				}, timeout, interval).Should(Succeed())

				Expect(service.Spec.Ports).To(HaveLen(1))
				Expect(service.Spec.Ports[0].Port).To(Equal(int32(8080)))
				Expect(service.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt32(8080)))

				By("Verifying the Agent status was updated")
				Eventually(func() string {
					err := k8sClient.Get(ctx, typeNamespacedName, agent)
					if err != nil {
						return ""
					}
					return agent.Status.Phase
				}, timeout, interval).Should(Equal("Pending"))

				Expect(agent.Status.Backend).To(Equal("core"))
				Expect(agent.Status.URL).To(ContainSubstring(agentName))
			})

			It("should add finalizer to the Agent", func() {
				By("Creating a new Agent resource")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Runtime: agentv1alpha1.RuntimeSpec{
							Container: corev1.Container{
								Image: "nginx:latest",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Reconciling the Agent")
				controllerReconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying finalizer was added")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agent)
					if err != nil {
						return false
					}
					return controllerutil.ContainsFinalizer(agent, agentFinalizer)
				}, timeout, interval).Should(BeTrue())
			})
		})

		Context("Custom configuration", func() {
			It("should respect custom replica count", func() {
				By("Creating an Agent with custom replica count")
				replicas := int32(3)
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Runtime: agentv1alpha1.RuntimeSpec{
							Replicas: &replicas,
							Container: corev1.Container{
								Image: "nginx:latest",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Reconciling the Agent")
				controllerReconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile creates resources
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Deployment has correct replica count")
				deployment := &appsv1.Deployment{}
				Eventually(func() int32 {
					err := k8sClient.Get(ctx, typeNamespacedName, deployment)
					if err != nil || deployment.Spec.Replicas == nil {
						return 0
					}
					return *deployment.Spec.Replicas
				}, timeout, interval).Should(Equal(int32(3)))
			})

			It("should create Service with custom ports", func() {
				By("Creating an Agent with custom ports")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Runtime: agentv1alpha1.RuntimeSpec{
							Container: corev1.Container{
								Image: "nginx:latest",
								Ports: []corev1.ContainerPort{
									{
										Name:          "http",
										ContainerPort: 3000,
										Protocol:      corev1.ProtocolTCP,
									},
									{
										Name:          "metrics",
										ContainerPort: 9090,
										Protocol:      corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Reconciling the Agent")
				controllerReconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile creates resources
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Service has custom ports")
				service := &corev1.Service{}
				Eventually(func() int {
					err := k8sClient.Get(ctx, typeNamespacedName, service)
					if err != nil {
						return 0
					}
					return len(service.Spec.Ports)
				}, timeout, interval).Should(Equal(2))

				Expect(service.Spec.Ports[0].Name).To(Equal("http"))
				Expect(service.Spec.Ports[0].Port).To(Equal(int32(3000)))
				Expect(service.Spec.Ports[1].Name).To(Equal("metrics"))
				Expect(service.Spec.Ports[1].Port).To(Equal(int32(9090)))
			})

			It("should propagate container resource limits", func() {
				By("Creating an Agent with resource limits")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Runtime: agentv1alpha1.RuntimeSpec{
							Container: corev1.Container{
								Image: "nginx:latest",
								Resources: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("500m"),
										corev1.ResourceMemory: resource.MustParse("512Mi"),
									},
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("128Mi"),
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Reconciling the Agent")
				controllerReconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile creates resources
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Deployment has resource limits")
				deployment := &appsv1.Deployment{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, deployment)
					return err == nil && len(deployment.Spec.Template.Spec.Containers) > 0
				}, timeout, interval).Should(BeTrue())

				container := deployment.Spec.Template.Spec.Containers[0]
				Expect(container.Resources.Limits.Cpu().String()).To(Equal("500m"))
				Expect(container.Resources.Limits.Memory().String()).To(Equal("512Mi"))
				Expect(container.Resources.Requests.Cpu().String()).To(Equal("100m"))
				Expect(container.Resources.Requests.Memory().String()).To(Equal("128Mi"))
			})
		})

		Context("Status updates", func() {
			It("should update status conditions", func() {
				By("Creating a new Agent")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Runtime: agentv1alpha1.RuntimeSpec{
							Container: corev1.Container{
								Image: "nginx:latest",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Reconciling the Agent")
				controllerReconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile creates resources and updates status
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Ready condition exists")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agent)
					if err != nil {
						return false
					}
					condition := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeReady)
					return condition != nil
				}, timeout, interval).Should(BeTrue())

				err = k8sClient.Get(ctx, typeNamespacedName, agent)
				Expect(err).NotTo(HaveOccurred())
				condition := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeReady)
				Expect(condition.Reason).To(Equal(ReasonDeploymentNotReady))
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			})

			It("should update observedGeneration", func() {
				By("Creating a new Agent")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Runtime: agentv1alpha1.RuntimeSpec{
							Container: corev1.Container{
								Image: "nginx:latest",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Reconciling the Agent")
				controllerReconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile creates resources and updates status
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying observedGeneration matches generation")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agent)
					if err != nil {
						return false
					}
					return agent.Status.ObservedGeneration == agent.Generation
				}, timeout, interval).Should(BeTrue())
			})
		})

		Context("Label propagation", func() {
			It("should apply correct labels to Deployment and Service", func() {
				By("Creating a new Agent")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Runtime: agentv1alpha1.RuntimeSpec{
							Container: corev1.Container{
								Image: "nginx:latest",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Reconciling the Agent")
				controllerReconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile creates resources
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Deployment labels")
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, deployment)
				}, timeout, interval).Should(Succeed())

				Expect(deployment.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", agentName))
				Expect(deployment.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", agentName))
				Expect(deployment.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "flokoa-operator"))
				Expect(deployment.Labels).To(HaveKeyWithValue("flokoa.ai/agent", agentName))

				By("Verifying Service labels")
				service := &corev1.Service{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, service)
				}, timeout, interval).Should(Succeed())

				Expect(service.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", agentName))
				Expect(service.Spec.Selector).To(HaveKeyWithValue("flokoa.ai/agent", agentName))
			})
		})

		Context("Default service ports", func() {
			It("should create default service port when no container ports specified", func() {
				By("Creating an Agent without container ports")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Runtime: agentv1alpha1.RuntimeSpec{
							Container: corev1.Container{
								Image: "nginx:latest",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Reconciling the Agent")
				controllerReconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile creates resources
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Service has default port")
				service := &corev1.Service{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, service)
					return err == nil && len(service.Spec.Ports) > 0
				}, timeout, interval).Should(BeTrue())

				Expect(service.Spec.Ports).To(HaveLen(1))
				Expect(service.Spec.Ports[0].Name).To(Equal("http"))
				Expect(service.Spec.Ports[0].Port).To(Equal(int32(80)))
				Expect(service.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt32(8080)))
			})
		})

		Context("Non-existent resource", func() {
			It("should handle reconcile request for non-existent Agent", func() {
				By("Reconciling a non-existent Agent")
				controllerReconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				nonExistentName := types.NamespacedName{
					Name:      "non-existent-agent",
					Namespace: agentNamespace,
				}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: nonExistentName,
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		// HIGH PRIORITY TEST: Deletion and cleanup logic
		Context("Deletion with finalizers", func() {
			It("should clean up owned resources before removing finalizer", func() {
				By("Creating an Agent")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Runtime: agentv1alpha1.RuntimeSpec{
							Container: corev1.Container{
								Image: "nginx:latest",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				controllerReconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				By("Reconciling to add finalizer and create resources")
				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile creates resources
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Deployment and Service exist")
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, deployment)
				}, timeout, interval).Should(Succeed())

				service := &corev1.Service{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, service)
				}, timeout, interval).Should(Succeed())

				By("Deleting the Agent")
				err = k8sClient.Get(ctx, typeNamespacedName, agent)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Delete(ctx, agent)).To(Succeed())

				By("Reconciling to process deletion")
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying finalizer was removed")
				err = k8sClient.Get(ctx, typeNamespacedName, agent)
				if err == nil {
					Expect(controllerutil.ContainsFinalizer(agent, agentFinalizer)).To(BeFalse())
				} else {
					// Agent may already be deleted
					Expect(errors.IsNotFound(err)).To(BeTrue())
				}

				By("Verifying owned resources are cleaned up by Kubernetes garbage collection")
				// Note: Owned resources will be deleted by Kubernetes GC since we set OwnerReference
				// In a real cluster, this happens automatically, but in envtest we verify the reference exists
				deployment = &appsv1.Deployment{}
				err = k8sClient.Get(ctx, typeNamespacedName, deployment)
				if err == nil {
					// Verify owner reference is set (GC will clean this up)
					Expect(deployment.OwnerReferences).NotTo(BeEmpty())
					Expect(deployment.OwnerReferences[0].Name).To(Equal(agentName))
				}
			})

			It("should not block deletion if owned resources are manually deleted", func() {
				By("Creating an Agent")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Runtime: agentv1alpha1.RuntimeSpec{
							Container: corev1.Container{
								Image: "nginx:latest",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				controllerReconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				By("Reconciling to create resources")
				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile creates resources
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Manually deleting owned Deployment and Service")
				deployment := &appsv1.Deployment{}
				err = k8sClient.Get(ctx, typeNamespacedName, deployment)
				if err == nil {
					Expect(k8sClient.Delete(ctx, deployment)).To(Succeed())
				}

				service := &corev1.Service{}
				err = k8sClient.Get(ctx, typeNamespacedName, service)
				if err == nil {
					Expect(k8sClient.Delete(ctx, service)).To(Succeed())
				}

				By("Deleting the Agent")
				err = k8sClient.Get(ctx, typeNamespacedName, agent)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Delete(ctx, agent)).To(Succeed())

				By("Reconciling to process deletion")
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying finalizer was removed despite missing resources")
				// Agent deletion should complete successfully
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agent)
					return errors.IsNotFound(err) || !controllerutil.ContainsFinalizer(agent, agentFinalizer)
				}, timeout, interval).Should(BeTrue())
			})

			It("should handle deletion when DeletionTimestamp is set", func() {
				By("Creating an Agent with finalizer")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:       agentName,
						Namespace:  agentNamespace,
						Finalizers: []string{agentFinalizer},
					},
					Spec: agentv1alpha1.AgentSpec{
						Runtime: agentv1alpha1.RuntimeSpec{
							Container: corev1.Container{
								Image: "nginx:latest",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Deleting the Agent")
				Expect(k8sClient.Delete(ctx, agent)).To(Succeed())

				By("Verifying DeletionTimestamp is set")
				err := k8sClient.Get(ctx, typeNamespacedName, agent)
				Expect(err).NotTo(HaveOccurred())
				Expect(agent.DeletionTimestamp.IsZero()).To(BeFalse())

				By("Reconciling to process deletion")
				controllerReconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Agent is eventually deleted")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agent)
					return errors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			})
		})

		// HIGH PRIORITY TEST: Helper functions
		Context("Helper functions", func() {
			Describe("buildLabels", func() {
				It("should generate correct standard labels", func() {
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-agent",
							Namespace: "default",
						},
					}

					reconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					labels := reconciler.buildLabels(agent)

					Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/name", "test-agent"))
					Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/instance", "test-agent"))
					Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "flokoa-operator"))
					Expect(labels).To(HaveKeyWithValue("flokoa.ai/agent", "test-agent"))
				})

				It("should use Agent name in labels", func() {
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-custom-agent",
							Namespace: "custom-namespace",
						},
					}

					reconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					labels := reconciler.buildLabels(agent)

					Expect(labels["app.kubernetes.io/name"]).To(Equal("my-custom-agent"))
					Expect(labels["flokoa.ai/agent"]).To(Equal("my-custom-agent"))
				})
			})

			Describe("calculatePhase", func() {
				It("should return Running when availableReplicas > 0", func() {
					deployment := &appsv1.Deployment{
						Status: appsv1.DeploymentStatus{
							AvailableReplicas: 3,
						},
					}

					reconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					phase := reconciler.calculatePhase(deployment)
					Expect(phase).To(Equal("Running"))
				})

				It("should return Pending when availableReplicas = 0", func() {
					deployment := &appsv1.Deployment{
						Status: appsv1.DeploymentStatus{
							AvailableReplicas: 0,
						},
					}

					reconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					phase := reconciler.calculatePhase(deployment)
					Expect(phase).To(Equal("Pending"))
				})

				It("should return Running when availableReplicas = 1", func() {
					deployment := &appsv1.Deployment{
						Status: appsv1.DeploymentStatus{
							AvailableReplicas: 1,
						},
					}

					reconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					phase := reconciler.calculatePhase(deployment)
					Expect(phase).To(Equal("Running"))
				})
			})

			Describe("setCondition", func() {
				It("should add new condition to Agent status", func() {
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Generation: 1,
						},
					}

					reconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					reconciler.setCondition(agent, "Ready", metav1.ConditionTrue, "TestReason", "Test message")

					condition := meta.FindStatusCondition(agent.Status.Conditions, "Ready")
					Expect(condition).NotTo(BeNil())
					Expect(condition.Status).To(Equal(metav1.ConditionTrue))
					Expect(condition.Reason).To(Equal("TestReason"))
					Expect(condition.Message).To(Equal("Test message"))
					Expect(condition.ObservedGeneration).To(Equal(int64(1)))
				})

				It("should update existing condition", func() {
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Generation: 1,
						},
						Status: agentv1alpha1.AgentStatus{
							Conditions: []metav1.Condition{
								{
									Type:               "Ready",
									Status:             metav1.ConditionFalse,
									Reason:             "OldReason",
									Message:            "Old message",
									ObservedGeneration: 1,
									LastTransitionTime: metav1.Now(),
								},
							},
						},
					}

					reconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					reconciler.setCondition(agent, "Ready", metav1.ConditionTrue, "NewReason", "New message")

					condition := meta.FindStatusCondition(agent.Status.Conditions, "Ready")
					Expect(condition).NotTo(BeNil())
					Expect(condition.Status).To(Equal(metav1.ConditionTrue))
					Expect(condition.Reason).To(Equal("NewReason"))
					Expect(condition.Message).To(Equal("New message"))
				})

				It("should preserve other conditions when updating one", func() {
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Generation: 1,
						},
						Status: agentv1alpha1.AgentStatus{
							Conditions: []metav1.Condition{
								{
									Type:               "Ready",
									Status:             metav1.ConditionTrue,
									Reason:             "Reason1",
									Message:            "Message1",
									ObservedGeneration: 1,
									LastTransitionTime: metav1.Now(),
								},
								{
									Type:               "Available",
									Status:             metav1.ConditionTrue,
									Reason:             "Reason2",
									Message:            "Message2",
									ObservedGeneration: 1,
									LastTransitionTime: metav1.Now(),
								},
							},
						},
					}

					reconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					reconciler.setCondition(agent, "Ready", metav1.ConditionFalse, "UpdatedReason", "Updated message")

					Expect(agent.Status.Conditions).To(HaveLen(2))
					readyCondition := meta.FindStatusCondition(agent.Status.Conditions, "Ready")
					Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
					Expect(readyCondition.Reason).To(Equal("UpdatedReason"))

					availableCondition := meta.FindStatusCondition(agent.Status.Conditions, "Available")
					Expect(availableCondition).NotTo(BeNil())
					Expect(availableCondition.Reason).To(Equal("Reason2"))
				})
			})
		})

		// HIGH PRIORITY TEST: Spec updates and reconciliation state
		Context("Spec updates and reconciliation state", func() {
			It("should detect and reconcile spec changes", func() {
				By("Creating an Agent")
				replicas := int32(1)
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Runtime: agentv1alpha1.RuntimeSpec{
							Replicas: &replicas,
							Container: corev1.Container{
								Image: "nginx:latest",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				controllerReconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				By("Initial reconciliation")
				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile creates resources
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying initial Deployment")
				deployment := &appsv1.Deployment{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, deployment)
					return err == nil && deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == 1
				}, timeout, interval).Should(BeTrue())

				By("Updating Agent spec")
				err = k8sClient.Get(ctx, typeNamespacedName, agent)
				Expect(err).NotTo(HaveOccurred())
				newReplicas := int32(5)
				agent.Spec.Runtime.Replicas = &newReplicas
				Expect(k8sClient.Update(ctx, agent)).To(Succeed())

				By("Reconciling after spec update")
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Deployment was updated")
				Eventually(func() int32 {
					err := k8sClient.Get(ctx, typeNamespacedName, deployment)
					if err != nil || deployment.Spec.Replicas == nil {
						return 0
					}
					return *deployment.Spec.Replicas
				}, timeout, interval).Should(Equal(int32(5)))

				By("Verifying ObservedGeneration was updated")
				err = k8sClient.Get(ctx, typeNamespacedName, agent)
				Expect(err).NotTo(HaveOccurred())
				Expect(agent.Status.ObservedGeneration).To(Equal(agent.Generation))
			})

			It("should reconcile manually modified Deployments", func() {
				By("Creating an Agent")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Runtime: agentv1alpha1.RuntimeSpec{
							Container: corev1.Container{
								Image: "nginx:latest",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				controllerReconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				By("Initial reconciliation")
				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile creates resources
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Deployment exists")
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, deployment)
				}, timeout, interval).Should(Succeed())

				originalImage := deployment.Spec.Template.Spec.Containers[0].Image
				Expect(originalImage).To(Equal("nginx:latest"))

				By("Manually modifying the Deployment")
				deployment.Spec.Template.Spec.Containers[0].Image = "nginx:alpine"
				Expect(k8sClient.Update(ctx, deployment)).To(Succeed())

				By("Verifying manual modification")
				Eventually(func() string {
					err := k8sClient.Get(ctx, typeNamespacedName, deployment)
					if err != nil {
						return ""
					}
					return deployment.Spec.Template.Spec.Containers[0].Image
				}, timeout, interval).Should(Equal("nginx:alpine"))

				By("Reconciling to restore desired state")
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Deployment restored to original spec")
				Eventually(func() string {
					err := k8sClient.Get(ctx, typeNamespacedName, deployment)
					if err != nil {
						return ""
					}
					return deployment.Spec.Template.Spec.Containers[0].Image
				}, timeout, interval).Should(Equal("nginx:latest"))
			})

			It("should update Service when container ports change", func() {
				By("Creating an Agent with initial ports")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Runtime: agentv1alpha1.RuntimeSpec{
							Container: corev1.Container{
								Image: "nginx:latest",
								Ports: []corev1.ContainerPort{
									{
										Name:          "http",
										ContainerPort: 8080,
										Protocol:      corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				controllerReconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				By("Initial reconciliation")
				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile creates resources
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying initial Service ports")
				service := &corev1.Service{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, service)
					return err == nil && len(service.Spec.Ports) == 1 && service.Spec.Ports[0].Port == 8080
				}, timeout, interval).Should(BeTrue())

				By("Updating Agent with new ports")
				err = k8sClient.Get(ctx, typeNamespacedName, agent)
				Expect(err).NotTo(HaveOccurred())
				agent.Spec.Runtime.Container.Ports = []corev1.ContainerPort{
					{
						Name:          "http",
						ContainerPort: 3000,
						Protocol:      corev1.ProtocolTCP,
					},
					{
						Name:          "metrics",
						ContainerPort: 9090,
						Protocol:      corev1.ProtocolTCP,
					},
				}
				Expect(k8sClient.Update(ctx, agent)).To(Succeed())

				By("Reconciling after port update")
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Service ports updated")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, service)
					if err != nil || len(service.Spec.Ports) != 2 {
						return false
					}
					return service.Spec.Ports[0].Port == 3000 && service.Spec.Ports[1].Port == 9090
				}, timeout, interval).Should(BeTrue())
			})
		})
	})
})
