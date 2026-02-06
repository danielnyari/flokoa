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
	"encoding/json"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// minimalCard creates a minimal valid AgentCard for testing
func minimalCard() agentv1alpha1.AgentCard {
	return agentv1alpha1.AgentCard{
		Name:        "Test Agent",
		Description: "A test agent",
		Version:     "1.0.0",
		Skills: []agentv1alpha1.AgentSkill{
			{
				ID:          "test-skill",
				Name:        "Test Skill",
				Description: "A test skill",
				Tags:        []string{"test"},
			},
		},
	}
}

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
						Card: minimalCard(),
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
						Card: minimalCard(),
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
						Card: minimalCard(),
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
						Card: minimalCard(),
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
						Card: minimalCard(),
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
						Card: minimalCard(),
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
						Card: minimalCard(),
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
						Card: minimalCard(),
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
						Card: minimalCard(),
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
			AfterEach(func() {
				// Cleanup any AgentTools created for inline tools
				agentToolList := &agentv1alpha1.AgentToolList{}
				_ = k8sClient.List(ctx, agentToolList, client.InNamespace(agentNamespace))
				for _, at := range agentToolList.Items {
					if at.Labels["flokoa.ai/agent"] == agentName {
						if controllerutil.ContainsFinalizer(&at, "agent.flokoa.ai/agenttool-finalizer") {
							controllerutil.RemoveFinalizer(&at, "agent.flokoa.ai/agenttool-finalizer")
							_ = k8sClient.Update(ctx, &at)
						}
						_ = k8sClient.Delete(ctx, &at)
					}
				}
			})

			It("should create AgentTool CR and mount inline tools", func() {
				By("Creating an Agent with inline tools")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Card: minimalCard(),
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
								Inline: &agentv1alpha1.AgentToolSpec{
									Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
									Description: "Get weather information",
									HTTPApi: &agentv1alpha1.HTTPApiSpec{
										URL:    "https://api.weather.com/v1/weather",
										Method: agentv1alpha1.HTTPMethodGet,
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

				// Second reconcile creates AgentTool CR
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying AgentTool CR was created for inline tool")
				agentToolName := fmt.Sprintf("%s-weather-api", agentName)
				agentTool := &agentv1alpha1.AgentTool{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      agentToolName,
						Namespace: agentNamespace,
					}, agentTool)
				}, timeout, interval).Should(Succeed())

				Expect(agentTool.Spec.Type).To(Equal(agentv1alpha1.AgentToolTypeHTTPAPI))
				Expect(agentTool.Spec.Description).To(Equal("Get weather information"))
				Expect(agentTool.Labels).To(HaveKeyWithValue("flokoa.ai/agent", agentName))
				Expect(agentTool.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "inline-tool"))

				By("Simulating AgentTool controller creating ConfigMap")
				configMapName := fmt.Sprintf("%s-spec", agentToolName)
				toolConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName,
						Namespace: agentNamespace,
					},
					Data: map[string]string{
						"spec.json": `{"type":"http-api","description":"Get weather information"}`,
					},
				}
				Expect(k8sClient.Create(ctx, toolConfigMap)).To(Succeed())

				By("Reconciling again to create deployment with volume mounts")
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

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

			It("should create multiple AgentTool CRs with unique mounts", func() {
				By("Creating an Agent with multiple inline tools")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Card: minimalCard(),
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
								Inline: &agentv1alpha1.AgentToolSpec{
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
								Inline: &agentv1alpha1.AgentToolSpec{
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

				// Second reconcile creates AgentTool CRs
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying both AgentTool CRs were created")
				at1 := &agentv1alpha1.AgentTool{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      fmt.Sprintf("%s-tool-one", agentName),
						Namespace: agentNamespace,
					}, at1)
				}, timeout, interval).Should(Succeed())

				at2 := &agentv1alpha1.AgentTool{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      fmt.Sprintf("%s-tool-two", agentName),
						Namespace: agentNamespace,
					}, at2)
				}, timeout, interval).Should(Succeed())

				By("Simulating AgentTool controller creating ConfigMaps")
				cm1 := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-tool-one-spec", agentName),
						Namespace: agentNamespace,
					},
					Data: map[string]string{"spec.json": `{"type":"http-api"}`},
				}
				Expect(k8sClient.Create(ctx, cm1)).To(Succeed())

				cm2 := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-tool-two-spec", agentName),
						Namespace: agentNamespace,
					},
					Data: map[string]string{"spec.json": `{"type":"http-api"}`},
				}
				Expect(k8sClient.Create(ctx, cm2)).To(Succeed())

				By("Reconciling again to create deployment with volume mounts")
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Deployment has both volume mounts")
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, deployment)
				}, timeout, interval).Should(Succeed())

				container := deployment.Spec.Template.Spec.Containers[0]
				Expect(container.VolumeMounts).To(HaveLen(3)) // 2 tools + 1 agent-card

				// Find both volume mounts
				mountPaths := make([]string, 0, 3)
				for _, vm := range container.VolumeMounts {
					mountPaths = append(mountPaths, vm.MountPath)
				}
				Expect(mountPaths).To(ContainElement("/etc/flokoa/tools/tool-one"))
				Expect(mountPaths).To(ContainElement("/etc/flokoa/tools/tool-two"))
				Expect(mountPaths).To(ContainElement("/etc/flokoa/agent-card.json"))
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
						Card: minimalCard(),
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
						Card: minimalCard(),
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
						Card: minimalCard(),
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
				// Cleanup all AgentTool resources (both referenced and inline-created)
				agentToolList := &agentv1alpha1.AgentToolList{}
				_ = k8sClient.List(ctx, agentToolList, client.InNamespace(agentNamespace))
				for _, at := range agentToolList.Items {
					if controllerutil.ContainsFinalizer(&at, "agent.flokoa.ai/agenttool-finalizer") {
						controllerutil.RemoveFinalizer(&at, "agent.flokoa.ai/agenttool-finalizer")
						_ = k8sClient.Update(ctx, &at)
					}
					_ = k8sClient.Delete(ctx, &at)
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
						Card: minimalCard(),
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
								Inline: &agentv1alpha1.AgentToolSpec{
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

				// Second reconcile creates AgentTool CR for inline tool
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying AgentTool CR was created for inline tool")
				inlineAgentToolName := fmt.Sprintf("%s-inline-tool", agentName)
				inlineAgentTool := &agentv1alpha1.AgentTool{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      inlineAgentToolName,
						Namespace: agentNamespace,
					}, inlineAgentTool)
				}, timeout, interval).Should(Succeed())

				By("Simulating AgentTool controller creating ConfigMap for inline tool")
				inlineConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-spec", inlineAgentToolName),
						Namespace: agentNamespace,
					},
					Data: map[string]string{"spec.json": `{"type":"http-api"}`},
				}
				Expect(k8sClient.Create(ctx, inlineConfigMap)).To(Succeed())

				By("Reconciling again to create deployment with volume mounts")
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Deployment has both volume mounts")
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, deployment)
				}, timeout, interval).Should(Succeed())

				container := deployment.Spec.Template.Spec.Containers[0]
				Expect(container.VolumeMounts).To(HaveLen(3)) // 2 tools + 1 agent-card

				mountPaths := make([]string, 0, 3)
				for _, vm := range container.VolumeMounts {
					mountPaths = append(mountPaths, vm.MountPath)
				}
				Expect(mountPaths).To(ContainElement("/etc/flokoa/tools/inline-tool"))
				Expect(mountPaths).To(ContainElement(fmt.Sprintf("/etc/flokoa/tools/%s", agentToolName)))
				Expect(mountPaths).To(ContainElement("/etc/flokoa/agent-card.json"))

				By("Verifying ToolsReady condition shows 2 tools synced")
				err = k8sClient.Get(ctx, typeNamespacedName, agent)
				Expect(err).NotTo(HaveOccurred())
				condition := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeToolsReady)
				Expect(condition).NotTo(BeNil())
				Expect(condition.Message).To(ContainSubstring("2 tools"))
			})
		})

		Context("Tool change propagation", func() {
			var agentToolName string

			BeforeEach(func() {
				agentToolName = fmt.Sprintf("test-agenttool-%d", time.Now().UnixNano())
			})

			AfterEach(func() {
				// Cleanup all AgentTool resources
				agentToolList := &agentv1alpha1.AgentToolList{}
				_ = k8sClient.List(ctx, agentToolList, client.InNamespace(agentNamespace))
				for _, at := range agentToolList.Items {
					if controllerutil.ContainsFinalizer(&at, "agent.flokoa.ai/agenttool-finalizer") {
						controllerutil.RemoveFinalizer(&at, "agent.flokoa.ai/agenttool-finalizer")
						_ = k8sClient.Update(ctx, &at)
					}
					_ = k8sClient.Delete(ctx, &at)
				}
			})

			It("should add tools-hash annotation to pod template", func() {
				By("Creating an AgentTool resource")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Test tool",
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
						Labels: map[string]string{
							"app.kubernetes.io/name":      agentToolName,
							"app.kubernetes.io/component": "agenttool-spec",
						},
					},
					Data: map[string]string{
						"spec.json": `{"type":"http-api","description":"Test tool"}`,
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
						Card: minimalCard(),
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

				By("Verifying pod template has tools-hash annotation")
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, deployment)
				}, timeout, interval).Should(Succeed())

				Expect(deployment.Spec.Template.Annotations).To(HaveKey("flokoa.ai/tools-hash"))
				initialHash := deployment.Spec.Template.Annotations["flokoa.ai/tools-hash"]
				Expect(initialHash).NotTo(BeEmpty())
			})

			It("should update tools-hash when ConfigMap changes", func() {
				By("Creating an AgentTool resource")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Original description",
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
						Labels: map[string]string{
							"app.kubernetes.io/name":      agentToolName,
							"app.kubernetes.io/component": "agenttool-spec",
						},
					},
					Data: map[string]string{
						"spec.json": `{"type":"http-api","description":"Original description"}`,
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
						Card: minimalCard(),
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

				By("Getting the initial tools-hash")
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, deployment)
				}, timeout, interval).Should(Succeed())

				initialHash := deployment.Spec.Template.Annotations["flokoa.ai/tools-hash"]
				Expect(initialHash).NotTo(BeEmpty())

				By("Updating the ConfigMap content")
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("%s-spec", agentToolName),
					Namespace: agentNamespace,
				}, toolConfigMap)
				Expect(err).NotTo(HaveOccurred())

				toolConfigMap.Data["spec.json"] = `{"type":"http-api","description":"Updated description"}`
				Expect(k8sClient.Update(ctx, toolConfigMap)).To(Succeed())

				By("Reconciling the Agent again")
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying the tools-hash has changed")
				err = k8sClient.Get(ctx, typeNamespacedName, deployment)
				Expect(err).NotTo(HaveOccurred())

				updatedHash := deployment.Spec.Template.Annotations["flokoa.ai/tools-hash"]
				Expect(updatedHash).NotTo(BeEmpty())
				Expect(updatedHash).NotTo(Equal(initialHash))
			})

			It("should not have tools-hash annotation when no tools are configured", func() {
				By("Creating an Agent without tools")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Card: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Spec: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Name:  "agent",
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

				By("Verifying pod template has no tools-hash annotation")
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, deployment)
				}, timeout, interval).Should(Succeed())

				Expect(deployment.Spec.Template.Annotations).To(BeNil())
			})

			It("should produce same hash for same ConfigMap data", func() {
				By("Creating two ConfigMaps with identical data")
				data1 := map[string]string{
					"spec.json": `{"type":"http-api","description":"Test"}`,
					"other.txt": "some content",
				}
				data2 := map[string]string{
					"other.txt": "some content",
					"spec.json": `{"type":"http-api","description":"Test"}`,
				}

				hash1 := hashConfigMapData(data1)
				hash2 := hashConfigMapData(data2)

				Expect(hash1).To(Equal(hash2))
				Expect(hash1).NotTo(BeEmpty())
			})

			It("should produce different hash for different ConfigMap data", func() {
				By("Creating two ConfigMaps with different data")
				data1 := map[string]string{
					"spec.json": `{"type":"http-api","description":"Original"}`,
				}
				data2 := map[string]string{
					"spec.json": `{"type":"http-api","description":"Modified"}`,
				}

				hash1 := hashConfigMapData(data1)
				hash2 := hashConfigMapData(data2)

				Expect(hash1).NotTo(Equal(hash2))
			})

			It("should return empty hash for empty ConfigMap data", func() {
				hash := hashConfigMapData(map[string]string{})
				Expect(hash).To(BeEmpty())

				hash = hashConfigMapData(nil)
				Expect(hash).To(BeEmpty())
			})
		})

		Context("findAgentsForAgentTool handler", func() {
			var agentToolName string
			var agent1Name string
			var agent2Name string

			BeforeEach(func() {
				agentToolName = fmt.Sprintf("shared-tool-%d", time.Now().UnixNano())
				agent1Name = fmt.Sprintf("agent1-%d", time.Now().UnixNano())
				agent2Name = fmt.Sprintf("agent2-%d", time.Now().UnixNano())
			})

			AfterEach(func() {
				// Cleanup agents
				for _, name := range []string{agent1Name, agent2Name} {
					agent := &agentv1alpha1.Agent{}
					err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: agentNamespace}, agent)
					if err == nil {
						if controllerutil.ContainsFinalizer(agent, agentFinalizer) {
							controllerutil.RemoveFinalizer(agent, agentFinalizer)
							_ = k8sClient.Update(ctx, agent)
						}
						_ = k8sClient.Delete(ctx, agent)
					}
				}

				// Cleanup AgentTools
				agentToolList := &agentv1alpha1.AgentToolList{}
				_ = k8sClient.List(ctx, agentToolList, client.InNamespace(agentNamespace))
				for _, at := range agentToolList.Items {
					if controllerutil.ContainsFinalizer(&at, "agent.flokoa.ai/agenttool-finalizer") {
						controllerutil.RemoveFinalizer(&at, "agent.flokoa.ai/agenttool-finalizer")
						_ = k8sClient.Update(ctx, &at)
					}
					_ = k8sClient.Delete(ctx, &at)
				}
			})

			It("should find all agents referencing an AgentTool", func() {
				By("Creating a shared AgentTool")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Shared tool",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.example.com",
							Method: agentv1alpha1.HTTPMethodGet,
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Creating two agents that reference the same tool")
				for _, name := range []string{agent1Name, agent2Name} {
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
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
				}

				By("Calling findAgentsForAgentTool")
				reconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				requests := reconciler.findAgentsForAgentTool(ctx, agentTool)

				By("Verifying both agents are returned")
				Expect(requests).To(HaveLen(2))

				names := []string{requests[0].Name, requests[1].Name}
				Expect(names).To(ContainElement(agent1Name))
				Expect(names).To(ContainElement(agent2Name))
			})

			It("should find agent for inline tool via label", func() {
				By("Creating an inline AgentTool with agent label")
				inlineAgentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-inline-tool", agent1Name),
						Namespace: agentNamespace,
						Labels: map[string]string{
							"flokoa.ai/agent": agent1Name,
						},
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Inline tool",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.example.com",
							Method: agentv1alpha1.HTTPMethodGet,
						},
					},
				}
				Expect(k8sClient.Create(ctx, inlineAgentTool)).To(Succeed())

				By("Calling findAgentsForAgentTool")
				reconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				requests := reconciler.findAgentsForAgentTool(ctx, inlineAgentTool)

				By("Verifying the owning agent is returned")
				Expect(requests).To(HaveLen(1))
				Expect(requests[0].Name).To(Equal(agent1Name))
			})

			It("should return empty for unreferenced AgentTool", func() {
				By("Creating an AgentTool that no agent references")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Unreferenced tool",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.example.com",
							Method: agentv1alpha1.HTTPMethodGet,
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Calling findAgentsForAgentTool")
				reconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				requests := reconciler.findAgentsForAgentTool(ctx, agentTool)

				By("Verifying no agents are returned")
				Expect(requests).To(BeEmpty())
			})
		})

		Context("findAgentsForConfigMap handler", func() {
			var agentToolName string

			BeforeEach(func() {
				agentToolName = fmt.Sprintf("cm-tool-%d", time.Now().UnixNano())
			})

			AfterEach(func() {
				// Cleanup AgentTools
				agentToolList := &agentv1alpha1.AgentToolList{}
				_ = k8sClient.List(ctx, agentToolList, client.InNamespace(agentNamespace))
				for _, at := range agentToolList.Items {
					if controllerutil.ContainsFinalizer(&at, "agent.flokoa.ai/agenttool-finalizer") {
						controllerutil.RemoveFinalizer(&at, "agent.flokoa.ai/agenttool-finalizer")
						_ = k8sClient.Update(ctx, &at)
					}
					_ = k8sClient.Delete(ctx, &at)
				}
			})

			It("should find agent for agenttool-spec ConfigMap", func() {
				By("Creating an AgentTool")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Test tool",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.example.com",
							Method: agentv1alpha1.HTTPMethodGet,
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Creating the AgentTool's ConfigMap with proper labels")
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
						"spec.json": `{"type":"http-api"}`,
					},
				}
				Expect(k8sClient.Create(ctx, toolConfigMap)).To(Succeed())

				By("Creating an Agent that references the tool")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Card: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Spec: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
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

				By("Calling findAgentsForConfigMap")
				reconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				requests := reconciler.findAgentsForConfigMap(ctx, toolConfigMap)

				By("Verifying the agent is returned")
				Expect(requests).To(HaveLen(1))
				Expect(requests[0].Name).To(Equal(agentName))
			})

			It("should find agent for inline-tool-spec ConfigMap via label", func() {
				By("Creating an inline tool ConfigMap with agent label")
				inlineConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-inline-tool-spec", agentName),
						Namespace: agentNamespace,
						Labels: map[string]string{
							"app.kubernetes.io/component": "inline-tool-spec",
							"flokoa.ai/agent":             agentName,
						},
					},
					Data: map[string]string{
						"spec.json": `{"type":"http-api"}`,
					},
				}
				Expect(k8sClient.Create(ctx, inlineConfigMap)).To(Succeed())

				By("Calling findAgentsForConfigMap")
				reconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				requests := reconciler.findAgentsForConfigMap(ctx, inlineConfigMap)

				By("Verifying the agent is returned")
				Expect(requests).To(HaveLen(1))
				Expect(requests[0].Name).To(Equal(agentName))
			})

			It("should ignore ConfigMaps without tool component label", func() {
				By("Creating a ConfigMap without tool labels")
				regularConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "regular-configmap",
						Namespace: agentNamespace,
					},
					Data: map[string]string{
						"config": "some value",
					},
				}
				Expect(k8sClient.Create(ctx, regularConfigMap)).To(Succeed())

				By("Calling findAgentsForConfigMap")
				reconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				requests := reconciler.findAgentsForConfigMap(ctx, regularConfigMap)

				By("Verifying no agents are returned")
				Expect(requests).To(BeEmpty())
			})
		})

		Context("Multiple agents with shared tool", func() {
			var agentToolName string
			var agent1Name string
			var agent2Name string

			BeforeEach(func() {
				agentToolName = fmt.Sprintf("shared-tool-%d", time.Now().UnixNano())
				agent1Name = fmt.Sprintf("agent1-%d", time.Now().UnixNano())
				agent2Name = fmt.Sprintf("agent2-%d", time.Now().UnixNano())
			})

			AfterEach(func() {
				// Cleanup agents
				for _, name := range []string{agent1Name, agent2Name} {
					agent := &agentv1alpha1.Agent{}
					err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: agentNamespace}, agent)
					if err == nil {
						if controllerutil.ContainsFinalizer(agent, agentFinalizer) {
							controllerutil.RemoveFinalizer(agent, agentFinalizer)
							_ = k8sClient.Update(ctx, agent)
						}
						_ = k8sClient.Delete(ctx, agent)
					}
				}

				// Cleanup AgentTools
				agentToolList := &agentv1alpha1.AgentToolList{}
				_ = k8sClient.List(ctx, agentToolList, client.InNamespace(agentNamespace))
				for _, at := range agentToolList.Items {
					if controllerutil.ContainsFinalizer(&at, "agent.flokoa.ai/agenttool-finalizer") {
						controllerutil.RemoveFinalizer(&at, "agent.flokoa.ai/agenttool-finalizer")
						_ = k8sClient.Update(ctx, &at)
					}
					_ = k8sClient.Delete(ctx, &at)
				}
			})

			It("should update all agents when shared tool ConfigMap changes", func() {
				By("Creating a shared AgentTool")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Shared tool",
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
						Labels: map[string]string{
							"app.kubernetes.io/name":      agentToolName,
							"app.kubernetes.io/component": "agenttool-spec",
						},
					},
					Data: map[string]string{
						"spec.json": `{"type":"http-api","description":"Original"}`,
					},
				}
				Expect(k8sClient.Create(ctx, toolConfigMap)).To(Succeed())

				By("Creating two agents referencing the shared tool")
				controllerReconciler := &AgentReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				initialHashes := make(map[string]string)

				for _, name := range []string{agent1Name, agent2Name} {
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
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

					// Reconcile to add finalizer
					_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: name, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					// Reconcile to create resources
					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: name, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					// Get initial hash
					deployment := &appsv1.Deployment{}
					err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: agentNamespace}, deployment)
					Expect(err).NotTo(HaveOccurred())
					initialHashes[name] = deployment.Spec.Template.Annotations["flokoa.ai/tools-hash"]
				}

				By("Updating the shared ConfigMap")
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("%s-spec", agentToolName),
					Namespace: agentNamespace,
				}, toolConfigMap)
				Expect(err).NotTo(HaveOccurred())

				toolConfigMap.Data["spec.json"] = `{"type":"http-api","description":"Updated"}`
				Expect(k8sClient.Update(ctx, toolConfigMap)).To(Succeed())

				By("Reconciling both agents")
				for _, name := range []string{agent1Name, agent2Name} {
					_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: name, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())
				}

				By("Verifying both agents have updated hashes")
				for _, name := range []string{agent1Name, agent2Name} {
					deployment := &appsv1.Deployment{}
					err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: agentNamespace}, deployment)
					Expect(err).NotTo(HaveOccurred())

					newHash := deployment.Spec.Template.Annotations["flokoa.ai/tools-hash"]
					Expect(newHash).NotTo(Equal(initialHashes[name]))
				}
			})
		})

		Context("AgentCard ConfigMap", func() {
			It("should create AgentCard ConfigMap with correct JSON data", func() {
				By("Creating an Agent with a Card")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Card: agentv1alpha1.AgentCard{
							Name:               "Test Agent",
							Description:        "A test agent for unit testing",
							Version:            "1.0.0",
							DefaultInputModes:  []agentv1alpha1.InputOutputMode{agentv1alpha1.InputOutputModeJSON},
							DefaultOutputModes: []agentv1alpha1.InputOutputMode{agentv1alpha1.InputOutputModeText},
							Capabilities: agentv1alpha1.AgentCapabilities{
								Streaming: true,
							},
							Skills: []agentv1alpha1.AgentSkill{
								{
									ID:          "skill-1",
									Name:        "Test Skill",
									Description: "A test skill",
									Tags:        []string{"test", "demo"},
									Examples:    []string{"example 1", "example 2"},
								},
							},
						},
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Spec: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Name:  "agent",
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

				By("Verifying the AgentCard ConfigMap was created")
				configMapName := fmt.Sprintf("%s-agent-card", agentName)
				configMap := &corev1.ConfigMap{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      configMapName,
						Namespace: agentNamespace,
					}, configMap)
				}, timeout, interval).Should(Succeed())

				By("Verifying ConfigMap labels")
				Expect(configMap.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", agentName))
				Expect(configMap.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "agent-card"))
				Expect(configMap.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "flokoa-operator"))
				Expect(configMap.Labels).To(HaveKeyWithValue("flokoa.ai/agent", agentName))

				By("Verifying ConfigMap contains valid JSON")
				Expect(configMap.Data).To(HaveKey("agent-card.json"))
				cardJSON := configMap.Data["agent-card.json"]
				Expect(cardJSON).NotTo(BeEmpty())

				By("Verifying JSON content matches AgentCard")
				var card agentv1alpha1.AgentCard
				err = json.Unmarshal([]byte(cardJSON), &card)
				Expect(err).NotTo(HaveOccurred())

				Expect(card.Name).To(Equal("Test Agent"))
				Expect(card.Description).To(Equal("A test agent for unit testing"))
				Expect(card.Version).To(Equal("1.0.0"))
				Expect(card.DefaultInputModes).To(ContainElement(agentv1alpha1.InputOutputModeJSON))
				Expect(card.DefaultOutputModes).To(ContainElement(agentv1alpha1.InputOutputModeText))
				Expect(card.Capabilities.Streaming).To(BeTrue())
				Expect(card.Skills).To(HaveLen(1))
				Expect(card.Skills[0].ID).To(Equal("skill-1"))
				Expect(card.Skills[0].Name).To(Equal("Test Skill"))
			})

			It("should mount AgentCard ConfigMap in Deployment", func() {
				By("Creating an Agent with a Card")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Card: agentv1alpha1.AgentCard{
							Name:        "Mount Test Agent",
							Description: "Testing volume mount",
							Version:     "1.0.0",
							Skills:      []agentv1alpha1.AgentSkill{{ID: "test", Name: "Test", Description: "Test skill", Tags: []string{"test"}}},
						},
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Spec: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Name:  "agent",
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

				By("Verifying Deployment has agent-card volume")
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, deployment)
				}, timeout, interval).Should(Succeed())

				var agentCardVolume *corev1.Volume
				for i := range deployment.Spec.Template.Spec.Volumes {
					if deployment.Spec.Template.Spec.Volumes[i].Name == "agent-card" {
						agentCardVolume = &deployment.Spec.Template.Spec.Volumes[i]
						break
					}
				}
				Expect(agentCardVolume).NotTo(BeNil())
				Expect(agentCardVolume.ConfigMap.Name).To(Equal(fmt.Sprintf("%s-agent-card", agentName)))

				By("Verifying container has agent-card volume mount")
				container := deployment.Spec.Template.Spec.Containers[0]
				var agentCardMount *corev1.VolumeMount
				for i := range container.VolumeMounts {
					if container.VolumeMounts[i].Name == "agent-card" {
						agentCardMount = &container.VolumeMounts[i]
						break
					}
				}
				Expect(agentCardMount).NotTo(BeNil())
				Expect(agentCardMount.MountPath).To(Equal("/etc/flokoa/agent-card.json"))
				Expect(agentCardMount.SubPath).To(Equal("agent-card.json"))
				Expect(agentCardMount.ReadOnly).To(BeTrue())
			})

			It("should inject FLOKOA_AGENT_URL environment variable", func() {
				By("Creating an Agent")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Card: agentv1alpha1.AgentCard{
							Name:        "URL Test Agent",
							Description: "Testing URL injection",
							Version:     "1.0.0",
							Skills:      []agentv1alpha1.AgentSkill{{ID: "test", Name: "Test", Description: "Test skill", Tags: []string{"test"}}},
						},
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Spec: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Name:  "agent",
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

				By("Verifying container has FLOKOA_AGENT_URL env var")
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, deployment)
				}, timeout, interval).Should(Succeed())

				container := deployment.Spec.Template.Spec.Containers[0]
				var agentURLEnv *corev1.EnvVar
				for i := range container.Env {
					if container.Env[i].Name == "FLOKOA_AGENT_URL" {
						agentURLEnv = &container.Env[i]
						break
					}
				}
				Expect(agentURLEnv).NotTo(BeNil())
				expectedURL := fmt.Sprintf("http://%s.%s.svc.cluster.local", agentName, agentNamespace)
				Expect(agentURLEnv.Value).To(Equal(expectedURL))
			})

			It("should update AgentCard ConfigMap when Agent spec changes", func() {
				By("Creating an Agent with initial Card")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Card: agentv1alpha1.AgentCard{
							Name:        "Original Name",
							Description: "Original description",
							Version:     "1.0.0",
							Skills:      []agentv1alpha1.AgentSkill{{ID: "test", Name: "Test", Description: "Test skill", Tags: []string{"test"}}},
						},
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Spec: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Name:  "agent",
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

				By("Verifying initial ConfigMap content")
				configMapName := fmt.Sprintf("%s-agent-card", agentName)
				configMap := &corev1.ConfigMap{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      configMapName,
						Namespace: agentNamespace,
					}, configMap)
				}, timeout, interval).Should(Succeed())

				var initialCard agentv1alpha1.AgentCard
				err = json.Unmarshal([]byte(configMap.Data["agent-card.json"]), &initialCard)
				Expect(err).NotTo(HaveOccurred())
				Expect(initialCard.Name).To(Equal("Original Name"))

				By("Updating the Agent Card")
				err = k8sClient.Get(ctx, typeNamespacedName, agent)
				Expect(err).NotTo(HaveOccurred())

				agent.Spec.Card.Name = "Updated Name"
				agent.Spec.Card.Description = "Updated description"
				agent.Spec.Card.Version = "2.0.0"
				Expect(k8sClient.Update(ctx, agent)).To(Succeed())

				By("Reconciling again")
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying ConfigMap was updated")
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentNamespace,
				}, configMap)
				Expect(err).NotTo(HaveOccurred())

				var updatedCard agentv1alpha1.AgentCard
				err = json.Unmarshal([]byte(configMap.Data["agent-card.json"]), &updatedCard)
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedCard.Name).To(Equal("Updated Name"))
				Expect(updatedCard.Description).To(Equal("Updated description"))
				Expect(updatedCard.Version).To(Equal("2.0.0"))
			})

			It("should preserve user-defined env vars when adding FLOKOA_AGENT_URL", func() {
				By("Creating an Agent with existing env vars")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						Card: agentv1alpha1.AgentCard{
							Name:        "Env Test Agent",
							Description: "Testing env preservation",
							Version:     "1.0.0",
							Skills:      []agentv1alpha1.AgentSkill{{ID: "test", Name: "Test", Description: "Test skill", Tags: []string{"test"}}},
						},
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Spec: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Name:  "agent",
									Image: "nginx:latest",
									Env: []corev1.EnvVar{
										{Name: "MY_VAR", Value: "my-value"},
										{Name: "ANOTHER_VAR", Value: "another-value"},
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

				By("Verifying all env vars are present")
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, typeNamespacedName, deployment)
				}, timeout, interval).Should(Succeed())

				container := deployment.Spec.Template.Spec.Containers[0]
				envNames := make([]string, 0, len(container.Env))
				for _, env := range container.Env {
					envNames = append(envNames, env.Name)
				}

				Expect(envNames).To(ContainElement("MY_VAR"))
				Expect(envNames).To(ContainElement("ANOTHER_VAR"))
				Expect(envNames).To(ContainElement("FLOKOA_AGENT_URL"))
			})
		})

		Context("Model reconciliation", func() {
			var (
				providerName string
				modelName    string
			)

			BeforeEach(func() {
				// Use unique names per test
				providerName = fmt.Sprintf("test-provider-%d", time.Now().UnixNano())
				modelName = fmt.Sprintf("test-model-%d", time.Now().UnixNano())
			})

			AfterEach(func() {
				// Cleanup Model resources
				model := &agentv1alpha1.Model{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: modelName, Namespace: agentNamespace}, model)
				if err == nil {
					_ = k8sClient.Delete(ctx, model)
				}

				// Cleanup ModelProvider resources
				provider := &agentv1alpha1.ModelProvider{}
				err = k8sClient.Get(ctx, types.NamespacedName{Name: providerName, Namespace: agentNamespace}, provider)
				if err == nil {
					_ = k8sClient.Delete(ctx, provider)
				}
			})

			Context("Model reference", func() {
				It("should reconcile agent with Model reference", func() {
					By("Creating a ModelProvider resource")
					provider := &agentv1alpha1.ModelProvider{
						ObjectMeta: metav1.ObjectMeta{
							Name:      providerName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelProviderSpec{
							APIKeySecretRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "openai-credentials",
								},
								Key: "api-key",
							},
							OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
						},
					}
					Expect(k8sClient.Create(ctx, provider)).To(Succeed())

					// Reconcile the provider to set its status
					providerReconciler := &ModelProviderReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err := providerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: providerName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating a Model resource")
					model := &agentv1alpha1.Model{
						ObjectMeta: metav1.ObjectMeta{
							Name:      modelName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelSpec{
							Model: "gpt-4o",
							ProviderRef: agentv1alpha1.ProviderRef{
								Name: providerName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, model)).To(Succeed())

					// Reconcile the model to set its status
					modelReconciler := &ModelReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err = modelReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: modelName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating an Agent with Model reference")
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      agentName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Name:  "agent",
										Image: "nginx:latest",
									},
								},
							},
							Model: &agentv1alpha1.AgentModelRef{
								Name: modelName,
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

					By("Verifying Model ConfigMap was created")
					modelConfigMapName := fmt.Sprintf("%s-model", agentName)
					modelCM := &corev1.ConfigMap{}
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{
							Name:      modelConfigMapName,
							Namespace: agentNamespace,
						}, modelCM)
					}, timeout, interval).Should(Succeed())

					Expect(modelCM.Data).To(HaveKey("model.json"))

					By("Verifying ResolvedModelConfig content")
					var providerConfig ResolvedModelConfig
					err = json.Unmarshal([]byte(modelCM.Data["model.json"]), &providerConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(providerConfig.Provider.Type).To(Equal(agentv1alpha1.ProviderTypeOpenAI))
					Expect(providerConfig.Model).To(Equal("gpt-4o"))

					By("Verifying ModelReady condition")
					err = k8sClient.Get(ctx, typeNamespacedName, agent)
					Expect(err).NotTo(HaveOccurred())
					condition := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeModelReady)
					Expect(condition).NotTo(BeNil())
					Expect(condition.Status).To(Equal(metav1.ConditionTrue))
					Expect(condition.Reason).To(Equal(ReasonModelResolved))
				})

				It("should mount model ConfigMap in deployment", func() {
					By("Creating an Anthropic ModelProvider resource")
					provider := &agentv1alpha1.ModelProvider{
						ObjectMeta: metav1.ObjectMeta{
							Name:      providerName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelProviderSpec{
							Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
						},
					}
					Expect(k8sClient.Create(ctx, provider)).To(Succeed())

					providerReconciler := &ModelProviderReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err := providerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: providerName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating a Model resource")
					model := &agentv1alpha1.Model{
						ObjectMeta: metav1.ObjectMeta{
							Name:      modelName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelSpec{
							Model: "claude-sonnet-4-20250514",
							ProviderRef: agentv1alpha1.ProviderRef{
								Name: providerName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, model)).To(Succeed())

					modelReconciler := &ModelReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err = modelReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: modelName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating an Agent with Model reference")
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      agentName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Name:  "agent",
										Image: "nginx:latest",
									},
								},
							},
							Model: &agentv1alpha1.AgentModelRef{
								Name: modelName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, agent)).To(Succeed())

					By("Reconciling the Agent")
					controllerReconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					// First reconcile
					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					// Second reconcile
					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Verifying Deployment has model volume")
					deployment := &appsv1.Deployment{}
					Eventually(func() error {
						return k8sClient.Get(ctx, typeNamespacedName, deployment)
					}, timeout, interval).Should(Succeed())

					var modelVolume *corev1.Volume
					for i := range deployment.Spec.Template.Spec.Volumes {
						if deployment.Spec.Template.Spec.Volumes[i].Name == "model-config" {
							modelVolume = &deployment.Spec.Template.Spec.Volumes[i]
							break
						}
					}
					Expect(modelVolume).NotTo(BeNil())
					Expect(modelVolume.ConfigMap.Name).To(Equal(fmt.Sprintf("%s-model", agentName)))

					By("Verifying container has model volume mount")
					container := deployment.Spec.Template.Spec.Containers[0]
					var modelMount *corev1.VolumeMount
					for i := range container.VolumeMounts {
						if container.VolumeMounts[i].Name == "model-config" {
							modelMount = &container.VolumeMounts[i]
							break
						}
					}
					Expect(modelMount).NotTo(BeNil())
					Expect(modelMount.MountPath).To(Equal("/etc/flokoa/model.json"))
					Expect(modelMount.SubPath).To(Equal("model.json"))
					Expect(modelMount.ReadOnly).To(BeTrue())
				})

				It("should inject API key secret env var for OpenAI", func() {
					By("Creating an OpenAI ModelProvider with API key reference")
					provider := &agentv1alpha1.ModelProvider{
						ObjectMeta: metav1.ObjectMeta{
							Name:      providerName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelProviderSpec{
							APIKeySecretRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "my-openai-secret",
								},
								Key: "api-key",
							},
							OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
						},
					}
					Expect(k8sClient.Create(ctx, provider)).To(Succeed())

					providerReconciler := &ModelProviderReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err := providerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: providerName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating a Model resource")
					model := &agentv1alpha1.Model{
						ObjectMeta: metav1.ObjectMeta{
							Name:      modelName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelSpec{
							Model: "gpt-4o",
							ProviderRef: agentv1alpha1.ProviderRef{
								Name: providerName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, model)).To(Succeed())

					modelReconciler := &ModelReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err = modelReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: modelName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating an Agent with Model reference")
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      agentName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Name:  "agent",
										Image: "nginx:latest",
									},
								},
							},
							Model: &agentv1alpha1.AgentModelRef{
								Name: modelName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, agent)).To(Succeed())

					By("Reconciling the Agent")
					controllerReconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Verifying container has OPENAI_API_KEY secret env var")
					deployment := &appsv1.Deployment{}
					Eventually(func() error {
						return k8sClient.Get(ctx, typeNamespacedName, deployment)
					}, timeout, interval).Should(Succeed())

					container := deployment.Spec.Template.Spec.Containers[0]
					var apiKeyEnv *corev1.EnvVar
					for i := range container.Env {
						if container.Env[i].Name == "OPENAI_API_KEY" {
							apiKeyEnv = &container.Env[i]
							break
						}
					}
					Expect(apiKeyEnv).NotTo(BeNil())
					Expect(apiKeyEnv.ValueFrom).NotTo(BeNil())
					Expect(apiKeyEnv.ValueFrom.SecretKeyRef).NotTo(BeNil())
					Expect(apiKeyEnv.ValueFrom.SecretKeyRef.Name).To(Equal("my-openai-secret"))
					Expect(apiKeyEnv.ValueFrom.SecretKeyRef.Key).To(Equal("api-key"))
				})

				It("should inject API key secret env var for Anthropic", func() {
					By("Creating an Anthropic ModelProvider with API key reference")
					provider := &agentv1alpha1.ModelProvider{
						ObjectMeta: metav1.ObjectMeta{
							Name:      providerName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelProviderSpec{
							APIKeySecretRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "my-anthropic-secret",
								},
								Key: "anthropic-key",
							},
							Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
						},
					}
					Expect(k8sClient.Create(ctx, provider)).To(Succeed())

					providerReconciler := &ModelProviderReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err := providerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: providerName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating a Model resource")
					model := &agentv1alpha1.Model{
						ObjectMeta: metav1.ObjectMeta{
							Name:      modelName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelSpec{
							Model: "claude-sonnet-4-20250514",
							ProviderRef: agentv1alpha1.ProviderRef{
								Name: providerName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, model)).To(Succeed())

					modelReconciler := &ModelReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err = modelReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: modelName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating an Agent with Model reference")
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      agentName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Name:  "agent",
										Image: "nginx:latest",
									},
								},
							},
							Model: &agentv1alpha1.AgentModelRef{
								Name: modelName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, agent)).To(Succeed())

					By("Reconciling the Agent")
					controllerReconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Verifying container has ANTHROPIC_API_KEY secret env var")
					deployment := &appsv1.Deployment{}
					Eventually(func() error {
						return k8sClient.Get(ctx, typeNamespacedName, deployment)
					}, timeout, interval).Should(Succeed())

					container := deployment.Spec.Template.Spec.Containers[0]
					var apiKeyEnv *corev1.EnvVar
					for i := range container.Env {
						if container.Env[i].Name == "ANTHROPIC_API_KEY" {
							apiKeyEnv = &container.Env[i]
							break
						}
					}
					Expect(apiKeyEnv).NotTo(BeNil())
					Expect(apiKeyEnv.ValueFrom.SecretKeyRef.Name).To(Equal("my-anthropic-secret"))
					Expect(apiKeyEnv.ValueFrom.SecretKeyRef.Key).To(Equal("anthropic-key"))
				})
			})

			Context("Model with parameters", func() {
				It("should reconcile agent with Model that has parameters", func() {
					By("Creating a ModelProvider resource")
					provider := &agentv1alpha1.ModelProvider{
						ObjectMeta: metav1.ObjectMeta{
							Name:      providerName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelProviderSpec{
							OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
						},
					}
					Expect(k8sClient.Create(ctx, provider)).To(Succeed())

					providerReconciler := &ModelProviderReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err := providerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: providerName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating a Model with parameters")
					maxTokens := int32(4096)
					model := &agentv1alpha1.Model{
						ObjectMeta: metav1.ObjectMeta{
							Name:      modelName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelSpec{
							Model: "gpt-4o",
							ProviderRef: agentv1alpha1.ProviderRef{
								Name: providerName,
							},
							Parameters: &agentv1alpha1.ModelParameters{
								Temperature: "0.7",
								MaxTokens:   &maxTokens,
								TopP:        "0.9",
							},
						},
					}
					Expect(k8sClient.Create(ctx, model)).To(Succeed())

					modelReconciler := &ModelReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err = modelReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: modelName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating an Agent with Model reference")
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      agentName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Name:  "agent",
										Image: "nginx:latest",
									},
								},
							},
							Model: &agentv1alpha1.AgentModelRef{
								Name: modelName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, agent)).To(Succeed())

					By("Reconciling the Agent")
					controllerReconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Verifying Model ConfigMap was created with parameters")
					modelCMName := fmt.Sprintf("%s-model", agentName)
					modelCM := &corev1.ConfigMap{}
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{
							Name:      modelCMName,
							Namespace: agentNamespace,
						}, modelCM)
					}, timeout, interval).Should(Succeed())

					var providerConfig ResolvedModelConfig
					err = json.Unmarshal([]byte(modelCM.Data["model.json"]), &providerConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(providerConfig.Provider.Type).To(Equal(agentv1alpha1.ProviderTypeOpenAI))
					Expect(providerConfig.Model).To(Equal("gpt-4o"))
					Expect(providerConfig.Parameters).NotTo(BeNil())
					Expect(providerConfig.Parameters.Temperature).To(Equal("0.7"))
					Expect(providerConfig.Parameters.MaxTokens).NotTo(BeNil())
					Expect(*providerConfig.Parameters.MaxTokens).To(Equal(int32(4096)))
					Expect(providerConfig.Parameters.TopP).To(Equal("0.9"))

					By("Verifying ModelReady condition")
					err = k8sClient.Get(ctx, typeNamespacedName, agent)
					Expect(err).NotTo(HaveOccurred())
					condition := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeModelReady)
					Expect(condition).NotTo(BeNil())
					Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				})

				It("should include penalty parameters from Model", func() {
					By("Creating a ModelProvider resource")
					provider := &agentv1alpha1.ModelProvider{
						ObjectMeta: metav1.ObjectMeta{
							Name:      providerName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelProviderSpec{
							OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
						},
					}
					Expect(k8sClient.Create(ctx, provider)).To(Succeed())

					providerReconciler := &ModelProviderReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err := providerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: providerName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating a Model with penalty parameters")
					model := &agentv1alpha1.Model{
						ObjectMeta: metav1.ObjectMeta{
							Name:      modelName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelSpec{
							Model: "gpt-4o",
							ProviderRef: agentv1alpha1.ProviderRef{
								Name: providerName,
							},
							Parameters: &agentv1alpha1.ModelParameters{
								FrequencyPenalty: "0.5",
								PresencePenalty:  "0.3",
							},
						},
					}
					Expect(k8sClient.Create(ctx, model)).To(Succeed())

					modelReconciler := &ModelReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err = modelReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: modelName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating an Agent with Model reference")
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      agentName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Name:  "agent",
										Image: "nginx:latest",
									},
								},
							},
							Model: &agentv1alpha1.AgentModelRef{
								Name: modelName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, agent)).To(Succeed())

					By("Reconciling the Agent")
					controllerReconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Verifying penalty parameters in ConfigMap")
					modelCMName := fmt.Sprintf("%s-model", agentName)
					modelCM := &corev1.ConfigMap{}
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{
							Name:      modelCMName,
							Namespace: agentNamespace,
						}, modelCM)
					}, timeout, interval).Should(Succeed())

					var providerConfig ResolvedModelConfig
					err = json.Unmarshal([]byte(modelCM.Data["model.json"]), &providerConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(providerConfig.Parameters).NotTo(BeNil())
					Expect(providerConfig.Parameters.FrequencyPenalty).To(Equal("0.5"))
					Expect(providerConfig.Parameters.PresencePenalty).To(Equal("0.3"))
				})

				It("should include Anthropic thinking parameters from Model", func() {
					By("Creating an Anthropic ModelProvider resource")
					provider := &agentv1alpha1.ModelProvider{
						ObjectMeta: metav1.ObjectMeta{
							Name:      providerName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelProviderSpec{
							Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
						},
					}
					Expect(k8sClient.Create(ctx, provider)).To(Succeed())

					providerReconciler := &ModelProviderReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err := providerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: providerName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating a Model with Anthropic thinking parameters")
					budgetTokens := int32(2048)
					model := &agentv1alpha1.Model{
						ObjectMeta: metav1.ObjectMeta{
							Name:      modelName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelSpec{
							Model: "claude-sonnet-4-20250514",
							ProviderRef: agentv1alpha1.ProviderRef{
								Name: providerName,
							},
							Parameters: &agentv1alpha1.ModelParameters{
								Anthropic: &agentv1alpha1.AnthropicParameters{
									Thinking: &agentv1alpha1.AnthropicThinkingConfig{
										Type:         agentv1alpha1.ThinkingTypeEnabled,
										BudgetTokens: &budgetTokens,
									},
								},
							},
						},
					}
					Expect(k8sClient.Create(ctx, model)).To(Succeed())

					modelReconciler := &ModelReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err = modelReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: modelName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating an Agent with Model reference")
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      agentName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Name:  "agent",
										Image: "nginx:latest",
									},
								},
							},
							Model: &agentv1alpha1.AgentModelRef{
								Name: modelName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, agent)).To(Succeed())

					By("Reconciling the Agent")
					controllerReconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Verifying Anthropic parameters in ConfigMap")
					modelCMName := fmt.Sprintf("%s-model", agentName)
					modelCM := &corev1.ConfigMap{}
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{
							Name:      modelCMName,
							Namespace: agentNamespace,
						}, modelCM)
					}, timeout, interval).Should(Succeed())

					var providerConfig ResolvedModelConfig
					err = json.Unmarshal([]byte(modelCM.Data["model.json"]), &providerConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(providerConfig.Parameters).NotTo(BeNil())
					Expect(providerConfig.Parameters.Anthropic).NotTo(BeNil())
					Expect(providerConfig.Parameters.Anthropic.Thinking).NotTo(BeNil())
					Expect(providerConfig.Parameters.Anthropic.Thinking.Type).To(Equal(agentv1alpha1.ThinkingTypeEnabled))
					Expect(providerConfig.Parameters.Anthropic.Thinking.BudgetTokens).NotTo(BeNil())
					Expect(*providerConfig.Parameters.Anthropic.Thinking.BudgetTokens).To(Equal(int32(2048)))
				})

				It("should resolve Model from different namespace", func() {
					By("Creating a namespace for cross-namespace test")
					otherNamespace := "model-namespace"
					ns := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: otherNamespace,
						},
					}
					err := k8sClient.Create(ctx, ns)
					if err != nil && !errors.IsAlreadyExists(err) {
						Expect(err).NotTo(HaveOccurred())
					}

					By("Creating a ModelProvider in other namespace")
					provider := &agentv1alpha1.ModelProvider{
						ObjectMeta: metav1.ObjectMeta{
							Name:      providerName,
							Namespace: otherNamespace,
						},
						Spec: agentv1alpha1.ModelProviderSpec{
							OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
						},
					}
					Expect(k8sClient.Create(ctx, provider)).To(Succeed())
					defer func() {
						_ = k8sClient.Delete(ctx, provider)
					}()

					providerReconciler := &ModelProviderReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err = providerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: providerName, Namespace: otherNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating a Model in other namespace")
					model := &agentv1alpha1.Model{
						ObjectMeta: metav1.ObjectMeta{
							Name:      modelName,
							Namespace: otherNamespace,
						},
						Spec: agentv1alpha1.ModelSpec{
							Model: "gpt-4o",
							ProviderRef: agentv1alpha1.ProviderRef{
								Name: providerName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, model)).To(Succeed())
					defer func() {
						_ = k8sClient.Delete(ctx, model)
					}()

					modelReconciler := &ModelReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err = modelReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: modelName, Namespace: otherNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating an Agent referencing Model in other namespace")
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      agentName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Name:  "agent",
										Image: "nginx:latest",
									},
								},
							},
							Model: &agentv1alpha1.AgentModelRef{
								Name:      modelName,
								Namespace: otherNamespace,
							},
						},
					}
					Expect(k8sClient.Create(ctx, agent)).To(Succeed())

					By("Reconciling the Agent")
					controllerReconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Verifying Model ConfigMap was created")
					modelCMName := fmt.Sprintf("%s-model", agentName)
					modelCM := &corev1.ConfigMap{}
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{
							Name:      modelCMName,
							Namespace: agentNamespace,
						}, modelCM)
					}, timeout, interval).Should(Succeed())

					var providerConfig ResolvedModelConfig
					err = json.Unmarshal([]byte(modelCM.Data["model.json"]), &providerConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(providerConfig.Provider.Type).To(Equal(agentv1alpha1.ProviderTypeOpenAI))
				})
			})

			Context("Provider-specific configurations", func() {
				It("should handle OpenAI provider with custom baseURL", func() {
					By("Creating an OpenAI ModelProvider with custom config")
					provider := &agentv1alpha1.ModelProvider{
						ObjectMeta: metav1.ObjectMeta{
							Name:      providerName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelProviderSpec{
							OpenAI: &agentv1alpha1.OpenAIProviderSpec{
								BaseURL: "https://custom.openai.api.com/v1",
							},
						},
					}
					Expect(k8sClient.Create(ctx, provider)).To(Succeed())

					providerReconciler := &ModelProviderReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err := providerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: providerName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating a Model resource")
					model := &agentv1alpha1.Model{
						ObjectMeta: metav1.ObjectMeta{
							Name:      modelName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelSpec{
							Model: "gpt-4o",
							ProviderRef: agentv1alpha1.ProviderRef{
								Name: providerName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, model)).To(Succeed())

					modelReconciler := &ModelReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err = modelReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: modelName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating an Agent with Model reference")
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      agentName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Name:  "agent",
										Image: "nginx:latest",
									},
								},
							},
							Model: &agentv1alpha1.AgentModelRef{
								Name: modelName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, agent)).To(Succeed())

					By("Reconciling the Agent")
					controllerReconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Verifying ConfigMap has OpenAI-specific config")
					modelCMName := fmt.Sprintf("%s-model", agentName)
					modelCM := &corev1.ConfigMap{}
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{
							Name:      modelCMName,
							Namespace: agentNamespace,
						}, modelCM)
					}, timeout, interval).Should(Succeed())

					var providerConfig ResolvedModelConfig
					err = json.Unmarshal([]byte(modelCM.Data["model.json"]), &providerConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(providerConfig.Provider.OpenAI).NotTo(BeNil())
					Expect(providerConfig.Provider.OpenAI.BaseURL).To(Equal("https://custom.openai.api.com/v1"))

					By("Verifying Deployment has env vars for OpenAI config")
					deployment := &appsv1.Deployment{}
					Eventually(func() error {
						return k8sClient.Get(ctx, typeNamespacedName, deployment)
					}, timeout, interval).Should(Succeed())

					container := deployment.Spec.Template.Spec.Containers[0]
					envMap := make(map[string]string)
					for _, env := range container.Env {
						if env.Value != "" {
							envMap[env.Name] = env.Value
						}
					}
					Expect(envMap["OPENAI_BASE_URL"]).To(Equal("https://custom.openai.api.com/v1"))
				})

				It("should handle Anthropic provider with custom baseURL", func() {
					By("Creating an Anthropic ModelProvider with custom config")
					provider := &agentv1alpha1.ModelProvider{
						ObjectMeta: metav1.ObjectMeta{
							Name:      providerName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelProviderSpec{
							Anthropic: &agentv1alpha1.AnthropicProviderSpec{
								BaseURL: "https://custom.anthropic.api.com",
							},
						},
					}
					Expect(k8sClient.Create(ctx, provider)).To(Succeed())

					providerReconciler := &ModelProviderReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err := providerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: providerName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating a Model resource")
					model := &agentv1alpha1.Model{
						ObjectMeta: metav1.ObjectMeta{
							Name:      modelName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelSpec{
							Model: "claude-sonnet-4-20250514",
							ProviderRef: agentv1alpha1.ProviderRef{
								Name: providerName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, model)).To(Succeed())

					modelReconciler := &ModelReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err = modelReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: modelName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating an Agent with Model reference")
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      agentName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Name:  "agent",
										Image: "nginx:latest",
									},
								},
							},
							Model: &agentv1alpha1.AgentModelRef{
								Name: modelName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, agent)).To(Succeed())

					By("Reconciling the Agent")
					controllerReconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Verifying ConfigMap has Anthropic-specific config")
					modelCMName := fmt.Sprintf("%s-model", agentName)
					modelCM := &corev1.ConfigMap{}
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{
							Name:      modelCMName,
							Namespace: agentNamespace,
						}, modelCM)
					}, timeout, interval).Should(Succeed())

					var providerConfig ResolvedModelConfig
					err = json.Unmarshal([]byte(modelCM.Data["model.json"]), &providerConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(providerConfig.Provider.Anthropic).NotTo(BeNil())
					Expect(providerConfig.Provider.Anthropic.BaseURL).To(Equal("https://custom.anthropic.api.com"))

					By("Verifying Deployment has ANTHROPIC_BASE_URL env var")
					deployment := &appsv1.Deployment{}
					Eventually(func() error {
						return k8sClient.Get(ctx, typeNamespacedName, deployment)
					}, timeout, interval).Should(Succeed())

					container := deployment.Spec.Template.Spec.Containers[0]
					var baseURLEnv *corev1.EnvVar
					for i := range container.Env {
						if container.Env[i].Name == "ANTHROPIC_BASE_URL" {
							baseURLEnv = &container.Env[i]
							break
						}
					}
					Expect(baseURLEnv).NotTo(BeNil())
					Expect(baseURLEnv.Value).To(Equal("https://custom.anthropic.api.com"))
				})

				It("should include default headers in config", func() {
					By("Creating a ModelProvider with default headers")
					provider := &agentv1alpha1.ModelProvider{
						ObjectMeta: metav1.ObjectMeta{
							Name:      providerName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelProviderSpec{
							OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
							DefaultHeaders: map[string]string{
								"X-Custom-Header":  "custom-value",
								"X-Request-Source": "flokoa",
							},
						},
					}
					Expect(k8sClient.Create(ctx, provider)).To(Succeed())

					providerReconciler := &ModelProviderReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err := providerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: providerName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating a Model resource")
					model := &agentv1alpha1.Model{
						ObjectMeta: metav1.ObjectMeta{
							Name:      modelName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelSpec{
							Model: "gpt-4o",
							ProviderRef: agentv1alpha1.ProviderRef{
								Name: providerName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, model)).To(Succeed())

					modelReconciler := &ModelReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err = modelReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: modelName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating an Agent with Model reference")
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      agentName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Name:  "agent",
										Image: "nginx:latest",
									},
								},
							},
							Model: &agentv1alpha1.AgentModelRef{
								Name: modelName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, agent)).To(Succeed())

					By("Reconciling the Agent")
					controllerReconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Verifying ConfigMap has default headers")
					modelCMName := fmt.Sprintf("%s-model", agentName)
					modelCM := &corev1.ConfigMap{}
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{
							Name:      modelCMName,
							Namespace: agentNamespace,
						}, modelCM)
					}, timeout, interval).Should(Succeed())

					var providerConfig ResolvedModelConfig
					err = json.Unmarshal([]byte(modelCM.Data["model.json"]), &providerConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(providerConfig.Provider.DefaultHeaders).NotTo(BeNil())
					Expect(providerConfig.Provider.DefaultHeaders["X-Custom-Header"]).To(Equal("custom-value"))
					Expect(providerConfig.Provider.DefaultHeaders["X-Request-Source"]).To(Equal("flokoa"))
				})
			})

			Context("Error handling", func() {
				It("should fail when referenced Model does not exist", func() {
					By("Creating an Agent referencing non-existent Model")
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      agentName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Name:  "agent",
										Image: "nginx:latest",
									},
								},
							},
							Model: &agentv1alpha1.AgentModelRef{
								Name: "non-existent-model",
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
					_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})

					// Second reconcile should fail due to missing Model
					_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to get Model"))

					By("Verifying ModelReady condition is false")
					err = k8sClient.Get(ctx, typeNamespacedName, agent)
					Expect(err).NotTo(HaveOccurred())
					condition := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeModelReady)
					Expect(condition).NotTo(BeNil())
					Expect(condition.Status).To(Equal(metav1.ConditionFalse))
					Expect(condition.Reason).To(Equal(ReasonModelResolveFailed))
				})

				It("should fail when Model references non-existent ModelProvider", func() {
					By("Creating a Model referencing non-existent ModelProvider")
					model := &agentv1alpha1.Model{
						ObjectMeta: metav1.ObjectMeta{
							Name:      modelName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelSpec{
							Model: "gpt-4o",
							ProviderRef: agentv1alpha1.ProviderRef{
								Name: "non-existent-provider",
							},
						},
					}
					Expect(k8sClient.Create(ctx, model)).To(Succeed())

					By("Reconciling the Model (which will fail to find the provider)")
					modelReconciler := &ModelReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err := modelReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: modelName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred()) // Returns nil, but sets status to not ready

					By("Verifying Model status is not ready due to missing provider")
					err = k8sClient.Get(ctx, types.NamespacedName{Name: modelName, Namespace: agentNamespace}, model)
					Expect(err).NotTo(HaveOccurred())
					Expect(model.Status.Ready).To(BeFalse())

					By("Creating an Agent referencing the Model")
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      agentName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Name:  "agent",
										Image: "nginx:latest",
									},
								},
							},
							Model: &agentv1alpha1.AgentModelRef{
								Name: modelName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, agent)).To(Succeed())

					By("Reconciling the Agent")
					controllerReconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})

					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("is not ready"))

					By("Verifying ModelReady condition is false")
					err = k8sClient.Get(ctx, typeNamespacedName, agent)
					Expect(err).NotTo(HaveOccurred())
					condition := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeModelReady)
					Expect(condition).NotTo(BeNil())
					Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				})
			})

			Context("Model ConfigMap updates", func() {
				It("should update model ConfigMap when Model spec changes", func() {
					By("Creating a ModelProvider resource")
					provider := &agentv1alpha1.ModelProvider{
						ObjectMeta: metav1.ObjectMeta{
							Name:      providerName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelProviderSpec{
							OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
						},
					}
					Expect(k8sClient.Create(ctx, provider)).To(Succeed())

					providerReconciler := &ModelProviderReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err := providerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: providerName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating a Model resource")
					model := &agentv1alpha1.Model{
						ObjectMeta: metav1.ObjectMeta{
							Name:      modelName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelSpec{
							Model: "gpt-4o",
							ProviderRef: agentv1alpha1.ProviderRef{
								Name: providerName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, model)).To(Succeed())

					modelReconciler := &ModelReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err = modelReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: modelName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating an Agent with Model reference")
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      agentName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Name:  "agent",
										Image: "nginx:latest",
									},
								},
							},
							Model: &agentv1alpha1.AgentModelRef{
								Name: modelName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, agent)).To(Succeed())

					By("Reconciling the Agent")
					controllerReconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Verifying initial ConfigMap content")
					modelCMName := fmt.Sprintf("%s-model", agentName)
					modelCM := &corev1.ConfigMap{}
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{
							Name:      modelCMName,
							Namespace: agentNamespace,
						}, modelCM)
					}, timeout, interval).Should(Succeed())

					var initialConfig ResolvedModelConfig
					err = json.Unmarshal([]byte(modelCM.Data["model.json"]), &initialConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(initialConfig.Model).To(Equal("gpt-4o"))

					By("Updating the Model to use a different model")
					err = k8sClient.Get(ctx, types.NamespacedName{Name: modelName, Namespace: agentNamespace}, model)
					Expect(err).NotTo(HaveOccurred())
					model.Spec.Model = "gpt-4o-mini"
					Expect(k8sClient.Update(ctx, model)).To(Succeed())

					By("Reconciling the Agent again")
					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Verifying ConfigMap was updated")
					err = k8sClient.Get(ctx, types.NamespacedName{
						Name:      modelCMName,
						Namespace: agentNamespace,
					}, modelCM)
					Expect(err).NotTo(HaveOccurred())

					var updatedConfig ResolvedModelConfig
					err = json.Unmarshal([]byte(modelCM.Data["model.json"]), &updatedConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(updatedConfig.Model).To(Equal("gpt-4o-mini"))
				})

				It("should have correct labels on model ConfigMap", func() {
					By("Creating a ModelProvider resource")
					provider := &agentv1alpha1.ModelProvider{
						ObjectMeta: metav1.ObjectMeta{
							Name:      providerName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelProviderSpec{
							OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
						},
					}
					Expect(k8sClient.Create(ctx, provider)).To(Succeed())

					providerReconciler := &ModelProviderReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err := providerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: providerName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating a Model resource")
					model := &agentv1alpha1.Model{
						ObjectMeta: metav1.ObjectMeta{
							Name:      modelName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelSpec{
							Model: "gpt-4o",
							ProviderRef: agentv1alpha1.ProviderRef{
								Name: providerName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, model)).To(Succeed())

					modelReconciler := &ModelReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err = modelReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: modelName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating an Agent with Model reference")
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      agentName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Name:  "agent",
										Image: "nginx:latest",
									},
								},
							},
							Model: &agentv1alpha1.AgentModelRef{
								Name: modelName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, agent)).To(Succeed())

					By("Reconciling the Agent")
					controllerReconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Verifying ConfigMap labels")
					modelCMName := fmt.Sprintf("%s-model", agentName)
					modelCM := &corev1.ConfigMap{}
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{
							Name:      modelCMName,
							Namespace: agentNamespace,
						}, modelCM)
					}, timeout, interval).Should(Succeed())

					Expect(modelCM.Labels["app.kubernetes.io/name"]).To(Equal(agentName))
					Expect(modelCM.Labels["app.kubernetes.io/component"]).To(Equal("model-config"))
					Expect(modelCM.Labels["app.kubernetes.io/managed-by"]).To(Equal("flokoa-operator"))
					Expect(modelCM.Labels["flokoa.ai/agent"]).To(Equal(agentName))
				})

				It("should set owner reference on model ConfigMap", func() {
					By("Creating a ModelProvider resource")
					provider := &agentv1alpha1.ModelProvider{
						ObjectMeta: metav1.ObjectMeta{
							Name:      providerName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelProviderSpec{
							OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
						},
					}
					Expect(k8sClient.Create(ctx, provider)).To(Succeed())

					providerReconciler := &ModelProviderReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err := providerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: providerName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating a Model resource")
					model := &agentv1alpha1.Model{
						ObjectMeta: metav1.ObjectMeta{
							Name:      modelName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.ModelSpec{
							Model: "gpt-4o",
							ProviderRef: agentv1alpha1.ProviderRef{
								Name: providerName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, model)).To(Succeed())

					modelReconciler := &ModelReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}
					_, err = modelReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: modelName, Namespace: agentNamespace},
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating an Agent with Model reference")
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      agentName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Name:  "agent",
										Image: "nginx:latest",
									},
								},
							},
							Model: &agentv1alpha1.AgentModelRef{
								Name: modelName,
							},
						},
					}
					Expect(k8sClient.Create(ctx, agent)).To(Succeed())

					By("Reconciling the Agent")
					controllerReconciler := &AgentReconciler{
						Client: k8sClient,
						Scheme: k8sClient.Scheme(),
					}

					_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Verifying ConfigMap has owner reference")
					modelCMName := fmt.Sprintf("%s-model", agentName)
					modelCM := &corev1.ConfigMap{}
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{
							Name:      modelCMName,
							Namespace: agentNamespace,
						}, modelCM)
					}, timeout, interval).Should(Succeed())

					Expect(modelCM.OwnerReferences).To(HaveLen(1))
					Expect(modelCM.OwnerReferences[0].Name).To(Equal(agentName))
					Expect(modelCM.OwnerReferences[0].Kind).To(Equal("Agent"))
				})
			})

			Context("Agent without model", func() {
				It("should not create model ConfigMap when no model is specified", func() {
					By("Creating an Agent without model reference")
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      agentName,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							Card: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Spec: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Name:  "agent",
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

					_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Verifying model ConfigMap was NOT created")
					modelCMName := fmt.Sprintf("%s-model", agentName)
					modelCM := &corev1.ConfigMap{}
					err = k8sClient.Get(ctx, types.NamespacedName{
						Name:      modelCMName,
						Namespace: agentNamespace,
					}, modelCM)
					Expect(errors.IsNotFound(err)).To(BeTrue())

					By("Verifying deployment does not have model volume")
					deployment := &appsv1.Deployment{}
					Eventually(func() error {
						return k8sClient.Get(ctx, typeNamespacedName, deployment)
					}, timeout, interval).Should(Succeed())

					for _, vol := range deployment.Spec.Template.Spec.Volumes {
						Expect(vol.Name).NotTo(Equal("model-config"))
					}

					By("Verifying no ModelReady condition exists")
					err = k8sClient.Get(ctx, typeNamespacedName, agent)
					Expect(err).NotTo(HaveOccurred())
					condition := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeModelReady)
					Expect(condition).To(BeNil())
				})
			})

			Context("Multiple providers", func() {
				It("should support all registered providers", func() {
					By("Verifying all providers have handlers")
					providers := []agentv1alpha1.ProviderType{
						agentv1alpha1.ProviderTypeOpenAI,
						agentv1alpha1.ProviderTypeAnthropic,
						agentv1alpha1.ProviderTypeGoogle,
						agentv1alpha1.ProviderTypeBedrock,
					}

					for _, provider := range providers {
						handler, ok := GetProviderHandler(provider)
						Expect(ok).To(BeTrue(), "Provider %s should have a handler", provider)
						Expect(handler).NotTo(BeNil())
					}
				})
			})
		})
	})
})
