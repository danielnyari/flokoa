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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

var _ = Describe("Agent Controller - Model", func() {
	Context("When reconciling an Agent with Model", func() {
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
