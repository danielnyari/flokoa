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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

var _ = Describe("Prompt Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			promptName = "test-prompt"
		)

		ctx := context.Background()

		promptNamespacedName := types.NamespacedName{
			Name:      promptName,
			Namespace: "default",
		}

		BeforeEach(func() {
			// Create inline Prompt
			var prompt agentv1alpha1.Prompt
			err := k8sClient.Get(ctx, promptNamespacedName, &prompt)
			if err != nil && errors.IsNotFound(err) {
				promptResource := &agentv1alpha1.Prompt{
					ObjectMeta: metav1.ObjectMeta{
						Name:      promptName,
						Namespace: "default",
					},
					Spec: agentv1alpha1.PromptSpec{
						Source: agentv1alpha1.PromptSource{
							Inline: &agentv1alpha1.InlineSource{
								Content: "You are a helpful assistant.",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, promptResource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Cleanup Prompt
			prompt := &agentv1alpha1.Prompt{}
			err := k8sClient.Get(ctx, promptNamespacedName, prompt)
			if err == nil {
				Expect(k8sClient.Delete(ctx, prompt)).To(Succeed())
			}
		})

		It("should successfully reconcile an inline prompt", func() {
			By("Reconciling the created resource")
			controllerReconciler := &PromptReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: promptNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is updated
			var updated agentv1alpha1.Prompt
			Expect(k8sClient.Get(ctx, promptNamespacedName, &updated)).To(Succeed())
			Expect(updated.Status.ResolvedContent).To(Equal("You are a helpful assistant."))
			Expect(updated.Status.SourceVersion).To(Equal("inline"))
			Expect(updated.Status.Checksum).To(HavePrefix("sha256:"))
			Expect(updated.Status.ResolvedAt).NotTo(BeNil())

			// Check Ready condition
			readyCondition := findCondition(updated.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should reject prompts with multiple sources", func() {
			By("Creating a prompt with multiple sources")
			multiSourcePrompt := &agentv1alpha1.Prompt{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-source-prompt",
					Namespace: "default",
				},
				Spec: agentv1alpha1.PromptSpec{
					Source: agentv1alpha1.PromptSource{
						Inline: &agentv1alpha1.InlineSource{
							Content: "Test content",
						},
						Langfuse: &agentv1alpha1.LangfuseSource{
							CredentialsSecretRef: corev1.LocalObjectReference{
								Name: "test-secret",
							},
							PromptName: "test-prompt",
							Version:    "1",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, multiSourcePrompt)).To(Succeed())

			By("Reconciling the resource")
			controllerReconciler := &PromptReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "multi-source-prompt",
					Namespace: "default",
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("only one of"))

			// Cleanup
			Expect(k8sClient.Delete(ctx, multiSourcePrompt)).To(Succeed())
		})
	})
})

// Helper function to find a condition by type
func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
