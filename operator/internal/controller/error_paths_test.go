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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	agentdomain "github.com/danielnyari/flokoa/internal/domain/agent"
)

// Negative/error path tests verify graceful degradation.
var _ = Describe("Error Path Tests", func() {
	const (
		namespace = "default"
		timeout   = time.Second * 10
		interval  = time.Millisecond * 250
	)

	Context("Model controller error paths", func() {
		var (
			ctx       context.Context
			modelName string
			modelNN   types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			modelName = fmt.Sprintf("err-model-%d", time.Now().UnixNano())
			modelNN = types.NamespacedName{Name: modelName, Namespace: namespace}
		})

		AfterEach(func() {
			model := &agentv1alpha1.Model{}
			if err := k8sClient.Get(ctx, modelNN, model); err == nil {
				_ = k8sClient.Delete(ctx, model)
			}
		})

		It("should set NotReady when provider does not exist", func() {
			By("Creating a Model with non-existent provider")
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.ModelSpec{
					Model: "gpt-5-mini",
					ProviderRef: agentv1alpha1.ProviderRef{
						Name: "non-existent-provider",
					},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			By("Reconciling the Model")
			r := &ModelReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: modelNN})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(modelRetryInterval))

			By("Verifying status shows provider not found")
			Expect(k8sClient.Get(ctx, modelNN, model)).To(Succeed())
			Expect(model.Status.Ready).To(BeFalse())
			Expect(model.Status.ObservedGeneration).To(Equal(model.Generation))

			readyCond := meta.FindStatusCondition(model.Status.Conditions, ModelConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(ModelReasonProviderNotFound))
		})

		It("should set NotReady when provider is not ready", func() {
			By("Creating an unreconciled ModelProvider (not ready)")
			providerName := fmt.Sprintf("err-prov-%d", time.Now().UnixNano())
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
			defer func() {
				_ = k8sClient.Delete(ctx, provider)
			}()
			// NOTE: Do not reconcile the provider — its status.Ready defaults to false

			By("Creating a Model referencing the unready provider")
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-5-mini",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			By("Reconciling the Model")
			r := &ModelReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: modelNN})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(modelRetryInterval))

			By("Verifying status shows provider not ready")
			Expect(k8sClient.Get(ctx, modelNN, model)).To(Succeed())
			Expect(model.Status.Ready).To(BeFalse())

			readyCond := meta.FindStatusCondition(model.Status.Conditions, ModelConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Reason).To(Equal(ModelReasonProviderNotReady))
		})

		It("should transition from NotReady to Ready when provider appears", func() {
			By("Creating a Model referencing a not-yet-existing provider")
			providerName := fmt.Sprintf("err-prov-%d", time.Now().UnixNano())
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-5-mini",
					ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			By("Reconciling — expect NotReady")
			modelReconciler := &ModelReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := modelReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNN})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(modelRetryInterval))

			Expect(k8sClient.Get(ctx, modelNN, model)).To(Succeed())
			Expect(model.Status.Ready).To(BeFalse())

			By("Now creating and reconciling the ModelProvider")
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
			defer func() {
				_ = k8sClient.Delete(ctx, provider)
			}()

			providerReconciler := &ModelProviderReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err = providerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: providerName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Re-reconciling the Model — expect Ready")
			_, err = modelReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: modelNN})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, modelNN, model)).To(Succeed())
			Expect(model.Status.Ready).To(BeTrue())

			readyCond := meta.FindStatusCondition(model.Status.Conditions, ModelConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("ModelProvider controller error paths", func() {
		var (
			ctx          context.Context
			providerName string
			providerNN   types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			providerName = fmt.Sprintf("err-prov-%d", time.Now().UnixNano())
			providerNN = types.NamespacedName{Name: providerName, Namespace: namespace}
		})

		AfterEach(func() {
			provider := &agentv1alpha1.ModelProvider{}
			if err := k8sClient.Get(ctx, providerNN, provider); err == nil {
				_ = k8sClient.Delete(ctx, provider)
			}
		})

		It("should set NotReady when no provider block is set", func() {
			By("Creating a ModelProvider with empty spec")
			provider := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.ModelProviderSpec{},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			By("Reconciling the ModelProvider")
			r := &ModelProviderReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: providerNN})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying status shows not ready")
			Expect(k8sClient.Get(ctx, providerNN, provider)).To(Succeed())
			Expect(provider.Status.Ready).To(BeFalse())
			Expect(provider.Status.Provider).To(BeEmpty())

			readyCond := meta.FindStatusCondition(provider.Status.Conditions, ModelProviderConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should set NotReady when multiple providers are set", func() {
			By("Creating a ModelProvider with both OpenAI and Anthropic")
			provider := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI:    &agentv1alpha1.OpenAIProviderSpec{},
					Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			By("Reconciling the ModelProvider")
			r := &ModelProviderReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: providerNN})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying status shows not ready")
			Expect(k8sClient.Get(ctx, providerNN, provider)).To(Succeed())
			Expect(provider.Status.Ready).To(BeFalse())

			validatedCond := meta.FindStatusCondition(provider.Status.Conditions, ModelProviderConditionTypeValidated)
			Expect(validatedCond).NotTo(BeNil())
			Expect(validatedCond.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("Instruction controller error paths", func() {
		var (
			ctx             context.Context
			instructionName string
			instNN          types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			instructionName = fmt.Sprintf("err-inst-%d", time.Now().UnixNano())
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

		It("should handle reconcile of non-existent resource gracefully", func() {
			By("Reconciling a non-existent Instruction")
			r := &InstructionReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: instNN})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle deletion during reconciliation", func() {
			By("Creating an Instruction")
			instruction := &agentv1alpha1.Instruction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instructionName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.InstructionSpec{
					Content: "Temporary instruction",
				},
			}
			Expect(k8sClient.Create(ctx, instruction)).To(Succeed())

			By("Reconciling to add finalizer and create ConfigMap")
			r := &InstructionReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Add finalizer
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: instNN})
			Expect(err).NotTo(HaveOccurred())

			// Create ConfigMap
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: instNN})
			Expect(err).NotTo(HaveOccurred())

			By("Deleting the Instruction")
			Expect(k8sClient.Delete(ctx, instruction)).To(Succeed())

			By("Reconciling the deletion — should remove finalizer and ConfigMap")
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: instNN})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the ConfigMap was deleted")
			cmName := fmt.Sprintf("%s-instruction", instructionName)
			cm := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: namespace}, cm)
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Agent controller error paths", func() {
		var (
			ctx       context.Context
			agentName string
			agentNN   types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			agentName = fmt.Sprintf("err-agent-%d", time.Now().UnixNano())
			agentNN = types.NamespacedName{Name: agentName, Namespace: namespace}
		})

		AfterEach(func() {
			cleanupAgent(ctx, agentNN)
		})

		expectNoDeployment := func() {
			GinkgoHelper()
			err := k8sClient.Get(ctx, agentNN, &appsv1.Deployment{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "no Deployment must exist for %s", agentNN)
		}

		It("should handle reconcile of deleted Agent gracefully", func() {
			By("Reconciling a non-existent Agent")
			r := newAgentReconciler()
			_, err := reconcileOnce(ctx, r, agentNN)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should set SpecValid=False/DependencyMissing and requeue when the referenced Model is missing", func() {
			By("Creating an Agent referencing a non-existent Model")
			agent := &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.AgentSpec{
					ModelRef: &agentv1alpha1.NamespacedRef{Name: "does-not-exist"},
					Card:     minimalCard(),
				},
			}
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

			By("Reconciling the Agent (finalizer + compile attempt)")
			r := newAgentReconciler()

			// First reconcile adds finalizer
			_, err := reconcileOnce(ctx, r, agentNN)
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: dependency error → fixed-interval requeue
			result, err := reconcileOnce(ctx, r, agentNN)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))

			By("Verifying SpecValid condition reports the missing dependency")
			agent = getAgent(ctx, agentNN)
			cond := meta.FindStatusCondition(agent.Status.Conditions, agentdomain.ConditionTypeSpecValid)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(agentdomain.ReasonDependencyMissing))
			Expect(cond.Message).To(ContainSubstring("does-not-exist"))

			By("Verifying no Deployment was created")
			expectNoDeployment()
		})

		It("should set SpecValid=False/SpecInvalid without requeue for a schema-invalid fragment", func() {
			By("Creating an Agent whose extra layer carries an unknown top-level field")
			agent := &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.AgentSpec{
					Spec: &agentv1alpha1.AgentSpecFragment{
						Model: "openai:gpt-5-mini",
						Extra: &apiextensionsv1.JSON{Raw: []byte(`{"bogus_field": true}`)},
					},
					Card: minimalCard(),
				},
			}
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

			r := newAgentReconciler()
			_, err := reconcileOnce(ctx, r, agentNN) // finalizer
			Expect(err).NotTo(HaveOccurred())

			result, err := reconcileOnce(ctx, r, agentNN)
			Expect(err).NotTo(HaveOccurred(), "schema-invalid specs are not retried")
			Expect(result.RequeueAfter).To(BeZero(), "no requeue: only an edit can fix the composition")

			By("Verifying SpecValid=False with reason SpecInvalid")
			agent = getAgent(ctx, agentNN)
			cond := meta.FindStatusCondition(agent.Status.Conditions, agentdomain.ConditionTypeSpecValid)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(agentdomain.ReasonSpecInvalid))
			Expect(agent.Status.SpecHash).To(BeEmpty(), "no spec ever compiled")

			By("Verifying no Deployment was created")
			expectNoDeployment()
		})

		It("should keep the last good Deployment when the composition turns invalid", func() {
			By("Creating a valid Agent and reconciling it")
			Expect(k8sClient.Create(ctx, minimalAgent(agentNN))).To(Succeed())

			r := newAgentReconciler()
			reconcileAgent(ctx, r, agentNN)

			agent := getAgent(ctx, agentNN)
			goodHash := agent.Status.SpecHash
			Expect(goodHash).NotTo(BeEmpty())

			By("Breaking the composition with an unknown top-level field")
			agent.Spec.Spec.Extra = &apiextensionsv1.JSON{Raw: []byte(`{"bogus_field": true}`)}
			Expect(k8sClient.Update(ctx, agent)).To(Succeed())

			result, err := reconcileOnce(ctx, r, agentNN)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			By("Verifying SpecValid=False but the last good spec keeps running")
			agent = getAgent(ctx, agentNN)
			cond := meta.FindStatusCondition(agent.Status.Conditions, agentdomain.ConditionTypeSpecValid)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(agentdomain.ReasonSpecInvalid))
			Expect(agent.Status.SpecHash).To(Equal(goodHash), "status keeps the last good hash")

			deployment := getDeployment(ctx, agentNN)
			Expect(deployment.Spec.Template.Annotations).To(
				HaveKeyWithValue("flokoa.ai/spec-hash", goodHash),
				"the Deployment must stay on the last good spec",
			)
		})

		It("should fail validation for the session isolation tier (not available yet)", func() {
			// The admission webhook also rejects this, but it is not running
			// in this suite — domain validation is the backstop under test.
			agent := minimalAgent(agentNN)
			agent.Spec.Runtime.Isolation = agentv1alpha1.IsolationSession
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

			r := newAgentReconciler()
			_, err := reconcileOnce(ctx, r, agentNN) // finalizer
			Expect(err).NotTo(HaveOccurred())

			result, err := reconcileOnce(ctx, r, agentNN)
			Expect(err).NotTo(HaveOccurred(), "validation failures are permanent, not retried")
			Expect(result.RequeueAfter).To(BeZero())

			agent = getAgent(ctx, agentNN)
			Expect(agent.Status.Phase).To(Equal(agentv1alpha1.AgentPhaseFailed))

			cond := meta.FindStatusCondition(agent.Status.Conditions, agentdomain.ConditionTypeSpecValid)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(agentdomain.ReasonValidationFailed))
			Expect(cond.Message).To(ContainSubstring("isolation"))

			expectNoDeployment()
		})

		It("should fail validation when no model source is set", func() {
			agent := &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.AgentSpec{
					// Neither spec.spec.model nor spec.modelRef.
					Spec: &agentv1alpha1.AgentSpecFragment{Description: "no model anywhere"},
					Card: minimalCard(),
				},
			}
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

			r := newAgentReconciler()
			_, _ = reconcileOnce(ctx, r, agentNN)
			_, err := reconcileOnce(ctx, r, agentNN)
			Expect(err).NotTo(HaveOccurred())

			agent = getAgent(ctx, agentNN)
			Expect(agent.Status.Phase).To(Equal(agentv1alpha1.AgentPhaseFailed))

			cond := meta.FindStatusCondition(agent.Status.Conditions, agentdomain.ConditionTypeSpecValid)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Reason).To(Equal(agentdomain.ReasonValidationFailed))
			Expect(cond.Message).To(ContainSubstring("model is required"))

			expectNoDeployment()
		})

		It("should report missing secrets via SecretsReady=False but still deploy", func() {
			agent := minimalAgent(agentNN)
			agent.Spec.SecretRefs = map[string]corev1.SecretKeySelector{
				"github-token": {
					LocalObjectReference: corev1.LocalObjectReference{Name: "no-such-secret"},
					Key:                  "token",
				},
			}
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

			r := newAgentReconciler()
			reconcileAgent(ctx, r, agentNN)

			agent = getAgent(ctx, agentNN)
			cond := meta.FindStatusCondition(agent.Status.Conditions, agentdomain.ConditionTypeSecretsReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(agentdomain.ReasonSecretsMissing))
			Expect(cond.Message).To(ContainSubstring("no-such-secret"))

			By("The Deployment is still created (the pod will crash-loop until the secret appears)")
			deployment := getDeployment(ctx, agentNN)
			env := envByName(firstContainer(deployment))
			Expect(env).To(HaveKey("FLOKOA_SECRET_GITHUB_TOKEN"))
			Expect(env["FLOKOA_SECRET_GITHUB_TOKEN"].ValueFrom.SecretKeyRef.Name).To(Equal("no-such-secret"))
		})
	})

	Context("AgentWorkflow controller error paths", func() {
		var (
			ctx          context.Context
			workflowName string
			workflowNN   types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			workflowName = fmt.Sprintf("err-wf-%d", time.Now().UnixNano())
			workflowNN = types.NamespacedName{Name: workflowName, Namespace: namespace}
		})

		AfterEach(func() {
			wf := &agentv1alpha1.AgentWorkflow{}
			if err := k8sClient.Get(ctx, workflowNN, wf); err == nil {
				_ = k8sClient.Delete(ctx, wf)
			}
		})

		It("should handle reconcile of non-existent workflow gracefully", func() {
			By("Reconciling a non-existent AgentWorkflow")
			r := &AgentWorkflowReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: workflowNN})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
