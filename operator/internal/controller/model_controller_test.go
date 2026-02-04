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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

var _ = Describe("Model Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			providerName = "test-provider"
			modelName    = "test-model"
		)

		ctx := context.Background()

		providerNamespacedName := types.NamespacedName{
			Name:      providerName,
			Namespace: "default",
		}
		modelNamespacedName := types.NamespacedName{
			Name:      modelName,
			Namespace: "default",
		}

		BeforeEach(func() {
			// Create ModelProvider first
			var provider agentv1alpha1.ModelProvider
			err := k8sClient.Get(ctx, providerNamespacedName, &provider)
			if err != nil && errors.IsNotFound(err) {
				timeoutSeconds := int32(120)
				providerResource := &agentv1alpha1.ModelProvider{
					ObjectMeta: metav1.ObjectMeta{
						Name:      providerName,
						Namespace: "default",
					},
					Spec: agentv1alpha1.ModelProviderSpec{
						OpenAI: &agentv1alpha1.OpenAIProviderSpec{
							TimeoutSeconds: &timeoutSeconds,
						},
					},
				}
				Expect(k8sClient.Create(ctx, providerResource)).To(Succeed())

				// Reconcile the provider to set its status
				providerReconciler := &ModelProviderReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}
				_, err = providerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: providerNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
			}

			// Create Model
			var model agentv1alpha1.Model
			err = k8sClient.Get(ctx, modelNamespacedName, &model)
			if err != nil && errors.IsNotFound(err) {
				maxTokens := int32(4096)
				modelResource := &agentv1alpha1.Model{
					ObjectMeta: metav1.ObjectMeta{
						Name:      modelName,
						Namespace: "default",
					},
					Spec: agentv1alpha1.ModelSpec{
						Model: "gpt-4o",
						ProviderRef: agentv1alpha1.ProviderRef{
							Name: providerName,
						},
						Parameters: &agentv1alpha1.ModelParameters{
							Temperature: "0.7",
							MaxTokens:   &maxTokens,
						},
					},
				}
				Expect(k8sClient.Create(ctx, modelResource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Cleanup Model
			model := &agentv1alpha1.Model{}
			err := k8sClient.Get(ctx, modelNamespacedName, model)
			if err == nil {
				Expect(k8sClient.Delete(ctx, model)).To(Succeed())
			}

			// Cleanup ModelProvider
			provider := &agentv1alpha1.ModelProvider{}
			err = k8sClient.Get(ctx, providerNamespacedName, provider)
			if err == nil {
				Expect(k8sClient.Delete(ctx, provider)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ModelReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: modelNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is updated
			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNamespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())
			Expect(updated.Status.ResolvedProvider).NotTo(BeNil())
			Expect(updated.Status.ResolvedProvider.Provider).To(Equal(agentv1alpha1.ProviderTypeOpenAI))
		})
	})

	Context("When ModelProvider is not found", func() {
		const (
			nonExistentProvider = "non-existent-provider"
			modelName           = "test-model-no-provider"
		)

		ctx := context.Background()
		modelNamespacedName := types.NamespacedName{
			Name:      modelName,
			Namespace: "default",
		}

		BeforeEach(func() {
			// Create Model referencing non-existent provider
			var model agentv1alpha1.Model
			err := k8sClient.Get(ctx, modelNamespacedName, &model)
			if err != nil && errors.IsNotFound(err) {
				maxTokens := int32(4096)
				modelResource := &agentv1alpha1.Model{
					ObjectMeta: metav1.ObjectMeta{
						Name:      modelName,
						Namespace: "default",
					},
					Spec: agentv1alpha1.ModelSpec{
						Model: "gpt-4o",
						ProviderRef: agentv1alpha1.ProviderRef{
							Name: nonExistentProvider,
						},
						Parameters: &agentv1alpha1.ModelParameters{
							Temperature: "0.7",
							MaxTokens:   &maxTokens,
						},
					},
				}
				Expect(k8sClient.Create(ctx, modelResource)).To(Succeed())
			}
		})

		AfterEach(func() {
			model := &agentv1alpha1.Model{}
			err := k8sClient.Get(ctx, modelNamespacedName, model)
			if err == nil {
				Expect(k8sClient.Delete(ctx, model)).To(Succeed())
			}
		})

		It("should set status to not ready with ProviderNotFound reason", func() {
			By("Reconciling the Model with non-existent provider")
			controllerReconciler := &ModelReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: modelNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is set to not ready
			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNamespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())
			Expect(updated.Status.ResolvedProvider).To(BeNil())

			// Check that the status condition reflects provider not found
			condition := findCondition(updated.Status.Conditions, ModelConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(ModelReasonProviderNotFound))
			Expect(condition.Message).To(ContainSubstring(nonExistentProvider))
		})
	})

	Context("When ModelProvider is not ready", func() {
		const (
			providerName = "test-provider-not-ready"
			modelName    = "test-model-provider-not-ready"
		)

		ctx := context.Background()
		providerNamespacedName := types.NamespacedName{
			Name:      providerName,
			Namespace: "default",
		}
		modelNamespacedName := types.NamespacedName{
			Name:      modelName,
			Namespace: "default",
		}

		BeforeEach(func() {
			// Create ModelProvider WITHOUT reconciling (so it's not ready)
			var provider agentv1alpha1.ModelProvider
			err := k8sClient.Get(ctx, providerNamespacedName, &provider)
			if err != nil && errors.IsNotFound(err) {
				timeoutSeconds := int32(120)
				providerResource := &agentv1alpha1.ModelProvider{
					ObjectMeta: metav1.ObjectMeta{
						Name:      providerName,
						Namespace: "default",
					},
					Spec: agentv1alpha1.ModelProviderSpec{
						OpenAI: &agentv1alpha1.OpenAIProviderSpec{
							TimeoutSeconds: &timeoutSeconds,
						},
					},
				}
				Expect(k8sClient.Create(ctx, providerResource)).To(Succeed())
				// NOTE: We do NOT reconcile the provider here, so it stays not ready
			}

			// Create Model
			var model agentv1alpha1.Model
			err = k8sClient.Get(ctx, modelNamespacedName, &model)
			if err != nil && errors.IsNotFound(err) {
				maxTokens := int32(4096)
				modelResource := &agentv1alpha1.Model{
					ObjectMeta: metav1.ObjectMeta{
						Name:      modelName,
						Namespace: "default",
					},
					Spec: agentv1alpha1.ModelSpec{
						Model: "gpt-4o",
						ProviderRef: agentv1alpha1.ProviderRef{
							Name: providerName,
						},
						Parameters: &agentv1alpha1.ModelParameters{
							Temperature: "0.7",
							MaxTokens:   &maxTokens,
						},
					},
				}
				Expect(k8sClient.Create(ctx, modelResource)).To(Succeed())
			}
		})

		AfterEach(func() {
			model := &agentv1alpha1.Model{}
			err := k8sClient.Get(ctx, modelNamespacedName, model)
			if err == nil {
				Expect(k8sClient.Delete(ctx, model)).To(Succeed())
			}

			provider := &agentv1alpha1.ModelProvider{}
			err = k8sClient.Get(ctx, providerNamespacedName, provider)
			if err == nil {
				Expect(k8sClient.Delete(ctx, provider)).To(Succeed())
			}
		})

		It("should set status to not ready with ProviderNotReady reason", func() {
			By("Reconciling the Model with not-ready provider")
			controllerReconciler := &ModelReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: modelNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is set to not ready
			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNamespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())

			// Check that the status condition reflects provider not ready
			condition := findCondition(updated.Status.Conditions, ModelConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(ModelReasonProviderNotReady))
			Expect(condition.Message).To(ContainSubstring(providerName))
		})
	})

	Context("When provider parameters mismatch", func() {
		const (
			providerName = "test-provider-params-mismatch"
			modelName    = "test-model-params-mismatch"
		)

		ctx := context.Background()
		providerNamespacedName := types.NamespacedName{
			Name:      providerName,
			Namespace: "default",
		}
		modelNamespacedName := types.NamespacedName{
			Name:      modelName,
			Namespace: "default",
		}

		BeforeEach(func() {
			// Create OpenAI ModelProvider
			var provider agentv1alpha1.ModelProvider
			err := k8sClient.Get(ctx, providerNamespacedName, &provider)
			if err != nil && errors.IsNotFound(err) {
				timeoutSeconds := int32(120)
				providerResource := &agentv1alpha1.ModelProvider{
					ObjectMeta: metav1.ObjectMeta{
						Name:      providerName,
						Namespace: "default",
					},
					Spec: agentv1alpha1.ModelProviderSpec{
						OpenAI: &agentv1alpha1.OpenAIProviderSpec{
							TimeoutSeconds: &timeoutSeconds,
						},
					},
				}
				Expect(k8sClient.Create(ctx, providerResource)).To(Succeed())

				// Reconcile the provider to set its status
				providerReconciler := &ModelProviderReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}
				_, err = providerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: providerNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
			}

			// Create Model with ANTHROPIC parameters (mismatch!)
			var model agentv1alpha1.Model
			err = k8sClient.Get(ctx, modelNamespacedName, &model)
			if err != nil && errors.IsNotFound(err) {
				maxTokens := int32(4096)
				modelResource := &agentv1alpha1.Model{
					ObjectMeta: metav1.ObjectMeta{
						Name:      modelName,
						Namespace: "default",
					},
					Spec: agentv1alpha1.ModelSpec{
						Model: "gpt-4o",
						ProviderRef: agentv1alpha1.ProviderRef{
							Name: providerName,
						},
						Parameters: &agentv1alpha1.ModelParameters{
							Temperature: "0.7",
							MaxTokens:   &maxTokens,
							Anthropic: &agentv1alpha1.AnthropicParameters{
								MetadataUserID: "test-user",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, modelResource)).To(Succeed())
			}
		})

		AfterEach(func() {
			model := &agentv1alpha1.Model{}
			err := k8sClient.Get(ctx, modelNamespacedName, model)
			if err == nil {
				Expect(k8sClient.Delete(ctx, model)).To(Succeed())
			}

			provider := &agentv1alpha1.ModelProvider{}
			err = k8sClient.Get(ctx, providerNamespacedName, provider)
			if err == nil {
				Expect(k8sClient.Delete(ctx, provider)).To(Succeed())
			}
		})

		It("should set status to not ready with ProviderParametersMismatch reason", func() {
			By("Reconciling the Model with mismatched provider parameters")
			controllerReconciler := &ModelReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: modelNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is set to not ready
			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNamespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())

			// Check that the status condition reflects parameter mismatch
			condition := findCondition(updated.Status.Conditions, ModelConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(ModelReasonProviderParamsMismatch))
			Expect(condition.Message).To(ContainSubstring("do not match"))
		})
	})

	Context("When multiple provider parameters are specified", func() {
		const (
			providerName = "test-provider-multi-params"
			modelName    = "test-model-multi-params"
		)

		ctx := context.Background()
		providerNamespacedName := types.NamespacedName{
			Name:      providerName,
			Namespace: "default",
		}
		modelNamespacedName := types.NamespacedName{
			Name:      modelName,
			Namespace: "default",
		}

		BeforeEach(func() {
			// Create OpenAI ModelProvider
			var provider agentv1alpha1.ModelProvider
			err := k8sClient.Get(ctx, providerNamespacedName, &provider)
			if err != nil && errors.IsNotFound(err) {
				timeoutSeconds := int32(120)
				providerResource := &agentv1alpha1.ModelProvider{
					ObjectMeta: metav1.ObjectMeta{
						Name:      providerName,
						Namespace: "default",
					},
					Spec: agentv1alpha1.ModelProviderSpec{
						OpenAI: &agentv1alpha1.OpenAIProviderSpec{
							TimeoutSeconds: &timeoutSeconds,
						},
					},
				}
				Expect(k8sClient.Create(ctx, providerResource)).To(Succeed())

				// Reconcile the provider to set its status
				providerReconciler := &ModelProviderReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}
				_, err = providerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: providerNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
			}

			// Create Model with BOTH OpenAI AND Anthropic parameters (invalid!)
			var model agentv1alpha1.Model
			err = k8sClient.Get(ctx, modelNamespacedName, &model)
			if err != nil && errors.IsNotFound(err) {
				maxTokens := int32(4096)
				modelResource := &agentv1alpha1.Model{
					ObjectMeta: metav1.ObjectMeta{
						Name:      modelName,
						Namespace: "default",
					},
					Spec: agentv1alpha1.ModelSpec{
						Model: "gpt-4o",
						ProviderRef: agentv1alpha1.ProviderRef{
							Name: providerName,
						},
						Parameters: &agentv1alpha1.ModelParameters{
							Temperature: "0.7",
							MaxTokens:   &maxTokens,
							OpenAI: &agentv1alpha1.OpenAIParameters{
								User: "test-user",
							},
							Anthropic: &agentv1alpha1.AnthropicParameters{
								MetadataUserID: "test-user",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, modelResource)).To(Succeed())
			}
		})

		AfterEach(func() {
			model := &agentv1alpha1.Model{}
			err := k8sClient.Get(ctx, modelNamespacedName, model)
			if err == nil {
				Expect(k8sClient.Delete(ctx, model)).To(Succeed())
			}

			provider := &agentv1alpha1.ModelProvider{}
			err = k8sClient.Get(ctx, providerNamespacedName, provider)
			if err == nil {
				Expect(k8sClient.Delete(ctx, provider)).To(Succeed())
			}
		})

		It("should set status to not ready when multiple provider params are specified", func() {
			By("Reconciling the Model with multiple provider parameters")
			controllerReconciler := &ModelReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: modelNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is set to not ready
			var updated agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNamespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())

			// Check that the status condition reflects multiple parameters error
			condition := findCondition(updated.Status.Conditions, ModelConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(ModelReasonProviderParamsMismatch))
			Expect(condition.Message).To(ContainSubstring("only one"))
		})
	})
})

// Helper function to find a condition by type
func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for _, c := range conditions {
		if c.Type == condType {
			return &c
		}
	}
	return nil
}
