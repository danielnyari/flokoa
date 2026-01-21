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
							Type: agentv1alpha1.RuntimeTypeStandard,
							Spec: &agentv1alpha1.StandardRuntimeSpec{
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
				Eventually(func() agentv1alpha1.AgentPhase {
					err := k8sClient.Get(ctx, typeNamespacedName, agent)
					if err != nil {
						return ""
					}
					return agent.Status.Phase
				}, timeout, interval).Should(Equal(agentv1alpha1.AgentPhasePending))

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
							Type: agentv1alpha1.RuntimeTypeStandard,
							Spec: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Image: "nginx:latest",
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
							Type: agentv1alpha1.RuntimeTypeStandard,
							Spec: &agentv1alpha1.StandardRuntimeSpec{
								Replicas: &replicas,
								Container: corev1.Container{
									Image: "nginx:latest",
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
							Type: agentv1alpha1.RuntimeTypeStandard,
							Spec: &agentv1alpha1.StandardRuntimeSpec{
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
							Type: agentv1alpha1.RuntimeTypeStandard,
							Spec: &agentv1alpha1.StandardRuntimeSpec{
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
							Type: agentv1alpha1.RuntimeTypeStandard,
							Spec: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Image: "nginx:latest",
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
							Type: agentv1alpha1.RuntimeTypeStandard,
							Spec: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Image: "nginx:latest",
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
							Type: agentv1alpha1.RuntimeTypeStandard,
							Spec: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Image: "nginx:latest",
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
							Type: agentv1alpha1.RuntimeTypeStandard,
							Spec: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Image: "nginx:latest",
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
	})
})
