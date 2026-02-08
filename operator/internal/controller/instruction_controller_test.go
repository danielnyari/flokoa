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

var _ = Describe("Instruction Controller", func() {
	Context("When reconciling an Instruction resource", func() {
		const (
			instructionNamespace = "default"
			timeout              = time.Second * 10
			interval             = time.Millisecond * 250
		)

		var (
			ctx                context.Context
			instructionName    string
			typeNamespacedName types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			instructionName = fmt.Sprintf("test-instruction-%d", time.Now().UnixNano())
			typeNamespacedName = types.NamespacedName{
				Name:      instructionName,
				Namespace: instructionNamespace,
			}
		})

		AfterEach(func() {
			instruction := &agentv1alpha1.Instruction{}
			err := k8sClient.Get(ctx, typeNamespacedName, instruction)
			if err == nil {
				// Remove finalizer for cleanup
				if controllerutil.ContainsFinalizer(instruction, instructionFinalizer) {
					controllerutil.RemoveFinalizer(instruction, instructionFinalizer)
					_ = k8sClient.Update(ctx, instruction)
				}
				_ = k8sClient.Delete(ctx, instruction)
			}
		})

		It("should create a ConfigMap with instruction.txt containing the instruction content", func() {
			By("Creating an Instruction resource")
			instruction := &agentv1alpha1.Instruction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instructionName,
					Namespace: instructionNamespace,
				},
				Spec: agentv1alpha1.InstructionSpec{
					Content: "You are a helpful assistant. Be concise and accurate.",
				},
			}
			Expect(k8sClient.Create(ctx, instruction)).To(Succeed())

			controllerReconciler := &InstructionReconciler{
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

			By("Verifying the ConfigMap was created with instruction.txt")
			configMapName := fmt.Sprintf("%s-instruction", instructionName)
			cm := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: instructionNamespace,
				}, cm)
			}, timeout, interval).Should(Succeed())

			Expect(cm.Data).To(HaveKey(instructionConfigMapKey))
			Expect(cm.Data[instructionConfigMapKey]).To(Equal("You are a helpful assistant. Be concise and accurate."))
			Expect(cm.Labels["app.kubernetes.io/component"]).To(Equal("instruction"))
			Expect(cm.Labels["app.kubernetes.io/managed-by"]).To(Equal("flokoa-operator"))

			By("Verifying the Instruction status")
			err = k8sClient.Get(ctx, typeNamespacedName, instruction)
			Expect(err).NotTo(HaveOccurred())
			Expect(instruction.Status.ConfigMapName).To(Equal(configMapName))
			Expect(instruction.Status.ObservedGeneration).To(Equal(instruction.Generation))

			storedCond := meta.FindStatusCondition(instruction.Status.Conditions, ConditionTypeInstructionStored)
			Expect(storedCond).NotTo(BeNil())
			Expect(storedCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(storedCond.Reason).To(Equal(ReasonInstructionStored))
		})

		It("should update the ConfigMap when instruction content changes", func() {
			By("Creating an Instruction resource")
			instruction := &agentv1alpha1.Instruction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instructionName,
					Namespace: instructionNamespace,
				},
				Spec: agentv1alpha1.InstructionSpec{
					Content: "Original instructions.",
				},
			}
			Expect(k8sClient.Create(ctx, instruction)).To(Succeed())

			controllerReconciler := &InstructionReconciler{
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

			By("Updating the instruction content")
			err = k8sClient.Get(ctx, typeNamespacedName, instruction)
			Expect(err).NotTo(HaveOccurred())
			instruction.Spec.Content = "Updated instructions with new behavior."
			Expect(k8sClient.Update(ctx, instruction)).To(Succeed())

			// Reconcile to update ConfigMap
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the ConfigMap was updated")
			configMapName := fmt.Sprintf("%s-instruction", instructionName)
			cm := &corev1.ConfigMap{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: instructionNamespace,
				}, cm)
				if err != nil {
					return ""
				}
				return cm.Data[instructionConfigMapKey]
			}, timeout, interval).Should(Equal("Updated instructions with new behavior."))
		})

		It("should handle deletion with finalizer cleanup", func() {
			By("Creating an Instruction resource")
			instruction := &agentv1alpha1.Instruction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instructionName,
					Namespace: instructionNamespace,
				},
				Spec: agentv1alpha1.InstructionSpec{
					Content: "Temporary instructions.",
				},
			}
			Expect(k8sClient.Create(ctx, instruction)).To(Succeed())

			controllerReconciler := &InstructionReconciler{
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
			err = k8sClient.Get(ctx, typeNamespacedName, instruction)
			Expect(err).NotTo(HaveOccurred())
			Expect(controllerutil.ContainsFinalizer(instruction, instructionFinalizer)).To(BeTrue())

			By("Deleting the Instruction")
			Expect(k8sClient.Delete(ctx, instruction)).To(Succeed())

			// Reconcile to process deletion
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
