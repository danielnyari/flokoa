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
				providerResource := &agentv1alpha1.ModelProvider{
					ObjectMeta: metav1.ObjectMeta{
						Name:      providerName,
						Namespace: "default",
					},
					Spec: agentv1alpha1.ModelProviderSpec{
						OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
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
})
