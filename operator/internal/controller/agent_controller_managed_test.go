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
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

var _ = Describe("Agent Controller - Managed Runtime", func() {
	It("should always set template runtime image in built deployment", func() {
		agent := &agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "template-image-guard",
				Namespace: "default",
			},
			Spec: agentv1alpha1.AgentSpec{
				CardOverride: minimalCard(),
				Runtime: agentv1alpha1.RuntimeSpec{
					Type:     agentv1alpha1.RuntimeTypeTemplate,
					Template: &agentv1alpha1.TemplatedRuntimeSpec{},
				},
			},
		}

		reconciler := &AgentReconciler{}
		deployment := reconciler.buildDeployment(agent, nil, "", nil, "", "")

		container := firstContainer(deployment)
		Expect(container.Image).NotTo(BeEmpty())
		Expect(container.Image).To(Equal(defaultTemplateRuntimeImage))
	})

	Context("When reconciling a managed Agent", func() {
		const agentNamespace = "default"

		var (
			ctx                context.Context
			agentName          string
			typeNamespacedName types.NamespacedName
		)

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

			r := newAgentReconciler()
			reconcileAgent(ctx, r, typeNamespacedName)

			By("Verifying agent status is Failed")
			agent = getAgent(ctx, typeNamespacedName)
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

			r := newAgentReconciler()
			reconcileAgent(ctx, r, typeNamespacedName)

			By("Verifying agent status is Failed")
			agent = getAgent(ctx, typeNamespacedName)
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

			r := newAgentReconciler()
			reconcileAgent(ctx, r, typeNamespacedName)

			By("Verifying agent status is Failed")
			agent = getAgent(ctx, typeNamespacedName)
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

			r := newAgentReconciler()
			reconcileAgent(ctx, r, typeNamespacedName)

			By("Verifying agent status is Failed")
			agent = getAgent(ctx, typeNamespacedName)
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

			r := newAgentReconciler()
			reconcileAgent(ctx, r, typeNamespacedName)

			By("Verifying the Instruction CR was created via label lookup")
			instructionList := &agentv1alpha1.InstructionList{}
			Eventually(func() int {
				_ = k8sClient.List(ctx, instructionList,
					client.InNamespace(agentNamespace),
					client.MatchingLabels{"flokoa.ai/agent": agentName},
				)
				return len(instructionList.Items)
			}, testTimeout, testInterval).Should(Equal(1))

			instruction := instructionList.Items[0]
			Expect(instruction.Spec.Content).To(Equal("You are a BYO agent with instructions."))
			Expect(instruction.Labels["flokoa.ai/agent"]).To(Equal(agentName))

			By("Verifying the Deployment has instruction volume mount")
			deployment := getDeployment(ctx, typeNamespacedName)
			container := firstContainer(deployment)

			vm := findVolumeMount(container, instructionVolumeName)
			Expect(vm).NotTo(BeNil(), "instruction volume mount should exist on BYO agent")
			Expect(vm.MountPath).To(Equal(instructionMountPath))
			Expect(vm.SubPath).To(Equal(instructionConfigMapKey))
			Expect(vm.ReadOnly).To(BeTrue())

			// Check FLOKOA_INSTRUCTION_PATH env var
			env := findEnvVar(container, "FLOKOA_INSTRUCTION_PATH")
			Expect(env).NotTo(BeNil())
			Expect(env.Value).To(Equal(instructionMountPath))
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

			r := newAgentReconciler()
			reconcileAgent(ctx, r, typeNamespacedName)

			By("Verifying the inline config ConfigMap via the deployment volume source")
			deployment := getDeployment(ctx, typeNamespacedName)

			templateConfigVol := findVolume(deployment.Spec.Template.Spec, templateConfigVolumeName)
			Expect(templateConfigVol).NotTo(BeNil(), "template-config volume should exist")
			Expect(templateConfigVol.ConfigMap).NotTo(BeNil())
			templateConfigCMName := templateConfigVol.ConfigMap.Name

			inlineCM := getConfigMap(ctx, types.NamespacedName{
				Name:      templateConfigCMName,
				Namespace: agentNamespace,
			})
			Expect(inlineCM.Data).To(HaveKey(templateConfigConfigMapKey))
			Expect(inlineCM.Labels["app.kubernetes.io/component"]).To(Equal("template-config"))

			By("Verifying the Deployment was created with correct inline configuration")
			Expect(*deployment.Spec.Replicas).To(Equal(int32(2)))

			container := firstContainer(deployment)
			Expect(container.Name).To(Equal("agent"))
			Expect(container.Image).To(Equal(defaultTemplateRuntimeImage))
			Expect(container.Ports).To(HaveLen(1))
			Expect(container.Ports[0].ContainerPort).To(Equal(int32(8080)))

			// Check for expected env vars
			runtimeModeEnv := findEnvVar(container, "FLOKOA_RUNTIME_MODE")
			Expect(runtimeModeEnv).NotTo(BeNil())
			Expect(runtimeModeEnv.Value).To(Equal("template"))

			templateConfigPathEnv := findEnvVar(container, "FLOKOA_TEMPLATE_CONFIG_PATH")
			Expect(templateConfigPathEnv).NotTo(BeNil())
			Expect(templateConfigPathEnv.Value).To(Equal(templateConfigMountPath))

			customVarEnv := findEnvVar(container, "CUSTOM_VAR")
			Expect(customVarEnv).NotTo(BeNil())
			Expect(customVarEnv.Value).To(Equal("custom-value"))

			agentURLEnv := findEnvVar(container, "FLOKOA_AGENT_URL")
			Expect(agentURLEnv).NotTo(BeNil())

			// Check resource requests
			Expect(container.Resources.Requests.Cpu().String()).To(Equal("100m"))
			Expect(container.Resources.Requests.Memory().String()).To(Equal("128Mi"))

			// Check managed config volume mount exists
			vm := findVolumeMount(container, templateConfigVolumeName)
			Expect(vm).NotTo(BeNil(), "managed config volume mount should exist")
			Expect(vm.MountPath).To(Equal(templateConfigMountPath))
			Expect(vm.ReadOnly).To(BeTrue())

			By("Verifying the Service was created with default ports")
			service := getService(ctx, typeNamespacedName)
			Expect(service.Spec.Ports).To(HaveLen(1))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(80)))
			Expect(service.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt32(8080)))

			By("Verifying the Agent status")
			agent = getAgent(ctx, typeNamespacedName)
			Expect(agent.Status.Backend).To(Equal("core"))
			Expect(agent.Status.URL).To(ContainSubstring(agentName))

			By("Cleanup model and provider")
			Expect(k8sClient.Delete(ctx, model)).To(Succeed())
			Expect(k8sClient.Delete(ctx, modelProvider)).To(Succeed())
		})

		It("should add template-config-hash annotation to pod template", func() {
			By("Creating prerequisite Model and ModelProvider")
			modelProvider := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("hash-provider-%s", agentName),
					Namespace: agentNamespace,
				},
				Spec: agentv1alpha1.ModelProviderSpec{
					Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
					APIKeySecretRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: fmt.Sprintf("hash-secret-%s", agentName),
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
					Name:      fmt.Sprintf("hash-model-%s", agentName),
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

			By("Creating a managed Agent with outputSchema")
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
									Name:        "original-schema",
									Description: "Original schema",
									JSONSchema:  &apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
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

			By("Reconciling the Agent")
			r := newAgentReconciler()
			reconcileAgent(ctx, r, typeNamespacedName)

			By("Verifying pod template has template-config-hash annotation")
			deployment := getDeployment(ctx, typeNamespacedName)
			Expect(deployment.Spec.Template.Annotations).To(HaveKey("flokoa.ai/template-config-hash"))
			initialHash := deployment.Spec.Template.Annotations["flokoa.ai/template-config-hash"]
			Expect(initialHash).NotTo(BeEmpty())

			By("Cleanup model and provider")
			Expect(k8sClient.Delete(ctx, model)).To(Succeed())
			Expect(k8sClient.Delete(ctx, modelProvider)).To(Succeed())
		})

		It("should update template-config-hash when outputSchema changes", func() {
			By("Creating prerequisite Model and ModelProvider")
			modelProvider := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("update-provider-%s", agentName),
					Namespace: agentNamespace,
				},
				Spec: agentv1alpha1.ModelProviderSpec{
					Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
					APIKeySecretRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: fmt.Sprintf("update-secret-%s", agentName),
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
					Name:      fmt.Sprintf("update-model-%s", agentName),
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

			By("Creating a managed Agent with outputSchema")
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
									Name:        "original-schema",
									Description: "Original schema",
									JSONSchema:  &apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
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

			By("Reconciling the Agent")
			r := newAgentReconciler()
			reconcileAgent(ctx, r, typeNamespacedName)

			By("Getting the initial template-config-hash")
			deployment := getDeployment(ctx, typeNamespacedName)
			initialHash := deployment.Spec.Template.Annotations["flokoa.ai/template-config-hash"]
			Expect(initialHash).NotTo(BeEmpty())

			By("Updating the Agent's outputSchema")
			Expect(k8sClient.Get(ctx, typeNamespacedName, agent)).To(Succeed())
			agent.Spec.Runtime.Template.Config.OutputSchema.JSONSchema = &apiextensionsv1.JSON{Raw: []byte(`{"type":"object","properties":{"dogs":{"type":"array"}}}`)}
			agent.Spec.Runtime.Template.Config.OutputSchema.Description = "Updated schema"
			Expect(k8sClient.Update(ctx, agent)).To(Succeed())

			By("Reconciling the Agent again")
			_, err := reconcileOnce(ctx, r, typeNamespacedName)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the template-config-hash has changed")
			Expect(k8sClient.Get(ctx, typeNamespacedName, deployment)).To(Succeed())
			updatedHash := deployment.Spec.Template.Annotations["flokoa.ai/template-config-hash"]
			Expect(updatedHash).NotTo(BeEmpty())
			Expect(updatedHash).NotTo(Equal(initialHash))

			By("Cleanup model and provider")
			Expect(k8sClient.Delete(ctx, model)).To(Succeed())
			Expect(k8sClient.Delete(ctx, modelProvider)).To(Succeed())
		})
	})
})
