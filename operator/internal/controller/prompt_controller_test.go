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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

var _ = Describe("Prompt Controller", func() {
	Context("When reconciling a Prompt resource", func() {
		const (
			promptNamespace = "default"
			timeout         = time.Second * 10
			interval        = time.Millisecond * 250
		)

		var (
			ctx                context.Context
			promptName         string
			typeNamespacedName types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			promptName = fmt.Sprintf("test-prompt-%d", time.Now().UnixNano())
			typeNamespacedName = types.NamespacedName{
				Name:      promptName,
				Namespace: promptNamespace,
			}
		})

		AfterEach(func() {
			prompt := &agentv1alpha1.Prompt{}
			err := k8sClient.Get(ctx, typeNamespacedName, prompt)
			if err == nil {
				// Remove finalizer for cleanup
				if controllerutil.ContainsFinalizer(prompt, promptFinalizer) {
					controllerutil.RemoveFinalizer(prompt, promptFinalizer)
					_ = k8sClient.Update(ctx, prompt)
				}
				_ = k8sClient.Delete(ctx, prompt)
			}
		})

		It("should create a ConfigMap with prompt.txt containing inline template content", func() {
			By("Creating a Prompt resource with inline value")
			templateContent := "Hello {{ name }}, welcome to {{ company }}."
			prompt := &agentv1alpha1.Prompt{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptName,
					Namespace: promptNamespace,
				},
				Spec: agentv1alpha1.PromptSpec{
					Source: agentv1alpha1.PromptSource{
						Value: &templateContent,
					},
					Variables: []agentv1alpha1.PromptVariable{
						{Name: "name", Description: "Customer name", Required: true},
						{Name: "company", Default: "Acme Corp"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, prompt)).To(Succeed())

			controllerReconciler := &PromptReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Second reconcile creates ConfigMap
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the ConfigMap was created with prompt.txt")
			configMapName := fmt.Sprintf("%s-prompt", promptName)
			cm := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: promptNamespace,
				}, cm)
			}, timeout, interval).Should(Succeed())

			Expect(cm.Data).To(HaveKey(promptConfigMapKey))
			Expect(cm.Data[promptConfigMapKey]).To(Equal(templateContent))
			Expect(cm.Labels["app.kubernetes.io/component"]).To(Equal("prompt"))
			Expect(cm.Labels["app.kubernetes.io/managed-by"]).To(Equal("flokoa-operator"))

			By("Verifying the Prompt status")
			err = k8sClient.Get(ctx, typeNamespacedName, prompt)
			Expect(err).NotTo(HaveOccurred())
			Expect(prompt.Status.ConfigMapName).To(Equal(configMapName))
			Expect(prompt.Status.ObservedGeneration).To(Equal(prompt.Generation))

			storedCond := meta.FindStatusCondition(prompt.Status.Conditions, ConditionTypePromptStored)
			Expect(storedCond).NotTo(BeNil())
			Expect(storedCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(storedCond.Reason).To(Equal(ReasonPromptStored))
		})

		It("should resolve template content from a ConfigMap source", func() {
			By("Creating a source ConfigMap")
			sourceContent := "Dear {{ recipient }}, your order {{ order_id }} is ready."
			sourceCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-source", promptName),
					Namespace: promptNamespace,
				},
				Data: map[string]string{
					"template.txt": sourceContent,
				},
			}
			Expect(k8sClient.Create(ctx, sourceCM)).To(Succeed())

			By("Creating a Prompt resource with valueFrom")
			prompt := &agentv1alpha1.Prompt{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptName,
					Namespace: promptNamespace,
				},
				Spec: agentv1alpha1.PromptSpec{
					Source: agentv1alpha1.PromptSource{
						ValueFrom: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: fmt.Sprintf("%s-source", promptName),
							},
							Key: "template.txt",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, prompt)).To(Succeed())

			controllerReconciler := &PromptReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile resolves source and creates ConfigMap
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the ConfigMap was created with resolved content")
			configMapName := fmt.Sprintf("%s-prompt", promptName)
			cm := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: promptNamespace,
				}, cm)
			}, timeout, interval).Should(Succeed())

			Expect(cm.Data[promptConfigMapKey]).To(Equal(sourceContent))

			// Cleanup source ConfigMap
			_ = k8sClient.Delete(ctx, sourceCM)
		})

		It("should update the ConfigMap when prompt template content changes", func() {
			By("Creating a Prompt resource")
			originalContent := "Original template: {{ var1 }}"
			prompt := &agentv1alpha1.Prompt{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptName,
					Namespace: promptNamespace,
				},
				Spec: agentv1alpha1.PromptSpec{
					Source: agentv1alpha1.PromptSource{
						Value: &originalContent,
					},
				},
			}
			Expect(k8sClient.Create(ctx, prompt)).To(Succeed())

			controllerReconciler := &PromptReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile creates ConfigMap
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Updating the prompt template content")
			err = k8sClient.Get(ctx, typeNamespacedName, prompt)
			Expect(err).NotTo(HaveOccurred())
			updatedContent := "Updated template: {{ var1 }} and {{ var2 }}"
			prompt.Spec.Source.Value = &updatedContent
			Expect(k8sClient.Update(ctx, prompt)).To(Succeed())

			// Reconcile to update ConfigMap
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the ConfigMap was updated")
			configMapName := fmt.Sprintf("%s-prompt", promptName)
			cm := &corev1.ConfigMap{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: promptNamespace,
				}, cm)
				if err != nil {
					return ""
				}
				return cm.Data[promptConfigMapKey]
			}, timeout, interval).Should(Equal(updatedContent))
		})

		It("should handle deletion with finalizer cleanup", func() {
			By("Creating a Prompt resource")
			templateContent := "Temporary template: {{ placeholder }}"
			prompt := &agentv1alpha1.Prompt{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptName,
					Namespace: promptNamespace,
				},
				Spec: agentv1alpha1.PromptSpec{
					Source: agentv1alpha1.PromptSource{
						Value: &templateContent,
					},
				},
			}
			Expect(k8sClient.Create(ctx, prompt)).To(Succeed())

			controllerReconciler := &PromptReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile creates ConfigMap
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the finalizer was added")
			err = k8sClient.Get(ctx, typeNamespacedName, prompt)
			Expect(err).NotTo(HaveOccurred())
			Expect(controllerutil.ContainsFinalizer(prompt, promptFinalizer)).To(BeTrue())

			By("Deleting the Prompt")
			Expect(k8sClient.Delete(ctx, prompt)).To(Succeed())

			// Reconcile to process deletion
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
