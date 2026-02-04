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

var _ = Describe("ModelProvider Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-modelprovider"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		modelprovider := &agentv1alpha1.ModelProvider{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ModelProvider")
			err := k8sClient.Get(ctx, typeNamespacedName, modelprovider)
			if err != nil && errors.IsNotFound(err) {
				timeoutSeconds := int32(120)
				resource := &agentv1alpha1.ModelProvider{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: agentv1alpha1.ModelProviderSpec{
						OpenAI: &agentv1alpha1.OpenAIProviderSpec{
							TimeoutSeconds: &timeoutSeconds,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &agentv1alpha1.ModelProvider{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ModelProvider")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ModelProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is updated
			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, typeNamespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Provider).To(Equal(agentv1alpha1.ProviderTypeOpenAI))
			Expect(updated.Status.Ready).To(BeTrue())
		})
	})

	Context("When reconciling Anthropic provider", func() {
		const resourceName = "test-anthropic-provider"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			var provider agentv1alpha1.ModelProvider
			err := k8sClient.Get(ctx, typeNamespacedName, &provider)
			if err != nil && errors.IsNotFound(err) {
				timeoutSeconds := int32(60)
				resource := &agentv1alpha1.ModelProvider{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: agentv1alpha1.ModelProviderSpec{
						Anthropic: &agentv1alpha1.AnthropicProviderSpec{
							TimeoutSeconds: &timeoutSeconds,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &agentv1alpha1.ModelProvider{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should identify provider type as Anthropic", func() {
			By("Reconciling the Anthropic provider")
			controllerReconciler := &ModelProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is updated with Anthropic provider
			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, typeNamespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Provider).To(Equal(agentv1alpha1.ProviderTypeAnthropic))
			Expect(updated.Status.Ready).To(BeTrue())
		})
	})

	Context("When reconciling Google provider", func() {
		const resourceName = "test-google-provider"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			var provider agentv1alpha1.ModelProvider
			err := k8sClient.Get(ctx, typeNamespacedName, &provider)
			if err != nil && errors.IsNotFound(err) {
				resource := &agentv1alpha1.ModelProvider{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: agentv1alpha1.ModelProviderSpec{
						Google: &agentv1alpha1.GoogleProviderSpec{
							Project: "test-project",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &agentv1alpha1.ModelProvider{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should identify provider type as Google", func() {
			By("Reconciling the Google provider")
			controllerReconciler := &ModelProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is updated with Google provider
			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, typeNamespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Provider).To(Equal(agentv1alpha1.ProviderTypeGoogle))
			Expect(updated.Status.Ready).To(BeTrue())
		})
	})

	Context("When reconciling Bedrock provider", func() {
		const resourceName = "test-bedrock-provider"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			var provider agentv1alpha1.ModelProvider
			err := k8sClient.Get(ctx, typeNamespacedName, &provider)
			if err != nil && errors.IsNotFound(err) {
				resource := &agentv1alpha1.ModelProvider{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: agentv1alpha1.ModelProviderSpec{
						Bedrock: &agentv1alpha1.BedrockProviderSpec{
							Region: "us-east-1",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &agentv1alpha1.ModelProvider{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should identify provider type as Bedrock", func() {
			By("Reconciling the Bedrock provider")
			controllerReconciler := &ModelProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is updated with Bedrock provider
			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, typeNamespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Provider).To(Equal(agentv1alpha1.ProviderTypeBedrock))
			Expect(updated.Status.Ready).To(BeTrue())
		})
	})

	Context("When no provider is specified", func() {
		const resourceName = "test-no-provider"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			var provider agentv1alpha1.ModelProvider
			err := k8sClient.Get(ctx, typeNamespacedName, &provider)
			if err != nil && errors.IsNotFound(err) {
				// Create provider with NO provider specs
				resource := &agentv1alpha1.ModelProvider{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: agentv1alpha1.ModelProviderSpec{
						// No provider specified!
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &agentv1alpha1.ModelProvider{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set status to not ready with NoProviderSet reason", func() {
			By("Reconciling the provider with no provider specified")
			controllerReconciler := &ModelProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is set to not ready
			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, typeNamespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())
			Expect(updated.Status.Provider).To(BeEmpty())

			// Check that the status condition reflects no provider set
			condition := findConditionInProvider(updated.Status.Conditions, ModelProviderConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(ModelProviderReasonNoProviderSet))
			Expect(condition.Message).To(ContainSubstring("exactly one"))
		})
	})

	Context("When multiple providers are specified", func() {
		const resourceName = "test-multiple-providers"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			var provider agentv1alpha1.ModelProvider
			err := k8sClient.Get(ctx, typeNamespacedName, &provider)
			if err != nil && errors.IsNotFound(err) {
				timeoutSeconds := int32(120)
				timeoutSecondsAnthropic := int32(60)
				// Create provider with MULTIPLE provider specs (invalid!)
				resource := &agentv1alpha1.ModelProvider{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: agentv1alpha1.ModelProviderSpec{
						OpenAI: &agentv1alpha1.OpenAIProviderSpec{
							TimeoutSeconds: &timeoutSeconds,
						},
						Anthropic: &agentv1alpha1.AnthropicProviderSpec{
							TimeoutSeconds: &timeoutSecondsAnthropic,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &agentv1alpha1.ModelProvider{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set status to not ready when multiple providers are specified", func() {
			By("Reconciling the provider with multiple providers")
			controllerReconciler := &ModelProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is set to not ready
			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, typeNamespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())
			Expect(updated.Status.Provider).To(BeEmpty())

			// Check that the status condition reflects multiple providers error
			condition := findConditionInProvider(updated.Status.Conditions, ModelProviderConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(ModelProviderReasonNoProviderSet))
			Expect(condition.Message).To(ContainSubstring("only one"))
		})
	})
})

// Helper function to find a condition by type in ModelProvider
func findConditionInProvider(conditions []metav1.Condition, condType string) *metav1.Condition {
	for _, c := range conditions {
		if c.Type == condType {
			return &c
		}
	}
	return nil
}
