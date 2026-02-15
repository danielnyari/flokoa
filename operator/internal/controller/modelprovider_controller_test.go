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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

var _ = Describe("ModelProvider Controller", func() {
	const (
		namespace = "default"
		timeout   = time.Second * 10
		interval  = time.Millisecond * 250
	)

	var (
		ctx            context.Context
		resourceName   string
		namespacedName types.NamespacedName
		reconciler     *ModelProviderReconciler
	)

	deleteProvider := func(name string) {
		provider := &agentv1alpha1.ModelProvider{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, provider)
		if err == nil {
			Expect(k8sClient.Delete(ctx, provider)).To(Succeed())
		}
	}

	BeforeEach(func() {
		ctx = context.Background()
		resourceName = fmt.Sprintf("test-mp-%d", time.Now().UnixNano())
		namespacedName = types.NamespacedName{Name: resourceName, Namespace: namespace}
		reconciler = &ModelProviderReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	AfterEach(func() {
		deleteProvider(resourceName)
	})

	Context("Basic reconciliation with each provider type", func() {
		It("should successfully reconcile an OpenAI provider", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Provider).To(Equal(agentv1alpha1.ProviderTypeOpenAI))
			Expect(updated.Status.Ready).To(BeTrue())
		})

		It("should successfully reconcile an Anthropic provider", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Provider).To(Equal(agentv1alpha1.ProviderTypeAnthropic))
			Expect(updated.Status.Ready).To(BeTrue())
		})

		It("should successfully reconcile a Google provider", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					Google: &agentv1alpha1.GoogleProviderSpec{
						Project:  "my-project",
						Location: "us-central1",
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Provider).To(Equal(agentv1alpha1.ProviderTypeGoogle))
			Expect(updated.Status.Ready).To(BeTrue())
		})

		It("should successfully reconcile a Bedrock provider", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					Bedrock: &agentv1alpha1.BedrockProviderSpec{
						Region: "us-east-1",
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Provider).To(Equal(agentv1alpha1.ProviderTypeBedrock))
			Expect(updated.Status.Ready).To(BeTrue())
		})
	})

	Context("Validation failures", func() {
		It("should fail when no provider block is set", func() {
			By("Creating a ModelProvider with empty spec")
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec:       agentv1alpha1.ModelProviderSpec{},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the resource")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying status shows validation failure")
			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())
			Expect(updated.Status.Provider).To(Equal(agentv1alpha1.ProviderType("")))

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, ModelProviderConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(ModelProviderReasonNoProviderSet))
			Expect(readyCond.Message).To(ContainSubstring("exactly one"))
		})

		It("should fail when multiple provider blocks are set (OpenAI + Anthropic)", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI:    &agentv1alpha1.OpenAIProviderSpec{},
					Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())
			Expect(updated.Status.Provider).To(Equal(agentv1alpha1.ProviderType("")))

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, ModelProviderConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Message).To(ContainSubstring("found 2"))
		})

		It("should fail when three provider blocks are set", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI:    &agentv1alpha1.OpenAIProviderSpec{},
					Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
					Google:    &agentv1alpha1.GoogleProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())
			Expect(updated.Status.Provider).To(Equal(agentv1alpha1.ProviderType("")))

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, ModelProviderConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Message).To(ContainSubstring("found 3"))
		})

		It("should fail when all four provider blocks are set", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI:    &agentv1alpha1.OpenAIProviderSpec{},
					Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
					Google:    &agentv1alpha1.GoogleProviderSpec{},
					Bedrock:   &agentv1alpha1.BedrockProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeFalse())

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, ModelProviderConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Message).To(ContainSubstring("found 4"))
		})
	})

	Context("Status conditions", func() {
		It("should set both conditions on successful reconciliation", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())

			By("Verifying Validated condition")
			validatedCond := meta.FindStatusCondition(updated.Status.Conditions, ModelProviderConditionTypeValidated)
			Expect(validatedCond).NotTo(BeNil())
			Expect(validatedCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(validatedCond.Reason).To(Equal(ModelProviderReasonValidated))
			Expect(validatedCond.Message).To(ContainSubstring("openai"))

			By("Verifying Ready condition")
			readyCond := meta.FindStatusCondition(updated.Status.Conditions, ModelProviderConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal(ModelProviderReasonValidated))
		})

		It("should set both conditions to False on validation failure", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec:       agentv1alpha1.ModelProviderSpec{},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())

			validatedCond := meta.FindStatusCondition(updated.Status.Conditions, ModelProviderConditionTypeValidated)
			Expect(validatedCond).NotTo(BeNil())
			Expect(validatedCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(validatedCond.Reason).To(Equal(ModelProviderReasonNoProviderSet))

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, ModelProviderConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(ModelProviderReasonNoProviderSet))
		})

		It("should update observedGeneration on success", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))
		})

		It("should update observedGeneration on failure", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec:       agentv1alpha1.ModelProviderSpec{},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))
		})

		It("should include observedGeneration in conditions", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, ModelProviderConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.ObservedGeneration).To(Equal(updated.Generation))

			validatedCond := meta.FindStatusCondition(updated.Status.Conditions, ModelProviderConditionTypeValidated)
			Expect(validatedCond).NotTo(BeNil())
			Expect(validatedCond.ObservedGeneration).To(Equal(updated.Generation))
		})
	})

	Context("Provider with additional configuration", func() {
		It("should reconcile OpenAI provider with custom base URL", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{
						BaseURL: "https://custom-openai.example.com/v1",
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())
			Expect(updated.Status.Provider).To(Equal(agentv1alpha1.ProviderTypeOpenAI))
		})

		It("should reconcile Anthropic provider with custom base URL", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					Anthropic: &agentv1alpha1.AnthropicProviderSpec{
						BaseURL: "https://custom-anthropic.example.com",
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())
			Expect(updated.Status.Provider).To(Equal(agentv1alpha1.ProviderTypeAnthropic))
		})

		It("should reconcile provider with API key secret reference", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					APIKeySecretRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "my-api-key-secret",
						},
						Key: "api-key",
					},
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())
			Expect(updated.Status.Provider).To(Equal(agentv1alpha1.ProviderTypeOpenAI))
		})

		It("should reconcile provider with TLS configuration", func() {
			useSystemCAs := true
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{
						BaseURL: "https://custom-endpoint.example.com",
					},
					TLS: &agentv1alpha1.TLSConfig{
						InsecureSkipVerify: false,
						UseSystemCAs:       &useSystemCAs,
						CASecretRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "custom-ca-secret",
							},
							Key: "ca.crt",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())
		})

		It("should reconcile provider with default headers", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
					DefaultHeaders: map[string]string{
						"X-Custom-Header": "custom-value",
						"X-Team":          "platform",
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())
		})

		It("should reconcile Google provider with Vertex AI configuration", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					Google: &agentv1alpha1.GoogleProviderSpec{
						Project:  "my-gcp-project",
						Location: "us-central1",
						ServiceAccountKeySecretRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "gcp-sa-key",
							},
							Key: "key.json",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())
			Expect(updated.Status.Provider).To(Equal(agentv1alpha1.ProviderTypeGoogle))
		})
	})

	Context("Non-existent resource", func() {
		It("should handle reconcile request for non-existent ModelProvider", func() {
			nonExistentName := types.NamespacedName{
				Name:      "non-existent-provider",
				Namespace: namespace,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nonExistentName})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Idempotent reconciliation", func() {
		It("should produce the same result on repeated reconciliation", func() {
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("First reconciliation")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var first agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &first)).To(Succeed())
			Expect(first.Status.Ready).To(BeTrue())
			Expect(first.Status.Provider).To(Equal(agentv1alpha1.ProviderTypeOpenAI))

			By("Second reconciliation")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			var second agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &second)).To(Succeed())
			Expect(second.Status.Ready).To(BeTrue())
			Expect(second.Status.Provider).To(Equal(first.Status.Provider))
		})
	})

	Context("Multiple models referencing the same provider", func() {
		It("should allow multiple Models to reference the same ready provider", func() {
			By("Creating a provider")
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying provider is ready")
			var updated agentv1alpha1.ModelProvider
			Expect(k8sClient.Get(ctx, namespacedName, &updated)).To(Succeed())
			Expect(updated.Status.Ready).To(BeTrue())

			By("Creating multiple Models referencing this provider")
			modelReconciler := &ModelReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			modelNames := make([]string, 3)
			for i := 0; i < 3; i++ {
				modelNames[i] = fmt.Sprintf("model-%s-%d", resourceName, i)
				model := &agentv1alpha1.Model{
					ObjectMeta: metav1.ObjectMeta{Name: modelNames[i], Namespace: namespace},
					Spec: agentv1alpha1.ModelSpec{
						Model:       fmt.Sprintf("model-variant-%d", i),
						ProviderRef: agentv1alpha1.ProviderRef{Name: resourceName},
					},
				}
				Expect(k8sClient.Create(ctx, model)).To(Succeed())

				modelNSName := types.NamespacedName{Name: modelNames[i], Namespace: namespace}
				_, err := modelReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
				Expect(err).NotTo(HaveOccurred())
			}

			By("Verifying all models are ready")
			for _, mn := range modelNames {
				var model agentv1alpha1.Model
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mn, Namespace: namespace}, &model)).To(Succeed())
				Expect(model.Status.Ready).To(BeTrue())
			}

			// Cleanup models
			for _, mn := range modelNames {
				m := &agentv1alpha1.Model{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: mn, Namespace: namespace}, m); err == nil {
					Expect(k8sClient.Delete(ctx, m)).To(Succeed())
				}
			}
		})
	})

	Context("Provider deletion with existing Model references", func() {
		It("should allow provider deletion and cause dependent Models to become not-ready", func() {
			By("Creating provider and reconciling it")
			resource := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Creating a Model referencing this provider")
			modelName := fmt.Sprintf("model-for-%s", resourceName)
			modelNSName := types.NamespacedName{Name: modelName, Namespace: namespace}
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: resourceName},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			modelReconciler := &ModelReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err = modelReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			var updatedModel agentv1alpha1.Model
			Expect(k8sClient.Get(ctx, modelNSName, &updatedModel)).To(Succeed())
			Expect(updatedModel.Status.Ready).To(BeTrue())

			By("Deleting the provider")
			deleteProvider(resourceName)

			Eventually(func() bool {
				p := &agentv1alpha1.ModelProvider{}
				err := k8sClient.Get(ctx, namespacedName, p)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			By("Re-reconciling the Model")
			_, err = modelReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNSName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Model is now not-ready")
			Expect(k8sClient.Get(ctx, modelNSName, &updatedModel)).To(Succeed())
			Expect(updatedModel.Status.Ready).To(BeFalse())

			condition := meta.FindStatusCondition(updatedModel.Status.Conditions, ModelConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Reason).To(Equal(ModelReasonProviderNotFound))

			// Cleanup model
			m := &agentv1alpha1.Model{}
			if err := k8sClient.Get(ctx, modelNSName, m); err == nil {
				Expect(k8sClient.Delete(ctx, m)).To(Succeed())
			}
		})
	})
})
