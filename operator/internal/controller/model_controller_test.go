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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

var _ = Describe("Model Controller", func() {
	const (
		namespace = "default"
		timeout   = time.Second * 10
		interval  = time.Millisecond * 250
	)

	var (
		ctx            context.Context
		providerName   string
		modelName      string
		providerNSName types.NamespacedName
		modelNSName    types.NamespacedName
		reconciler     *ModelReconciler
	)

	// createProvider creates a ModelProvider with the given spec and reconciles it
	createProvider := func(name string, spec agentv1alpha1.ModelProviderSpec) {
		provider := &agentv1alpha1.ModelProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: spec,
		}
		Expect(k8sClient.Create(ctx, provider)).To(Succeed())

		providerReconciler := &ModelProviderReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
		_, err := providerReconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
		})
		Expect(err).NotTo(HaveOccurred())
	}

	// deleteResource deletes a resource if it exists
	deleteProvider := func(name string) {
		provider := &agentv1alpha1.ModelProvider{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, provider)
		if err == nil {
			Expect(k8sClient.Delete(ctx, provider)).To(Succeed())
		}
	}

	deleteModel := func(name string) {
		model := &agentv1alpha1.Model{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, model)
		if err == nil {
			Expect(k8sClient.Delete(ctx, model)).To(Succeed())
		}
	}

	BeforeEach(func() {
		ctx = context.Background()
		providerName = fmt.Sprintf("test-provider-%d", time.Now().UnixNano())
		modelName = fmt.Sprintf("test-model-%d", time.Now().UnixNano())
		providerNSName = types.NamespacedName{Name: providerName, Namespace: namespace}
		modelNSName = types.NamespacedName{Name: modelName, Namespace: namespace}
		reconciler = &ModelReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	AfterEach(func() {
		deleteModel(modelName)
		deleteProvider(providerName)
	})

	Context("Basic reconciliation", func() {
		It("should successfully reconcile a Model with an OpenAI provider", func() {
			By("Creating a ModelProvider")
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
			})

			By("Creating a Model referencing the provider")
			maxTokens := int32(4096)
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
					Parameters: &agentv1alpha1.ModelParameters{
						Temperature: "0.7",
						MaxTokens:   &maxTokens,
					},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			By("Reconciling the Model")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying status is updated correctly")
			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())
			Expect(updated.Status.ResolvedProvider).NotTo(BeNil())
			Expect(updated.Status.ResolvedProvider.Provider).To(Equal(agentv1alpha1.ProviderTypeOpenAI))
			Expect(updated.Status.ResolvedProvider.Name).To(Equal(providerName))
			Expect(updated.Status.ResolvedProvider.Namespace).To(Equal(namespace))
		})

		It("should reconcile a Model without parameters", func() {
			By("Creating a ModelProvider")
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
			})

			By("Creating a Model with no parameters")
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o-mini",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			By("Reconciling the Model")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())
		})
	})

	Context("Provider type resolution", func() {
		It("should resolve Anthropic provider type", func() {
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
			})

			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "claude-sonnet-4-20250514",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())
			Expect(updated.Status.ResolvedProvider.Provider).To(Equal(agentv1alpha1.ProviderTypeAnthropic))
		})

		It("should resolve Google provider type", func() {
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				Google: &agentv1alpha1.GoogleProviderSpec{},
			})

			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gemini-2.0-flash",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())
			Expect(updated.Status.ResolvedProvider.Provider).To(Equal(agentv1alpha1.ProviderTypeGoogle))
		})

		It("should resolve Bedrock provider type", func() {
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				Bedrock: &agentv1alpha1.BedrockProviderSpec{Region: "us-east-1"},
			})

			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "anthropic.claude-3-5-sonnet-20241022-v2:0",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())
			Expect(updated.Status.ResolvedProvider.Provider).To(Equal(agentv1alpha1.ProviderTypeBedrock))
		})
	})

	Context("Invalid provider reference", func() {
		It("should set not-ready when provider does not exist", func() {
			By("Creating a Model referencing a non-existent provider")
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: "non-existent-provider"},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			By("Reconciling the Model")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying status shows ProviderNotFound")
			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())

			condition := meta.FindStatusCondition(updated.Status.Conditions, ModelConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(ModelReasonProviderNotFound))
			Expect(condition.Message).To(ContainSubstring("non-existent-provider"))
		})

		It("should set not-ready when provider is not ready", func() {
			By("Creating a ModelProvider without reconciling it (so it stays not-ready)")
			provider := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: providerName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())
			// Deliberately NOT reconciling the provider - it stays not-ready

			By("Creating a Model referencing the not-ready provider")
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			By("Reconciling the Model")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying status shows ProviderNotReady")
			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())

			condition := meta.FindStatusCondition(updated.Status.Conditions, ModelConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(ModelReasonProviderNotReady))
		})
	})

	Context("Provider parameter validation", func() {
		It("should reject OpenAI params with Anthropic provider", func() {
			By("Creating an Anthropic provider")
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
			})

			By("Creating a Model with OpenAI-specific parameters")
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "claude-sonnet-4-20250514",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
					Parameters: &agentv1alpha1.ModelParameters{
						OpenAI: &agentv1alpha1.OpenAIParameters{
							ServiceTier: "auto",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())

			condition := meta.FindStatusCondition(updated.Status.Conditions, ModelConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Reason).To(Equal(ModelReasonProviderParamsMismatch))
			Expect(condition.Message).To(ContainSubstring("openai"))
			Expect(condition.Message).To(ContainSubstring("anthropic"))
		})

		It("should reject Anthropic params with OpenAI provider", func() {
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
			})

			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
					Parameters: &agentv1alpha1.ModelParameters{
						Anthropic: &agentv1alpha1.AnthropicParameters{
							MetadataUserID: "test-user",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())

			condition := meta.FindStatusCondition(updated.Status.Conditions, ModelConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Reason).To(Equal(ModelReasonProviderParamsMismatch))
		})

		It("should reject Google params with Bedrock provider", func() {
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				Bedrock: &agentv1alpha1.BedrockProviderSpec{Region: "us-west-2"},
			})

			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "anthropic.claude-3-5-sonnet",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
					Parameters: &agentv1alpha1.ModelParameters{
						Google: &agentv1alpha1.GoogleParameters{
							CachedContent: "some-cache",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())

			condition := meta.FindStatusCondition(updated.Status.Conditions, ModelConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Reason).To(Equal(ModelReasonProviderParamsMismatch))
		})

		It("should reject multiple provider-specific parameter blocks", func() {
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
			})

			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
					Parameters: &agentv1alpha1.ModelParameters{
						OpenAI: &agentv1alpha1.OpenAIParameters{
							ServiceTier: "auto",
						},
						Anthropic: &agentv1alpha1.AnthropicParameters{
							MetadataUserID: "user",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())

			condition := meta.FindStatusCondition(updated.Status.Conditions, ModelConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Reason).To(Equal(ModelReasonProviderParamsMismatch))
			Expect(condition.Message).To(ContainSubstring("found 2"))
		})

		It("should accept matching provider-specific parameters", func() {
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
			})

			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
					Parameters: &agentv1alpha1.ModelParameters{
						Temperature: "0.5",
						OpenAI: &agentv1alpha1.OpenAIParameters{
							ServiceTier: "auto",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())
		})

		It("should accept base parameters only without provider-specific params", func() {
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
			})

			maxTokens := int32(2048)
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "claude-sonnet-4-20250514",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
					Parameters: &agentv1alpha1.ModelParameters{
						Temperature: "1.0",
						MaxTokens:   &maxTokens,
						TopP:        "0.9",
					},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())
		})
	})

	Context("Status conditions", func() {
		It("should set all three conditions on successful reconciliation", func() {
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
			})

			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())

			By("Verifying ProviderResolved condition")
			providerCond := meta.FindStatusCondition(updated.Status.Conditions, ModelConditionTypeProviderResolved)
			Expect(providerCond).NotTo(BeNil())
			Expect(providerCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(providerCond.Reason).To(Equal(ModelReasonResolved))
			Expect(providerCond.Message).To(ContainSubstring("openai"))

			By("Verifying Validated condition")
			validatedCond := meta.FindStatusCondition(updated.Status.Conditions, ModelConditionTypeValidated)
			Expect(validatedCond).NotTo(BeNil())
			Expect(validatedCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(validatedCond.Reason).To(Equal(ModelReasonValidated))

			By("Verifying Ready condition")
			readyCond := meta.FindStatusCondition(updated.Status.Conditions, ModelConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal(ModelReasonResolved))
		})

		It("should set Ready=False with ProviderNotFound when provider is missing", func() {
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: "missing-provider"},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(modelRetryInterval))

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, ModelConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(ModelReasonProviderNotFound))
		})

		It("should requeue when provider exists but is not ready", func() {
			provider := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: providerName, Namespace: namespace},
				Spec:       agentv1alpha1.ModelProviderSpec{OpenAI: &agentv1alpha1.OpenAIProviderSpec{}},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(modelRetryInterval))

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, ModelConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(ModelReasonProviderNotReady))
		})

		It("should update observedGeneration", func() {
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
			})

			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))
		})

		It("should update observedGeneration even on failure", func() {
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: "missing"},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))
		})
	})

	Context("Status transitions", func() {
		It("should transition from not-ready to ready when provider becomes available", func() {
			By("Creating a Model referencing a non-existent provider")
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			By("First reconciliation - provider not found")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())

			By("Creating the provider")
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
			})

			By("Re-reconciling the Model")
			// Need to re-fetch the model to get the latest version
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying transition to ready")
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, ModelConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should transition from ready to not-ready when provider is deleted", func() {
			By("Creating provider and model, and reconciling to ready state")
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
			})

			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())

			By("Deleting the provider")
			deleteProvider(providerName)

			Eventually(func() bool {
				provider := &agentv1alpha1.ModelProvider{}
				err := k8sClient.Get(ctx, providerNSName, provider)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			By("Re-reconciling the Model")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying transition to not-ready")
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, ModelConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(ModelReasonProviderNotFound))
		})
	})

	Context("Non-existent resource", func() {
		It("should handle reconcile request for non-existent Model", func() {
			nonExistentName := types.NamespacedName{
				Name:      "non-existent-model",
				Namespace: namespace,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nonExistentName})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Multiple models referencing the same provider", func() {
		It("should independently reconcile multiple models with the same provider", func() {
			By("Creating a shared provider")
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
			})

			model2Name := fmt.Sprintf("test-model-2-%d", time.Now().UnixNano())
			model2NSName := types.NamespacedName{Name: model2Name, Namespace: namespace}

			By("Creating two Models referencing the same provider")
			model1 := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
				},
			}
			model2 := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: model2Name, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o-mini",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
				},
			}
			Expect(k8sClient.Create(ctx, model1)).To(Succeed())
			Expect(k8sClient.Create(ctx, model2)).To(Succeed())

			By("Reconciling both models")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: model2NSName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying both models are ready")
			var updated1 agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated1)).To(Succeed())
			Expect(updated1.Status.Ready).To(BeTrue())

			var updated2 agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, model2NSName, &updated2)).To(Succeed())
			Expect(updated2.Status.Ready).To(BeTrue())

			// Cleanup second model
			defer deleteModel(model2Name)
		})
	})

	Context("Provider namespace resolution", func() {
		It("should default provider namespace to model namespace when not specified", func() {
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
			})

			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model: "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{
						Name: providerName,
						// Namespace deliberately omitted
					},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())
			Expect(updated.Status.ResolvedProvider.Namespace).To(Equal(namespace))
		})

		It("should use explicit provider namespace when specified", func() {
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
			})

			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model: "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{
						Name:      providerName,
						Namespace: namespace,
					},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())
			Expect(updated.Status.ResolvedProvider.Namespace).To(Equal(namespace))
		})

		It("should fail when explicit provider namespace points to non-existent provider", func() {
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model: "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{
						Name:      "some-provider",
						Namespace: "non-existent-namespace",
					},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())

			condition := meta.FindStatusCondition(updated.Status.Conditions, ModelConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Reason).To(Equal(ModelReasonProviderNotFound))
		})
	})

	Context("Idempotent reconciliation", func() {
		It("should produce the same result on repeated reconciliation", func() {
			createProvider(providerName, agentv1alpha1.ModelProviderSpec{
				OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
			})

			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			By("First reconciliation")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var first agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &first)).To(Succeed())
			Expect(first.Status.Ready).To(BeTrue())

			By("Second reconciliation")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var second agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &second)).To(Succeed())
			Expect(second.Status.Ready).To(BeTrue())
			Expect(second.Status.ResolvedProvider.Provider).To(Equal(first.Status.ResolvedProvider.Provider))
			Expect(second.Status.ResolvedProvider.Name).To(Equal(first.Status.ResolvedProvider.Name))
		})
	})
})
