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
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

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
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

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
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

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
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

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
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

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
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

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
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

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
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

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

		Context("Inline tools", func() {
			It("should create ConfigMap and mount inline tools", func() {
				By("Creating an Agent with inline tools")
				timeout := int32(60)
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
								},
							},
						},
						Tools: []agentv1alpha1.ToolEntry{
							{
								Name: "weather-api",
								Inline: &agentv1alpha1.InlineToolSpec{
									Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
									Description: "Get weather information",
									HTTPApi: &agentv1alpha1.HTTPApiSpec{
										URL:            "https://api.weather.com/v1/weather",
										Method:         agentv1alpha1.HTTPMethodGet,
										TimeoutSeconds: &timeout,
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
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile creates resources
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying ConfigMap was created for inline tool")
				configMapName := fmt.Sprintf("%s-tool-weather-api", agentName)
				cm := &corev1.ConfigMap{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      configMapName,
						Namespace: agentNamespace,
					}, cm)
				}, timeout, interval).Should(Succeed())

				Expect(cm.Data).To(HaveKey("spec.json"))
				Expect(cm.Data["spec.json"]).To(ContainSubstring("weather"))
				Expect(cm.Labels).To(HaveKeyWithValue("flokoa.ai/agent", agentName))
				Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "inline-tool-spec"))

				By("Verifying Deployment has volume mount for the tool")
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, deployment)
				}, timeout, interval).Should(Succeed())

				// Check volumes
				var toolVolume *corev1.Volume
				for i := range deployment.Spec.Template.Spec.Volumes {
					if deployment.Spec.Template.Spec.Volumes[i].Name == "tool-weather-api" {
						toolVolume = &deployment.Spec.Template.Spec.Volumes[i]
						break
					}
				}
				Expect(toolVolume).NotTo(BeNil())
				Expect(toolVolume.ConfigMap.Name).To(Equal(configMapName))

				// Check volume mounts
				container := deployment.Spec.Template.Spec.Containers[0]
				var toolMount *corev1.VolumeMount
				for i := range container.VolumeMounts {
					if container.VolumeMounts[i].Name == "tool-weather-api" {
						toolMount = &container.VolumeMounts[i]
						break
					}
				}
				Expect(toolMount).NotTo(BeNil())
				Expect(toolMount.MountPath).To(Equal("/etc/flokoa/tools/weather-api"))
				Expect(toolMount.ReadOnly).To(BeTrue())

				By("Verifying ToolsReady condition is set")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agent)
					if err != nil {
						return false
					}
					condition := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeToolsReady)
					return condition != nil && condition.Status == metav1.ConditionTrue
				}, timeout, interval).Should(BeTrue())

				By("Verifying LastToolSync is set")
				err = k8sClient.Get(ctx, typeNamespacedName, agent)
				Expect(err).NotTo(HaveOccurred())
				Expect(agent.Status.LastToolSync).NotTo(BeNil())
			})

			It("should create multiple inline tools with unique mounts", func() {
				By("Creating an Agent with multiple inline tools")
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
								},
							},
						},
						Tools: []agentv1alpha1.ToolEntry{
							{
								Name: "tool-one",
								Inline: &agentv1alpha1.InlineToolSpec{
									Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
									Description: "First tool",
									HTTPApi: &agentv1alpha1.HTTPApiSpec{
										URL:    "https://api.example.com/one",
										Method: agentv1alpha1.HTTPMethodGet,
									},
								},
							},
							{
								Name: "tool-two",
								Inline: &agentv1alpha1.InlineToolSpec{
									Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
									Description: "Second tool",
									HTTPApi: &agentv1alpha1.HTTPApiSpec{
										URL:    "https://api.example.com/two",
										Method: agentv1alpha1.HTTPMethodPost,
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
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile creates resources
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying both ConfigMaps were created")
				cm1 := &corev1.ConfigMap{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      fmt.Sprintf("%s-tool-tool-one", agentName),
						Namespace: agentNamespace,
					}, cm1)
				}, timeout, interval).Should(Succeed())

				cm2 := &corev1.ConfigMap{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      fmt.Sprintf("%s-tool-tool-two", agentName),
						Namespace: agentNamespace,
					}, cm2)
				}, timeout, interval).Should(Succeed())

				By("Verifying Deployment has both volume mounts")
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, deployment)
				}, timeout, interval).Should(Succeed())

				container := deployment.Spec.Template.Spec.Containers[0]
				Expect(container.VolumeMounts).To(HaveLen(2))

				// Find both volume mounts
				mountPaths := make([]string, 0, 2)
				for _, vm := range container.VolumeMounts {
					mountPaths = append(mountPaths, vm.MountPath)
				}
				Expect(mountPaths).To(ContainElement("/etc/flokoa/tools/tool-one"))
				Expect(mountPaths).To(ContainElement("/etc/flokoa/tools/tool-two"))
			})
		})

		Context("Tool references", func() {
			var agentToolName string

			BeforeEach(func() {
				agentToolName = fmt.Sprintf("test-agenttool-%d", time.Now().UnixNano())
			})

			AfterEach(func() {
				// Cleanup the AgentTool resource
				agentTool := &agentv1alpha1.AgentTool{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: agentToolName, Namespace: agentNamespace}, agentTool)
				if err == nil {
					// Remove finalizer if present
					if controllerutil.ContainsFinalizer(agentTool, "agent.flokoa.ai/agenttool-finalizer") {
						controllerutil.RemoveFinalizer(agentTool, "agent.flokoa.ai/agenttool-finalizer")
						_ = k8sClient.Update(ctx, agentTool)
					}
					_ = k8sClient.Delete(ctx, agentTool)
				}
			})

			It("should mount referenced AgentTool ConfigMap", func() {
				By("Creating an AgentTool resource")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "External weather API tool",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.weather.com/v2",
							Method: agentv1alpha1.HTTPMethodGet,
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Creating the AgentTool's ConfigMap (simulating AgentTool controller)")
				toolConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-spec", agentToolName),
						Namespace: agentNamespace,
						Labels: map[string]string{
							"app.kubernetes.io/name":      agentToolName,
							"app.kubernetes.io/component": "agenttool-spec",
						},
					},
					Data: map[string]string{
						"spec.json": `{"type":"http-api","description":"External weather API tool"}`,
					},
				}
				Expect(k8sClient.Create(ctx, toolConfigMap)).To(Succeed())

				By("Creating an Agent that references the AgentTool")
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
								},
							},
						},
						Tools: []agentv1alpha1.ToolEntry{
							{
								ToolRef: &agentv1alpha1.ToolRef{
									Name: agentToolName,
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
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile creates resources
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Deployment mounts the referenced AgentTool ConfigMap")
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, deployment)
				}, timeout, interval).Should(Succeed())

				// Check volumes - should reference the AgentTool's ConfigMap
				var toolVolume *corev1.Volume
				for i := range deployment.Spec.Template.Spec.Volumes {
					if deployment.Spec.Template.Spec.Volumes[i].Name == fmt.Sprintf("tool-%s", agentToolName) {
						toolVolume = &deployment.Spec.Template.Spec.Volumes[i]
						break
					}
				}
				Expect(toolVolume).NotTo(BeNil())
				Expect(toolVolume.ConfigMap.Name).To(Equal(fmt.Sprintf("%s-spec", agentToolName)))

				// Check volume mounts
				container := deployment.Spec.Template.Spec.Containers[0]
				var toolMount *corev1.VolumeMount
				for i := range container.VolumeMounts {
					if container.VolumeMounts[i].Name == fmt.Sprintf("tool-%s", agentToolName) {
						toolMount = &container.VolumeMounts[i]
						break
					}
				}
				Expect(toolMount).NotTo(BeNil())
				Expect(toolMount.MountPath).To(Equal(fmt.Sprintf("/etc/flokoa/tools/%s", agentToolName)))
				Expect(toolMount.ReadOnly).To(BeTrue())
			})

			It("should fail when referenced AgentTool does not exist", func() {
				By("Creating an Agent that references a non-existent AgentTool")
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
								},
							},
						},
						Tools: []agentv1alpha1.ToolEntry{
							{
								ToolRef: &agentv1alpha1.ToolRef{
									Name: "non-existent-tool",
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
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile should fail due to missing AgentTool
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("non-existent-tool"))

				By("Verifying ToolsReady condition shows failure")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agent)
					if err != nil {
						return false
					}
					condition := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeToolsReady)
					return condition != nil && condition.Status == metav1.ConditionFalse
				}, timeout, interval).Should(BeTrue())
			})

			It("should use custom name when specified in ToolRef", func() {
				By("Creating an AgentTool resource")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Some API tool",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.example.com",
							Method: agentv1alpha1.HTTPMethodGet,
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Creating the AgentTool's ConfigMap")
				toolConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-spec", agentToolName),
						Namespace: agentNamespace,
					},
					Data: map[string]string{
						"spec.json": `{"type":"http-api"}`,
					},
				}
				Expect(k8sClient.Create(ctx, toolConfigMap)).To(Succeed())

				By("Creating an Agent with a custom name for the tool reference")
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
								},
							},
						},
						Tools: []agentv1alpha1.ToolEntry{
							{
								Name: "my-custom-tool-name",
								ToolRef: &agentv1alpha1.ToolRef{
									Name: agentToolName,
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
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile creates resources
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying volume mount uses custom name")
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, deployment)
				}, timeout, interval).Should(Succeed())

				container := deployment.Spec.Template.Spec.Containers[0]
				var toolMount *corev1.VolumeMount
				for i := range container.VolumeMounts {
					if container.VolumeMounts[i].Name == "tool-my-custom-tool-name" {
						toolMount = &container.VolumeMounts[i]
						break
					}
				}
				Expect(toolMount).NotTo(BeNil())
				Expect(toolMount.MountPath).To(Equal("/etc/flokoa/tools/my-custom-tool-name"))
			})
		})

		Context("Mixed inline and referenced tools", func() {
			var agentToolName string

			BeforeEach(func() {
				agentToolName = fmt.Sprintf("test-agenttool-%d", time.Now().UnixNano())
			})

			AfterEach(func() {
				// Cleanup the AgentTool resource
				agentTool := &agentv1alpha1.AgentTool{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: agentToolName, Namespace: agentNamespace}, agentTool)
				if err == nil {
					if controllerutil.ContainsFinalizer(agentTool, "agent.flokoa.ai/agenttool-finalizer") {
						controllerutil.RemoveFinalizer(agentTool, "agent.flokoa.ai/agenttool-finalizer")
						_ = k8sClient.Update(ctx, agentTool)
					}
					_ = k8sClient.Delete(ctx, agentTool)
				}
			})

			It("should handle both inline and referenced tools together", func() {
				By("Creating an AgentTool resource")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Referenced tool",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.example.com/ref",
							Method: agentv1alpha1.HTTPMethodGet,
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Creating the AgentTool's ConfigMap")
				toolConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-spec", agentToolName),
						Namespace: agentNamespace,
					},
					Data: map[string]string{
						"spec.json": `{"type":"http-api"}`,
					},
				}
				Expect(k8sClient.Create(ctx, toolConfigMap)).To(Succeed())

				By("Creating an Agent with both inline and referenced tools")
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
								},
							},
						},
						Tools: []agentv1alpha1.ToolEntry{
							{
								Name: "inline-tool",
								Inline: &agentv1alpha1.InlineToolSpec{
									Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
									Description: "Inline tool",
									HTTPApi: &agentv1alpha1.HTTPApiSpec{
										URL:    "https://api.example.com/inline",
										Method: agentv1alpha1.HTTPMethodPost,
									},
								},
							},
							{
								ToolRef: &agentv1alpha1.ToolRef{
									Name: agentToolName,
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
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile creates resources
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying ConfigMap was created for inline tool")
				inlineConfigMap := &corev1.ConfigMap{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      fmt.Sprintf("%s-tool-inline-tool", agentName),
						Namespace: agentNamespace,
					}, inlineConfigMap)
				}, timeout, interval).Should(Succeed())

				By("Verifying Deployment has both volume mounts")
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, deployment)
				}, timeout, interval).Should(Succeed())

				container := deployment.Spec.Template.Spec.Containers[0]
				Expect(container.VolumeMounts).To(HaveLen(2))

				mountPaths := make([]string, 0, 2)
				for _, vm := range container.VolumeMounts {
					mountPaths = append(mountPaths, vm.MountPath)
				}
				Expect(mountPaths).To(ContainElement("/etc/flokoa/tools/inline-tool"))
				Expect(mountPaths).To(ContainElement(fmt.Sprintf("/etc/flokoa/tools/%s", agentToolName)))

				By("Verifying ToolsReady condition shows 2 tools synced")
				err = k8sClient.Get(ctx, typeNamespacedName, agent)
				Expect(err).NotTo(HaveOccurred())
				condition := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeToolsReady)
				Expect(condition).NotTo(BeNil())
				Expect(condition.Message).To(ContainSubstring("2 tools"))
			})
		})
	})
})
