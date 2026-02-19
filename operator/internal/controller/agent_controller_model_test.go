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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// getModelConfigMap discovers the model ConfigMap name from the deployment's
// "model-config" volume and fetches it. This avoids reconstructing the name
// with the same logic as production code (tautological assertion).
func getModelConfigMap(ctx context.Context, nn types.NamespacedName) *corev1.ConfigMap {
	deployment := getDeployment(ctx, nn)
	vol := findVolume(deployment.Spec.Template.Spec, "model-config")
	ExpectWithOffset(1, vol).NotTo(BeNil(), "deployment should have a model-config volume")
	ExpectWithOffset(1, vol.ConfigMap).NotTo(BeNil(), "model-config volume should reference a ConfigMap")

	cmName := vol.ConfigMap.Name
	return getConfigMap(ctx, types.NamespacedName{Name: cmName, Namespace: nn.Namespace})
}

var _ = Describe("Agent Controller - Model", func() {
	Context("When reconciling an Agent with Model", func() {
		var (
			ctx                context.Context
			agentName          string
			typeNamespacedName types.NamespacedName
		)

		const agentNamespace = "default"

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
			cleanupAgent(ctx, typeNamespacedName)
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
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
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
					r := newAgentReconciler()
					reconcileAgent(ctx, r, typeNamespacedName)

					By("Verifying Model ConfigMap was created")
					modelCM := getModelConfigMap(ctx, typeNamespacedName)
					Expect(modelCM.Data).To(HaveKey("model.json"))

					By("Verifying ResolvedModelConfig content")
					var providerConfig ResolvedModelConfig
					err = json.Unmarshal([]byte(modelCM.Data["model.json"]), &providerConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(providerConfig.Provider.Type).To(Equal(agentv1alpha1.ProviderTypeOpenAI))
					Expect(providerConfig.Model).To(Equal("gpt-4o"))

					By("Verifying ModelReady condition")
					agent = getAgent(ctx, typeNamespacedName)
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
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
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
					r := newAgentReconciler()
					reconcileAgent(ctx, r, typeNamespacedName)

					By("Verifying Deployment has model volume")
					deployment := getDeployment(ctx, typeNamespacedName)

					modelVolume := findVolume(deployment.Spec.Template.Spec, "model-config")
					Expect(modelVolume).NotTo(BeNil())
					Expect(modelVolume.ConfigMap).NotTo(BeNil())

					By("Verifying container has model volume mount")
					container := firstContainer(deployment)
					modelMount := findVolumeMount(container, "model-config")
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
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
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
					r := newAgentReconciler()
					reconcileAgent(ctx, r, typeNamespacedName)

					By("Verifying container has OPENAI_API_KEY secret env var")
					deployment := getDeployment(ctx, typeNamespacedName)
					container := firstContainer(deployment)
					apiKeyEnv := findEnvVar(container, "OPENAI_API_KEY")
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
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
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
					r := newAgentReconciler()
					reconcileAgent(ctx, r, typeNamespacedName)

					By("Verifying container has ANTHROPIC_API_KEY secret env var")
					deployment := getDeployment(ctx, typeNamespacedName)
					container := firstContainer(deployment)
					apiKeyEnv := findEnvVar(container, "ANTHROPIC_API_KEY")
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
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
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
					r := newAgentReconciler()
					reconcileAgent(ctx, r, typeNamespacedName)

					By("Verifying Model ConfigMap was created with parameters")
					modelCM := getModelConfigMap(ctx, typeNamespacedName)

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
					agent = getAgent(ctx, typeNamespacedName)
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
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
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
					r := newAgentReconciler()
					reconcileAgent(ctx, r, typeNamespacedName)

					By("Verifying penalty parameters in ConfigMap")
					modelCM := getModelConfigMap(ctx, typeNamespacedName)

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
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
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
					r := newAgentReconciler()
					reconcileAgent(ctx, r, typeNamespacedName)

					By("Verifying Anthropic parameters in ConfigMap")
					modelCM := getModelConfigMap(ctx, typeNamespacedName)

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
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
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
					r := newAgentReconciler()
					reconcileAgent(ctx, r, typeNamespacedName)

					By("Verifying Model ConfigMap was created")
					modelCM := getModelConfigMap(ctx, typeNamespacedName)

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
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
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
					r := newAgentReconciler()
					reconcileAgent(ctx, r, typeNamespacedName)

					By("Verifying ConfigMap has OpenAI-specific config")
					modelCM := getModelConfigMap(ctx, typeNamespacedName)

					var providerConfig ResolvedModelConfig
					err = json.Unmarshal([]byte(modelCM.Data["model.json"]), &providerConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(providerConfig.Provider.OpenAI).NotTo(BeNil())
					Expect(providerConfig.Provider.OpenAI.BaseURL).To(Equal("https://custom.openai.api.com/v1"))

					By("Verifying Deployment has env vars for OpenAI config")
					deployment := getDeployment(ctx, typeNamespacedName)
					container := firstContainer(deployment)
					baseURLEnv := findEnvVar(container, "OPENAI_BASE_URL")
					Expect(baseURLEnv).NotTo(BeNil())
					Expect(baseURLEnv.Value).To(Equal("https://custom.openai.api.com/v1"))
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
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
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
					r := newAgentReconciler()
					reconcileAgent(ctx, r, typeNamespacedName)

					By("Verifying ConfigMap has Anthropic-specific config")
					modelCM := getModelConfigMap(ctx, typeNamespacedName)

					var providerConfig ResolvedModelConfig
					err = json.Unmarshal([]byte(modelCM.Data["model.json"]), &providerConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(providerConfig.Provider.Anthropic).NotTo(BeNil())
					Expect(providerConfig.Provider.Anthropic.BaseURL).To(Equal("https://custom.anthropic.api.com"))

					By("Verifying Deployment has ANTHROPIC_BASE_URL env var")
					deployment := getDeployment(ctx, typeNamespacedName)
					container := firstContainer(deployment)
					baseURLEnv := findEnvVar(container, "ANTHROPIC_BASE_URL")
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
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
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
					r := newAgentReconciler()
					reconcileAgent(ctx, r, typeNamespacedName)

					By("Verifying ConfigMap has default headers")
					modelCM := getModelConfigMap(ctx, typeNamespacedName)

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
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
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
					r := newAgentReconciler()

					// First reconcile adds finalizer
					_, _ = reconcileOnce(ctx, r, typeNamespacedName)

					// Second reconcile detects missing Model as dependency error
					// → requeue after fixed interval, no error returned (#96)
					result, err := reconcileOnce(ctx, r, typeNamespacedName)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.RequeueAfter).To(Equal(30 * time.Second))

					By("Verifying ModelReady condition is false")
					agent = getAgent(ctx, typeNamespacedName)
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
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
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
					r := newAgentReconciler()

					_, _ = reconcileOnce(ctx, r, typeNamespacedName)

					// Dependency error (Model not ready) → requeue after interval (#96)
					result, err := reconcileOnce(ctx, r, typeNamespacedName)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.RequeueAfter).To(Equal(30 * time.Second))

					By("Verifying ModelReady condition is false")
					agent = getAgent(ctx, typeNamespacedName)
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
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
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
					r := newAgentReconciler()
					reconcileAgent(ctx, r, typeNamespacedName)

					By("Verifying initial ConfigMap content")
					modelCM := getModelConfigMap(ctx, typeNamespacedName)

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
					_, err = reconcileOnce(ctx, r, typeNamespacedName)
					Expect(err).NotTo(HaveOccurred())

					By("Verifying ConfigMap was updated")
					// Re-discover the ConfigMap name from the deployment volume
					modelCM = getModelConfigMap(ctx, typeNamespacedName)

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
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
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
					r := newAgentReconciler()
					reconcileAgent(ctx, r, typeNamespacedName)

					By("Verifying ConfigMap labels")
					modelCM := getModelConfigMap(ctx, typeNamespacedName)

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
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
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
					r := newAgentReconciler()
					reconcileAgent(ctx, r, typeNamespacedName)

					By("Verifying ConfigMap has owner reference")
					modelCM := getModelConfigMap(ctx, typeNamespacedName)

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
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
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
					r := newAgentReconciler()
					reconcileAgent(ctx, r, typeNamespacedName)

					By("Verifying deployment does not have model volume")
					deployment := getDeployment(ctx, typeNamespacedName)
					Expect(findVolume(deployment.Spec.Template.Spec, "model-config")).To(BeNil())

					By("Verifying no ModelReady condition exists")
					agent = getAgent(ctx, typeNamespacedName)
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
