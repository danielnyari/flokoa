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
	agentdomain "github.com/danielnyari/flokoa/internal/domain/agent"
)

// Integration tests verify cross-controller resource flows end-to-end (fixes #98).
var _ = Describe("Cross-Controller Integration", func() {
	const (
		namespace = "default"
		timeout   = time.Second * 10
		interval  = time.Millisecond * 250
	)

	Context("Instruction → ConfigMap flow", func() {
		var (
			ctx             context.Context
			instructionName string
			instNN          types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			instructionName = fmt.Sprintf("integ-inst-%d", time.Now().UnixNano())
			instNN = types.NamespacedName{Name: instructionName, Namespace: namespace}
		})

		AfterEach(func() {
			instruction := &agentv1alpha1.Instruction{}
			if err := k8sClient.Get(ctx, instNN, instruction); err == nil {
				if controllerutil.ContainsFinalizer(instruction, instructionFinalizer) {
					controllerutil.RemoveFinalizer(instruction, instructionFinalizer)
					_ = k8sClient.Update(ctx, instruction)
				}
				_ = k8sClient.Delete(ctx, instruction)
			}
		})

		It("should create a ConfigMap containing instruction content", func() {
			By("Creating an Instruction resource")
			instruction := &agentv1alpha1.Instruction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instructionName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.InstructionSpec{
					Content: "You are a helpful coding assistant.",
				},
			}
			Expect(k8sClient.Create(ctx, instruction)).To(Succeed())

			By("Reconciling the Instruction (finalizer + ConfigMap)")
			r := &InstructionReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// First reconcile adds finalizer
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: instNN})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Second reconcile creates ConfigMap
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: instNN})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the ConfigMap was created with correct content")
			cmName := fmt.Sprintf("%s-instruction", instructionName)
			cm := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: namespace}, cm)
			}, timeout, interval).Should(Succeed())

			Expect(cm.Data).To(HaveKey(instructionConfigMapKey))
			Expect(cm.Data[instructionConfigMapKey]).To(Equal("You are a helpful coding assistant."))

			By("Verifying Instruction status reflects stored state")
			Expect(k8sClient.Get(ctx, instNN, instruction)).To(Succeed())
			Expect(instruction.Status.ConfigMapName).To(Equal(cmName))
			Expect(instruction.Status.ObservedGeneration).To(Equal(instruction.Generation))

			storedCond := meta.FindStatusCondition(instruction.Status.Conditions, ConditionTypeInstructionStored)
			Expect(storedCond).NotTo(BeNil())
			Expect(storedCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("AgentTool validation flow", func() {
		var (
			ctx           context.Context
			agentToolName string
			toolNN        types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			agentToolName = fmt.Sprintf("integ-tool-%d", time.Now().UnixNano())
			toolNN = types.NamespacedName{Name: agentToolName, Namespace: namespace}
		})

		AfterEach(func() {
			agentTool := &agentv1alpha1.AgentTool{}
			if err := k8sClient.Get(ctx, toolNN, agentTool); err == nil {
				_ = k8sClient.Delete(ctx, agentTool)
			}
		})

		It("should validate an MCP AgentTool and surface the condition", func() {
			By("Creating an AgentTool resource")
			port := int32(8080)
			agentTool := &agentv1alpha1.AgentTool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentToolName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.AgentToolSpec{
					Type:        agentv1alpha1.AgentToolTypeMCP,
					Description: "Integration test MCP endpoint",
					ServiceRef:  &agentv1alpha1.ServiceRef{Name: "tool-svc", Port: &port},
				},
			}
			Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

			By("Reconciling the AgentTool")
			r := &AgentToolReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: toolNN})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Validated condition is True")
			Expect(k8sClient.Get(ctx, toolNN, agentTool)).To(Succeed())
			validatedCond := meta.FindStatusCondition(agentTool.Status.Conditions, ConditionTypeValidated)
			Expect(validatedCond).NotTo(BeNil())
			Expect(validatedCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("ModelProvider → Model flow", func() {
		var (
			ctx          context.Context
			providerName string
			modelName    string
			providerNN   types.NamespacedName
			modelNN      types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			providerName = fmt.Sprintf("integ-prov-%d", time.Now().UnixNano())
			modelName = fmt.Sprintf("integ-model-%d", time.Now().UnixNano())
			providerNN = types.NamespacedName{Name: providerName, Namespace: namespace}
			modelNN = types.NamespacedName{Name: modelName, Namespace: namespace}
		})

		AfterEach(func() {
			model := &agentv1alpha1.Model{}
			if err := k8sClient.Get(ctx, modelNN, model); err == nil {
				_ = k8sClient.Delete(ctx, model)
			}
			provider := &agentv1alpha1.ModelProvider{}
			if err := k8sClient.Get(ctx, providerNN, provider); err == nil {
				_ = k8sClient.Delete(ctx, provider)
			}
		})

		It("should resolve Model to Ready when ModelProvider is Ready", func() {
			By("Creating a ModelProvider with OpenAI config")
			provider := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			By("Reconciling the ModelProvider")
			providerReconciler := &ModelProviderReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := providerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: providerNN})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying ModelProvider is Ready")
			Expect(k8sClient.Get(ctx, providerNN, provider)).To(Succeed())
			Expect(provider.Status.Ready).To(BeTrue())
			Expect(provider.Status.Provider).To(Equal(agentv1alpha1.ProviderTypeOpenAI))

			By("Creating a Model referencing the provider")
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.ModelSpec{
					Model: "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{
						Name: providerName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			By("Reconciling the Model")
			modelReconciler := &ModelReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err = modelReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNN})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Model is Ready with resolved provider")
			Expect(k8sClient.Get(ctx, modelNN, model)).To(Succeed())
			Expect(model.Status.Ready).To(BeTrue())
			Expect(model.Status.ResolvedProvider).NotTo(BeNil())
			Expect(model.Status.ResolvedProvider.Provider).To(Equal(agentv1alpha1.ProviderTypeOpenAI))
			Expect(model.Status.ResolvedProvider.Name).To(Equal(providerName))
			Expect(model.Status.ObservedGeneration).To(Equal(model.Generation))

			readyCond := meta.FindStatusCondition(model.Status.Conditions, ModelConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))

			resolvedCond := meta.FindStatusCondition(model.Status.Conditions, ModelConditionTypeProviderResolved)
			Expect(resolvedCond).NotTo(BeNil())
			Expect(resolvedCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Full Agent deployment flow", func() {
		var (
			ctx          context.Context
			agentName    string
			providerName string
			modelName    string
			agentNN      types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			suffix := time.Now().UnixNano()
			agentName = fmt.Sprintf("integ-agent-%d", suffix)
			providerName = fmt.Sprintf("integ-prov-%d", suffix)
			modelName = fmt.Sprintf("integ-model-%d", suffix)
			agentNN = types.NamespacedName{Name: agentName, Namespace: namespace}
		})

		AfterEach(func() {
			cleanupAgent(ctx, agentNN)
			model := &agentv1alpha1.Model{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: modelName, Namespace: namespace}, model); err == nil {
				_ = k8sClient.Delete(ctx, model)
			}
			provider := &agentv1alpha1.ModelProvider{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: providerName, Namespace: namespace}, provider); err == nil {
				_ = k8sClient.Delete(ctx, provider)
			}
		})

		It("should create Deployment and Service when all dependencies are ready", func() {
			By("Setting up ModelProvider → Model chain")

			// Create ModelProvider
			provider := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			providerReconciler := &ModelProviderReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := providerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: providerName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Create Model
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			modelReconciler := &ModelReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err = modelReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: modelName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Creating an Agent referencing the Model")
			agent := &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.AgentSpec{
					Card:     minimalCard(),
					ModelRef: &agentv1alpha1.NamespacedRef{Name: modelName},
				},
			}
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

			By("Reconciling the Agent")
			r := newAgentReconciler()
			reconcileAgent(ctx, r, agentNN)

			By("Verifying Deployment was created")
			deployment := getDeployment(ctx, agentNN)
			Expect(deployment).NotTo(BeNil())
			Expect(deployment.Spec.Template.Spec.Containers).NotTo(BeEmpty())

			By("Verifying Service was created")
			service := getService(ctx, agentNN)
			Expect(service).NotTo(BeNil())

			By("Verifying the compiled spec and conditions")
			agent = getAgent(ctx, agentNN)
			Expect(agent.Status.SpecHash).NotTo(BeEmpty())
			specValid := meta.FindStatusCondition(agent.Status.Conditions, agentdomain.ConditionTypeSpecValid)
			Expect(specValid).NotTo(BeNil())
			Expect(specValid.Status).To(Equal(metav1.ConditionTrue))
		})
	})
})
