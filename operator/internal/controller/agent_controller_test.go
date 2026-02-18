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

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

var _ = Describe("Agent Controller", func() {
	Context("When reconciling an Agent resource", func() {
		var (
			ctx                context.Context
			agentName          string
			typeNamespacedName types.NamespacedName
		)

		const agentNamespace = "default"

		BeforeEach(func() {
			ctx = context.Background()
			agentName = fmt.Sprintf("test-agent-%d", time.Now().UnixNano())
			typeNamespacedName = types.NamespacedName{
				Name:      agentName,
				Namespace: agentNamespace,
			}
		})

		AfterEach(func() {
			cleanupAgent(ctx, typeNamespacedName)
		})

		Context("Basic reconciliation", func() {
			It("should create Deployment and Service for a new Agent", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Name:  "agent",
									Image: "nginx:latest",
									Ports: []corev1.ContainerPort{
										{ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				By("Verifying the Deployment was created with correct spec")
				deployment := getDeployment(ctx, typeNamespacedName)
				Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
				Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
				Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal("nginx:latest"))

				By("Verifying the Service was created with correct ports")
				service := getService(ctx, typeNamespacedName)
				Expect(service.Spec.Ports).To(HaveLen(1))
				Expect(service.Spec.Ports[0].Port).To(Equal(int32(8080)))
				Expect(service.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt32(8080)))

				By("Verifying status was set correctly")
				agent = getAgent(ctx, typeNamespacedName)
				Expect(agent.Status.Phase).To(Equal(agentv1alpha1.AgentPhasePending))
				Expect(agent.Status.Backend).To(Equal("core"))
				// Verify URL has the expected Kubernetes service DNS format
				Expect(agent.Status.URL).To(Equal(
					fmt.Sprintf("http://%s.%s.svc.cluster.local", agentName, agentNamespace),
				))
			})

			It("should add finalizer on first reconcile and requeue", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{Image: "nginx:latest"},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				result, err := reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(time.Second))

				agent = getAgent(ctx, typeNamespacedName)
				Expect(controllerutil.ContainsFinalizer(agent, agentFinalizer)).To(BeTrue())
			})

			It("should handle reconcile request for non-existent Agent gracefully", func() {
				r := newAgentReconciler()
				_, err := reconcileOnce(ctx, r, types.NamespacedName{
					Name: "non-existent-agent", Namespace: agentNamespace,
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("Deletion handling", func() {
			It("should remove finalizer and allow deletion when DeletionTimestamp is set", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{Image: "nginx:latest"},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				// Add finalizer
				_, err := reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).NotTo(HaveOccurred())

				By("Verifying finalizer is present")
				agent = getAgent(ctx, typeNamespacedName)
				Expect(controllerutil.ContainsFinalizer(agent, agentFinalizer)).To(BeTrue())

				By("Deleting the agent (triggers deletion timestamp)")
				Expect(k8sClient.Delete(ctx, agent)).To(Succeed())

				By("Reconciling the deleted agent should remove finalizer")
				_, err = reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).NotTo(HaveOccurred())

				By("Agent should now be fully deleted")
				Eventually(func() bool {
					return errors.IsNotFound(k8sClient.Get(ctx, typeNamespacedName, &agentv1alpha1.Agent{}))
				}, testTimeout, testInterval).Should(BeTrue())
			})
		})

		Context("Custom configuration", func() {
			It("should respect custom replica count", func() {
				replicas := int32(3)
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								DeploymentOverrides: agentv1alpha1.DeploymentOverrides{
									Replicas: &replicas,
								},
								Container: corev1.Container{Image: "nginx:latest"},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				deployment := getDeployment(ctx, typeNamespacedName)
				Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))
			})

			It("should create Service with custom ports from container spec", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Image: "nginx:latest",
									Ports: []corev1.ContainerPort{
										{Name: "http", ContainerPort: 3000, Protocol: corev1.ProtocolTCP},
										{Name: "metrics", ContainerPort: 9090, Protocol: corev1.ProtocolTCP},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				service := getService(ctx, typeNamespacedName)
				Expect(service.Spec.Ports).To(HaveLen(2))
				Expect(service.Spec.Ports[0].Name).To(Equal("http"))
				Expect(service.Spec.Ports[0].Port).To(Equal(int32(3000)))
				Expect(service.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt32(3000)))
				Expect(service.Spec.Ports[1].Name).To(Equal("metrics"))
				Expect(service.Spec.Ports[1].Port).To(Equal(int32(9090)))
			})

			It("should propagate container resource limits to deployment", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
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

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				deployment := getDeployment(ctx, typeNamespacedName)
				container := firstContainer(deployment)
				Expect(container.Resources.Limits.Cpu().String()).To(Equal("500m"))
				Expect(container.Resources.Limits.Memory().String()).To(Equal("512Mi"))
				Expect(container.Resources.Requests.Cpu().String()).To(Equal("100m"))
				Expect(container.Resources.Requests.Memory().String()).To(Equal("128Mi"))
			})

			It("should create default service port (80→8080) when no container ports specified", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{Image: "nginx:latest"},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				service := getService(ctx, typeNamespacedName)
				Expect(service.Spec.Ports).To(HaveLen(1))
				Expect(service.Spec.Ports[0].Name).To(Equal("http"))
				Expect(service.Spec.Ports[0].Port).To(Equal(int32(80)))
				Expect(service.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt32(8080)))
			})

			It("should default container name to 'agent' when not specified", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{Image: "nginx:latest"},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				deployment := getDeployment(ctx, typeNamespacedName)
				container := firstContainer(deployment)
				Expect(container.Name).To(Equal("agent"))
			})
		})

		Context("Status updates", func() {
			It("should set Ready=False/DeploymentNotReady when no pods are available", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{Image: "nginx:latest"},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				agent = getAgent(ctx, typeNamespacedName)
				condition := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeReady)
				Expect(condition).NotTo(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Reason).To(Equal(ReasonDeploymentNotReady))
				Expect(condition.Message).To(Equal("Waiting for pods"))
			})

			It("should set observedGeneration matching the agent's generation", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{Image: "nginx:latest"},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				agent = getAgent(ctx, typeNamespacedName)
				Expect(agent.Status.ObservedGeneration).To(Equal(agent.Generation))
				// Generation should be at least 1 (created) + possibly more from finalizer update
				Expect(agent.Status.ObservedGeneration).To(BeNumerically(">=", int64(1)))
			})
		})

		Context("Label propagation", func() {
			It("should apply standard Kubernetes and Flokoa labels to all resources", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{Image: "nginx:latest"},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				deployment := getDeployment(ctx, typeNamespacedName)
				Expect(deployment.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", agentName))
				Expect(deployment.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", agentName))
				Expect(deployment.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "flokoa-operator"))
				Expect(deployment.Labels).To(HaveKeyWithValue("flokoa.ai/agent", agentName))

				service := getService(ctx, typeNamespacedName)
				Expect(service.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", agentName))
				Expect(service.Spec.Selector).To(HaveKeyWithValue("flokoa.ai/agent", agentName))
			})
		})

		Context("Owner references", func() {
			It("should set controller owner reference on Deployment and Service", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{Image: "nginx:latest"},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				deployment := getDeployment(ctx, typeNamespacedName)
				Expect(deployment.OwnerReferences).To(HaveLen(1))
				Expect(deployment.OwnerReferences[0].Kind).To(Equal("Agent"))
				Expect(deployment.OwnerReferences[0].Name).To(Equal(agentName))
				Expect(*deployment.OwnerReferences[0].Controller).To(BeTrue())

				service := getService(ctx, typeNamespacedName)
				Expect(service.OwnerReferences).To(HaveLen(1))
				Expect(service.OwnerReferences[0].Kind).To(Equal("Agent"))
				Expect(service.OwnerReferences[0].Name).To(Equal(agentName))
				Expect(*service.OwnerReferences[0].Controller).To(BeTrue())
			})
		})

		Context("Security context defaults", func() {
			It("should apply restricted PSS-compliant pod security context by default", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{Image: "nginx:latest"},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				deployment := getDeployment(ctx, typeNamespacedName)
				podSC := deployment.Spec.Template.Spec.SecurityContext
				Expect(podSC).NotTo(BeNil())
				Expect(*podSC.RunAsNonRoot).To(BeTrue())
				Expect(podSC.SeccompProfile).NotTo(BeNil())
				Expect(podSC.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeRuntimeDefault))
			})
		})

		Context("Deployment update path", func() {
			It("should update an existing Deployment when agent spec changes", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Image: "nginx:1.0",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				By("Verifying initial image")
				deployment := getDeployment(ctx, typeNamespacedName)
				Expect(firstContainer(deployment).Image).To(Equal("nginx:1.0"))

				By("Updating the agent image")
				agent = getAgent(ctx, typeNamespacedName)
				agent.Spec.Runtime.Standard.Container.Image = "nginx:2.0"
				Expect(k8sClient.Update(ctx, agent)).To(Succeed())

				By("Re-reconciling should update the deployment")
				_, err := reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).NotTo(HaveOccurred())

				deployment = getDeployment(ctx, typeNamespacedName)
				Expect(firstContainer(deployment).Image).To(Equal("nginx:2.0"))
			})

			It("should update Service when container ports change", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Image: "nginx:latest",
									Ports: []corev1.ContainerPort{
										{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				By("Verifying initial port")
				service := getService(ctx, typeNamespacedName)
				Expect(service.Spec.Ports[0].Port).To(Equal(int32(8080)))

				By("Changing the container port")
				agent = getAgent(ctx, typeNamespacedName)
				agent.Spec.Runtime.Standard.Container.Ports[0].ContainerPort = 9090
				Expect(k8sClient.Update(ctx, agent)).To(Succeed())

				_, err := reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).NotTo(HaveOccurred())

				service = getService(ctx, typeNamespacedName)
				Expect(service.Spec.Ports[0].Port).To(Equal(int32(9090)))
			})
		})

		Context("calculatePhase", func() {
			It("should return Running when available replicas > 0", func() {
				r := &AgentReconciler{}
				deployment := &appsv1.Deployment{
					Status: appsv1.DeploymentStatus{
						AvailableReplicas: 2,
					},
				}
				Expect(r.calculatePhase(deployment)).To(Equal(agentv1alpha1.AgentPhaseRunning))
			})

			It("should return Pending when available replicas == 0", func() {
				r := &AgentReconciler{}
				deployment := &appsv1.Deployment{
					Status: appsv1.DeploymentStatus{
						AvailableReplicas: 0,
					},
				}
				Expect(r.calculatePhase(deployment)).To(Equal(agentv1alpha1.AgentPhasePending))
			})
		})

		Context("Instruction validation", func() {
			It("should fail validation when instruction has both inline and ref set", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{Image: "nginx:latest"},
							},
						},
						Instruction: &agentv1alpha1.InstructionEntry{
							Template: "Some inline instruction",
							InstructionRef: &agentv1alpha1.NamespacedRef{
								Name: "some-instruction",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()

				// First reconcile: finalizer
				_, err := reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).NotTo(HaveOccurred())

				// Second reconcile: validation failure
				_, err = reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).NotTo(HaveOccurred()) // validation errors don't return err

				agent = getAgent(ctx, typeNamespacedName)
				Expect(agent.Status.Phase).To(Equal(agentv1alpha1.AgentPhaseFailed))

				readyCond := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeReady)
				Expect(readyCond).NotTo(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
				Expect(readyCond.Reason).To(Equal(ReasonValidationFailed))
				Expect(readyCond.Message).To(ContainSubstring("mutually exclusive"))
			})

			It("should fail validation when instruction has neither inline nor ref set", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{Image: "nginx:latest"},
							},
						},
						Instruction: &agentv1alpha1.InstructionEntry{},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				_, _ = reconcileOnce(ctx, r, typeNamespacedName)
				_, err := reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).NotTo(HaveOccurred())

				agent = getAgent(ctx, typeNamespacedName)
				Expect(agent.Status.Phase).To(Equal(agentv1alpha1.AgentPhaseFailed))

				readyCond := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeReady)
				Expect(readyCond).NotTo(BeNil())
				Expect(readyCond.Reason).To(Equal(ReasonValidationFailed))
				Expect(readyCond.Message).To(ContainSubstring("either inline or instructionRef"))
			})

			It("should reject unsupported runtime type at CRD validation", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: "unsupported-type",
						},
					},
				}
				err := k8sClient.Create(ctx, agent)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsInvalid(err)).To(BeTrue())
			})
		})

		Context("Watch handlers", func() {
			It("findAgentsForModel should return agents referencing the given Model", func() {
				By("Creating an Agent that references a Model")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{Image: "nginx:latest"},
							},
						},
						Model: &agentv1alpha1.AgentModelRef{
							Name: "target-model",
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				model := &agentv1alpha1.Model{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "target-model",
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.ModelSpec{
						Model:       "gpt-4o",
						ProviderRef: agentv1alpha1.ProviderRef{Name: "some-provider"},
					},
				}
				Expect(k8sClient.Create(ctx, model)).To(Succeed())
				defer func() { _ = k8sClient.Delete(ctx, model) }()

				r := newAgentReconciler()
				requests := r.findAgentsForModel(ctx, model)

				found := false
				for _, req := range requests {
					if req.Name == agentName {
						found = true
						break
					}
				}
				Expect(found).To(BeTrue(), "should find the agent referencing the model")
			})

			It("findAgentsForModel should not return agents referencing a different Model", func() {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{Image: "nginx:latest"},
							},
						},
						Model: &agentv1alpha1.AgentModelRef{
							Name: "other-model",
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				unrelatedModel := &agentv1alpha1.Model{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unrelated-model",
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.ModelSpec{
						Model:       "gpt-4o",
						ProviderRef: agentv1alpha1.ProviderRef{Name: "some-provider"},
					},
				}
				Expect(k8sClient.Create(ctx, unrelatedModel)).To(Succeed())
				defer func() { _ = k8sClient.Delete(ctx, unrelatedModel) }()

				r := newAgentReconciler()
				requests := r.findAgentsForModel(ctx, unrelatedModel)

				for _, req := range requests {
					Expect(req.Name).NotTo(Equal(agentName))
				}
			})

			It("findAgentsForModelProvider should return agents referencing models that use the provider", func() {
				providerName := fmt.Sprintf("target-provider-%d", time.Now().UnixNano())
				modelName := fmt.Sprintf("target-model-%d", time.Now().UnixNano())

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
				defer func() { _ = k8sClient.Delete(ctx, provider) }()

				model := &agentv1alpha1.Model{
					ObjectMeta: metav1.ObjectMeta{
						Name:      modelName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.ModelSpec{
						Model:       "gpt-4o",
						ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
					},
				}
				Expect(k8sClient.Create(ctx, model)).To(Succeed())
				defer func() { _ = k8sClient.Delete(ctx, model) }()

				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{Image: "nginx:latest"},
							},
						},
						Model: &agentv1alpha1.AgentModelRef{Name: modelName},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				requests := r.findAgentsForModelProvider(ctx, provider)

				found := false
				for _, req := range requests {
					if req.Name == agentName {
						found = true
						break
					}
				}
				Expect(found).To(BeTrue(), "should find the agent depending on models that use this provider")
			})

			It("findAgentsForInstruction should find agent by label for inline instruction", func() {
				instruction := &agentv1alpha1.Instruction{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-inline-instruction",
						Namespace: agentNamespace,
						Labels: map[string]string{
							"flokoa.ai/agent": "owner-agent",
						},
					},
					Spec: agentv1alpha1.InstructionSpec{
						Content: "test",
					},
				}
				Expect(k8sClient.Create(ctx, instruction)).To(Succeed())
				defer func() { _ = k8sClient.Delete(ctx, instruction) }()

				r := newAgentReconciler()
				requests := r.findAgentsForInstruction(ctx, instruction)

				Expect(requests).To(HaveLen(1))
				Expect(requests[0].Name).To(Equal("owner-agent"))
				Expect(requests[0].Namespace).To(Equal(agentNamespace))
			})

			It("findAgentsForInstruction should find agents with matching instructionRef", func() {
				instructionName := fmt.Sprintf("shared-instr-%d", time.Now().UnixNano())

				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{Image: "nginx:latest"},
							},
						},
						Instruction: &agentv1alpha1.InstructionEntry{
							InstructionRef: &agentv1alpha1.NamespacedRef{
								Name: instructionName,
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				instruction := &agentv1alpha1.Instruction{
					ObjectMeta: metav1.ObjectMeta{
						Name:      instructionName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.InstructionSpec{
						Content: "Shared instruction content",
					},
				}
				Expect(k8sClient.Create(ctx, instruction)).To(Succeed())
				defer func() { _ = k8sClient.Delete(ctx, instruction) }()

				r := newAgentReconciler()
				requests := r.findAgentsForInstruction(ctx, instruction)

				found := false
				for _, req := range requests {
					if req.Name == agentName {
						found = true
						break
					}
				}
				Expect(found).To(BeTrue(), "should find agent with matching instructionRef")
			})
		})

		Context("computeSecretRefsHash", func() {
			It("should return empty hash when there are no secret env vars", func() {
				r := newAgentReconciler()
				hash, missing, err := r.computeSecretRefsHash(ctx, agentNamespace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(hash).To(BeEmpty())
				Expect(missing).To(BeNil())
			})

			It("should report missing secrets", func() {
				r := newAgentReconciler()
				envVars := []corev1.EnvVar{
					{
						Name: "MY_KEY",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "nonexistent-secret",
								},
								Key: "key",
							},
						},
					},
				}
				hash, missing, err := r.computeSecretRefsHash(ctx, agentNamespace, envVars)
				Expect(err).NotTo(HaveOccurred())
				Expect(hash).NotTo(BeEmpty())
				Expect(missing).To(ContainElement("nonexistent-secret"))
			})

			It("should compute hash from existing secrets using resource version", func() {
				secretName := fmt.Sprintf("test-secret-%d", time.Now().UnixNano())
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: agentNamespace,
					},
					Data: map[string][]byte{"key": []byte("value")},
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())
				defer func() { _ = k8sClient.Delete(ctx, secret) }()

				r := newAgentReconciler()
				envVars := []corev1.EnvVar{
					{
						Name: "MY_KEY",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
								Key:                  "key",
							},
						},
					},
				}
				hash, missing, err := r.computeSecretRefsHash(ctx, agentNamespace, envVars)
				Expect(err).NotTo(HaveOccurred())
				Expect(hash).NotTo(BeEmpty())
				Expect(hash).To(HaveLen(16))
				Expect(missing).To(BeEmpty())
			})

			It("should produce different hashes when secret resource version changes", func() {
				secretName := fmt.Sprintf("test-secret-%d", time.Now().UnixNano())
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: agentNamespace,
					},
					Data: map[string][]byte{"key": []byte("value1")},
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())
				defer func() { _ = k8sClient.Delete(ctx, secret) }()

				r := newAgentReconciler()
				envVars := []corev1.EnvVar{
					{
						Name: "MY_KEY",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
								Key:                  "key",
							},
						},
					},
				}

				hash1, _, err := r.computeSecretRefsHash(ctx, agentNamespace, envVars)
				Expect(err).NotTo(HaveOccurred())

				// Update the secret to change its ResourceVersion
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: agentNamespace}, secret)).To(Succeed())
				secret.Data["key"] = []byte("value2")
				Expect(k8sClient.Update(ctx, secret)).To(Succeed())

				hash2, _, err := r.computeSecretRefsHash(ctx, agentNamespace, envVars)
				Expect(err).NotTo(HaveOccurred())

				Expect(hash1).NotTo(Equal(hash2))
			})
		})

		Context("hashConfigMapData", func() {
			It("should return empty string for nil or empty data", func() {
				Expect(hashConfigMapData(nil)).To(BeEmpty())
				Expect(hashConfigMapData(map[string]string{})).To(BeEmpty())
			})

			It("should produce deterministic hash regardless of map iteration order", func() {
				data1 := map[string]string{"a": "1", "b": "2", "c": "3"}
				data2 := map[string]string{"c": "3", "a": "1", "b": "2"}
				Expect(hashConfigMapData(data1)).To(Equal(hashConfigMapData(data2)))
			})

			It("should produce different hashes for different data", func() {
				data1 := map[string]string{"key": "value1"}
				data2 := map[string]string{"key": "value2"}
				Expect(hashConfigMapData(data1)).NotTo(Equal(hashConfigMapData(data2)))
			})

			It("should produce 16-character hex string", func() {
				hash := hashConfigMapData(map[string]string{"key": "value"})
				Expect(hash).To(HaveLen(16))
				Expect(hash).To(MatchRegexp("^[0-9a-f]{16}$"))
			})
		})
	})
})
