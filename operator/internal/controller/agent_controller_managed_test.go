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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
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

var _ = Describe("Agent Controller - Managed Runtime", func() {
	Context("When reconciling a managed Agent", func() {
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

		It("should fail validation when model is not set for managed runtime", func() {
			By("Creating a managed Agent without a model reference")
			agent := &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: agentNamespace,
				},
				Spec: agentv1alpha1.AgentSpec{
					CardOverride: minimalCard(),
					Instruction: &agentv1alpha1.InstructionEntry{
						Template: "You are a test agent.",
					},
					Runtime: agentv1alpha1.RuntimeSpec{
						Type: agentv1alpha1.RuntimeTypeTemplate,
						Template: &agentv1alpha1.TemplatedRuntimeSpec{
							Config: &agentv1alpha1.TemplatedAgentConfig{
								OutputSchema: &agentv1alpha1.StructuredIOSchema{
									Name:        "test-schema",
									Description: "Test schema",
									JSONSchema:  &apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
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

			// First reconcile adds finalizer
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Second reconcile should fail validation (no model)
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred()) // validation errors don't return err, they set status

			By("Verifying agent status is Failed")
			err = k8sClient.Get(ctx, typeNamespacedName, agent)
			Expect(err).NotTo(HaveOccurred())
			Expect(agent.Status.Phase).To(Equal(agentv1alpha1.AgentPhaseFailed))

			readyCond := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(ReasonValidationFailed))
			Expect(readyCond.Message).To(ContainSubstring("spec.model is required"))
		})

		It("should fail validation when runtime.standard is set with managed type", func() {
			By("Creating a managed Agent with both standard and managed set")
			agent := &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: agentNamespace,
				},
				Spec: agentv1alpha1.AgentSpec{
					CardOverride: minimalCard(),
					Instruction: &agentv1alpha1.InstructionEntry{
						Template: "You are a test agent.",
					},
					Runtime: agentv1alpha1.RuntimeSpec{
						Type: agentv1alpha1.RuntimeTypeTemplate,
						Standard: &agentv1alpha1.StandardRuntimeSpec{
							Container: corev1.Container{
								Image: "nginx:latest",
							},
						},
						Template: &agentv1alpha1.TemplatedRuntimeSpec{
							Config: &agentv1alpha1.TemplatedAgentConfig{
								OutputSchema: &agentv1alpha1.StructuredIOSchema{
									Name:        "test-schema",
									Description: "Test schema",
									JSONSchema:  &apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
								},
							},
						},
					},
					Model: &agentv1alpha1.AgentModelRef{
						Name: "test-model",
					},
				},
			}
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

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

			// Second reconcile should fail validation
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying agent status is Failed")
			err = k8sClient.Get(ctx, typeNamespacedName, agent)
			Expect(err).NotTo(HaveOccurred())
			Expect(agent.Status.Phase).To(Equal(agentv1alpha1.AgentPhaseFailed))

			readyCond := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Reason).To(Equal(ReasonValidationFailed))
			Expect(readyCond.Message).To(ContainSubstring("runtime.standard must not be set"))
		})

		It("should fail validation when runtime.managed is set with standard type", func() {
			By("Creating a standard Agent with managed set")
			agent := &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: agentNamespace,
				},
				Spec: agentv1alpha1.AgentSpec{
					CardOverride: minimalCard(),
					Runtime: agentv1alpha1.RuntimeSpec{
						Type: agentv1alpha1.RuntimeTypeStandard,
						Template: &agentv1alpha1.TemplatedRuntimeSpec{
							Config: &agentv1alpha1.TemplatedAgentConfig{
								OutputSchema: &agentv1alpha1.StructuredIOSchema{
									Name:        "test-schema",
									Description: "Test schema",
									JSONSchema:  &apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
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

			// First reconcile adds finalizer
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Second reconcile should fail validation
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying agent status is Failed")
			err = k8sClient.Get(ctx, typeNamespacedName, agent)
			Expect(err).NotTo(HaveOccurred())
			Expect(agent.Status.Phase).To(Equal(agentv1alpha1.AgentPhaseFailed))

			readyCond := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Reason).To(Equal(ReasonValidationFailed))
			Expect(readyCond.Message).To(ContainSubstring("runtime.managed must not be set"))
		})

		It("should fail validation when instruction is not set for managed runtime", func() {
			By("Creating a managed Agent without instruction")
			agent := &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: agentNamespace,
				},
				Spec: agentv1alpha1.AgentSpec{
					CardOverride: minimalCard(),
					Runtime: agentv1alpha1.RuntimeSpec{
						Type: agentv1alpha1.RuntimeTypeTemplate,
						Template: &agentv1alpha1.TemplatedRuntimeSpec{
							Config: &agentv1alpha1.TemplatedAgentConfig{
								OutputSchema: &agentv1alpha1.StructuredIOSchema{
									Name:        "test-schema",
									Description: "Test schema",
									JSONSchema:  &apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
								},
							},
						},
					},
					Model: &agentv1alpha1.AgentModelRef{
						Name: "test-model",
					},
				},
			}
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

			controllerReconciler := &AgentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile should fail validation
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying agent status is Failed")
			err = k8sClient.Get(ctx, typeNamespacedName, agent)
			Expect(err).NotTo(HaveOccurred())
			Expect(agent.Status.Phase).To(Equal(agentv1alpha1.AgentPhaseFailed))

			readyCond := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Reason).To(Equal(ReasonValidationFailed))
			Expect(readyCond.Message).To(ContainSubstring("spec.instruction is required"))
		})

		It("should allow standard agents to have optional instruction", func() {
			By("Creating a standard Agent with inline instruction")
			agent := &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: agentNamespace,
				},
				Spec: agentv1alpha1.AgentSpec{
					CardOverride: minimalCard(),
					Instruction: &agentv1alpha1.InstructionEntry{
						Template: "You are a BYO agent with instructions.",
					},
					Runtime: agentv1alpha1.RuntimeSpec{
						Type: agentv1alpha1.RuntimeTypeStandard,
						Standard: &agentv1alpha1.StandardRuntimeSpec{
							Container: corev1.Container{
								Name:  "agent",
								Image: "my-byo-agent:latest",
								Ports: []corev1.ContainerPort{
									{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
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

			// First reconcile adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile creates resources including Instruction CR
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Instruction CR was created")
			instruction := &agentv1alpha1.Instruction{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("%s-instruction", agentName),
					Namespace: agentNamespace,
				}, instruction)
			}, timeout, interval).Should(Succeed())

			Expect(instruction.Spec.Content).To(Equal("You are a BYO agent with instructions."))
			Expect(instruction.Labels["flokoa.ai/agent"]).To(Equal(agentName))

			By("Verifying the Deployment has instruction volume mount")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, deployment)
			}, timeout, interval).Should(Succeed())

			container := deployment.Spec.Template.Spec.Containers[0]
			var foundInstructionMount bool
			for _, vm := range container.VolumeMounts {
				if vm.Name == instructionVolumeName {
					foundInstructionMount = true
					Expect(vm.MountPath).To(Equal(instructionMountPath))
					Expect(vm.SubPath).To(Equal(instructionConfigMapKey))
					Expect(vm.ReadOnly).To(BeTrue())
				}
			}
			Expect(foundInstructionMount).To(BeTrue(), "instruction volume mount should exist on BYO agent")

			// Check FLOKOA_INSTRUCTION_PATH env var
			envMap := make(map[string]string)
			for _, env := range container.Env {
				if env.Value != "" {
					envMap[env.Name] = env.Value
				}
			}
			Expect(envMap).To(HaveKeyWithValue("FLOKOA_INSTRUCTION_PATH", instructionMountPath))
		})

		It("should create Deployment, Service, and managed config ConfigMap for a managed agent", func() {
			By("Creating prerequisite Model and ModelProvider")
			modelProvider := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("inline-provider-%s", agentName),
					Namespace: agentNamespace,
				},
				Spec: agentv1alpha1.ModelProviderSpec{
					Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
					APIKeySecretRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: fmt.Sprintf("inline-secret-%s", agentName),
						},
						Key: "api-key",
					},
				},
			}
			Expect(k8sClient.Create(ctx, modelProvider)).To(Succeed())
			modelProvider.Status.Ready = true
			modelProvider.Status.Provider = agentv1alpha1.ProviderTypeAnthropic
			Expect(k8sClient.Status().Update(ctx, modelProvider)).To(Succeed())

			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("inline-model-%s", agentName),
					Namespace: agentNamespace,
				},
				Spec: agentv1alpha1.ModelSpec{
					Model: "claude-sonnet-4-20250514",
					ProviderRef: agentv1alpha1.ProviderRef{
						Name: modelProvider.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())
			model.Status.Ready = true
			model.Status.ResolvedProvider = &agentv1alpha1.ResolvedProviderInfo{
				Provider:  agentv1alpha1.ProviderTypeAnthropic,
				Namespace: agentNamespace,
				Name:      modelProvider.Name,
			}
			Expect(k8sClient.Status().Update(ctx, model)).To(Succeed())

			By("Creating the managed Agent")
			replicas := int32(2)
			agent := &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: agentNamespace,
				},
				Spec: agentv1alpha1.AgentSpec{
					CardOverride: minimalCard(),
					Instruction: &agentv1alpha1.InstructionEntry{
						Template: "You are a support triage agent. Classify tickets by severity.",
					},
					Runtime: agentv1alpha1.RuntimeSpec{
						Type: agentv1alpha1.RuntimeTypeTemplate,
						Template: &agentv1alpha1.TemplatedRuntimeSpec{
							Config: &agentv1alpha1.TemplatedAgentConfig{
								OutputSchema: &agentv1alpha1.StructuredIOSchema{
									Name:        "ticket-classification",
									Description: "Ticket classification agent",
									JSONSchema:  &apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
								},
							},
							DeploymentOverrides: agentv1alpha1.DeploymentOverrides{
								Replicas: &replicas,
							},
							Env: []corev1.EnvVar{
								{Name: "CUSTOM_VAR", Value: "custom-value"},
							},
							Resources: &corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
							},
						},
					},
					Model: &agentv1alpha1.AgentModelRef{
						Name: model.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

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

			By("Verifying the inline config ConfigMap was created")
			inlineCM := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("%s-managed-config", agentName),
					Namespace: agentNamespace,
				}, inlineCM)
			}, timeout, interval).Should(Succeed())

			Expect(inlineCM.Data).To(HaveKey(templateConfigConfigMapKey))
			Expect(inlineCM.Labels["app.kubernetes.io/component"]).To(Equal("managed-config"))

			By("Verifying the Deployment was created with correct inline configuration")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, deployment)
			}, timeout, interval).Should(Succeed())

			Expect(*deployment.Spec.Replicas).To(Equal(int32(2)))
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))

			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.Name).To(Equal("agent"))
			Expect(container.Image).To(Equal(defaultManagedRuntimeImage))
			Expect(container.Ports).To(HaveLen(1))
			Expect(container.Ports[0].ContainerPort).To(Equal(int32(8080)))

			// Check for FLOKOA_RUNTIME_MODE env var
			envMap := make(map[string]string)
			for _, env := range container.Env {
				if env.Value != "" {
					envMap[env.Name] = env.Value
				}
			}
			Expect(envMap).To(HaveKeyWithValue("FLOKOA_RUNTIME_MODE", "managed"))
			Expect(envMap).To(HaveKeyWithValue("FLOKOA_MANAGED_CONFIG_PATH", templateConfigMountPath))
			Expect(envMap).To(HaveKeyWithValue("CUSTOM_VAR", "custom-value"))
			Expect(envMap).To(HaveKey("FLOKOA_AGENT_URL"))

			// Check resource requests
			Expect(container.Resources.Requests.Cpu().String()).To(Equal("100m"))
			Expect(container.Resources.Requests.Memory().String()).To(Equal("128Mi"))

			// Check managed config volume mount exists
			var foundManagedMount bool
			for _, vm := range container.VolumeMounts {
				if vm.Name == templateConfigVolumeName {
					foundManagedMount = true
					Expect(vm.MountPath).To(Equal(templateConfigMountPath))
					Expect(vm.ReadOnly).To(BeTrue())
				}
			}
			Expect(foundManagedMount).To(BeTrue(), "managed config volume mount should exist")

			By("Verifying the Service was created with default ports")
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, service)
			}, timeout, interval).Should(Succeed())

			Expect(service.Spec.Ports).To(HaveLen(1))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(80)))
			Expect(service.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt32(8080)))

			By("Verifying the Agent status")
			err = k8sClient.Get(ctx, typeNamespacedName, agent)
			Expect(err).NotTo(HaveOccurred())
			Expect(agent.Status.Backend).To(Equal("core"))
			Expect(agent.Status.URL).To(ContainSubstring(agentName))

			By("Cleanup model and provider")
			Expect(k8sClient.Delete(ctx, model)).To(Succeed())
			Expect(k8sClient.Delete(ctx, modelProvider)).To(Succeed())
		})
	})
})
