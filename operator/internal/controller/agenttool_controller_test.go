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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

var _ = Describe("AgentTool Controller", func() {
	ctx := context.Background()

	newReconciler := func() *AgentToolReconciler {
		return &AgentToolReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	}

	reconcileTool := func(name types.NamespacedName) {
		_, err := newReconciler().Reconcile(ctx, ctrl.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
	}

	makeTool := func(name string, mutate func(*agentv1alpha1.AgentToolSpec)) *agentv1alpha1.AgentTool {
		port := int32(8080)
		tool := &agentv1alpha1.AgentTool{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: agentv1alpha1.AgentToolSpec{
				Type:        agentv1alpha1.AgentToolTypeMCP,
				Description: "test MCP endpoint",
				ServiceRef:  &agentv1alpha1.ServiceRef{Name: "tool-svc", Port: &port},
			},
		}
		if mutate != nil {
			mutate(&tool.Spec)
		}
		return tool
	}

	getCondition := func(name types.NamespacedName) *metav1.Condition {
		tool := &agentv1alpha1.AgentTool{}
		ExpectWithOffset(1, k8sClient.Get(ctx, name, tool)).To(Succeed())
		return meta.FindStatusCondition(tool.Status.Conditions, ConditionTypeValidated)
	}

	var counter int
	newName := func() types.NamespacedName {
		counter++
		return types.NamespacedName{Name: fmt.Sprintf("tool-%d", counter), Namespace: "default"}
	}

	It("marks a valid MCP tool Validated=True", func() {
		name := newName()
		tool := makeTool(name.Name, nil)
		Expect(k8sClient.Create(ctx, tool)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, tool) })

		reconcileTool(name)

		condition := getCondition(name)
		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		Expect(condition.Reason).To(Equal(ReasonValidationSuccess))
	})

	It("marks a tool with url+serviceRef Validated=False", func() {
		name := newName()
		tool := makeTool(name.Name, func(spec *agentv1alpha1.AgentToolSpec) {
			spec.URL = "http://both.example.com/mcp"
		})
		Expect(k8sClient.Create(ctx, tool)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, tool) })

		reconcileTool(name)

		condition := getCondition(name)
		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.Message).To(ContainSubstring("mutually exclusive"))
	})

	It("records header secrets and prefixes without error", func() {
		name := newName()
		tool := makeTool(name.Name, func(spec *agentv1alpha1.AgentToolSpec) {
			spec.ToolPrefix = "kb"
			spec.AllowedTools = []string{"search"}
			spec.HeaderSecrets = []agentv1alpha1.SecretHeader{{
				Name: "Authorization",
				SecretRef: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "kb-token"},
					Key:                  "token",
				},
			}}
		})
		Expect(k8sClient.Create(ctx, tool)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, tool) })

		reconcileTool(name)

		condition := getCondition(name)
		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionTrue))
	})

	It("updates ObservedGeneration on each reconcile", func() {
		name := newName()
		tool := makeTool(name.Name, nil)
		Expect(k8sClient.Create(ctx, tool)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, tool) })

		reconcileTool(name)

		fetched := &agentv1alpha1.AgentTool{}
		Expect(k8sClient.Get(ctx, name, fetched)).To(Succeed())
		Expect(fetched.Status.ObservedGeneration).To(Equal(fetched.Generation))
	})
})
