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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/internal/infra/builder"
)

// Virtual endpoint identity (roadmap 06): the published endpoint is a
// flokoa-owned identity. v1 backs it with the runner pods directly; the P1
// session router insertion is a selector swap behind the same Service.
var _ = Describe("Virtual endpoint identity", func() {
	ctx := context.Background()

	var counter int
	newAgent := func() (*agentv1alpha1.Agent, types.NamespacedName) {
		counter++
		name := fmt.Sprintf("vep-agent-%d", counter)
		agent := &agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: agentv1alpha1.AgentSpec{
				Card: minimalCard(),
				Spec: &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"},
			},
		}
		return agent, types.NamespacedName{Name: name, Namespace: "default"}
	}

	It("creates both the published and the runtime Service, operator-owned", func() {
		agent, nn := newAgent()
		Expect(k8sClient.Create(ctx, agent)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, agent) })

		reconcileAgent(ctx, newAgentReconciler(), nn)

		published := getService(ctx, nn)
		Expect(published.Labels["flokoa.ai/endpoint"]).To(Equal("published"))
		Expect(published.OwnerReferences).NotTo(BeEmpty())

		runtime := getService(ctx, types.NamespacedName{
			Name: builder.RuntimeServiceName(nn.Name), Namespace: nn.Namespace,
		})
		Expect(runtime.Labels["flokoa.ai/endpoint"]).To(Equal("runtime"))
		Expect(runtime.OwnerReferences).NotTo(BeEmpty())

		// v1: same backend — published selects the runner pods directly.
		Expect(published.Spec.Selector).To(Equal(runtime.Spec.Selector))
	})

	It("publishes the normative status.url backed by the published Service", func() {
		agent, nn := newAgent()
		Expect(k8sClient.Create(ctx, agent)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, agent) })

		reconcileAgent(ctx, newAgentReconciler(), nn)

		fetched := getAgent(ctx, nn)
		Expect(fetched.Status.URL).To(Equal(
			fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/", nn.Name, nn.Namespace, builder.PublishedPort)))
	})

	It("keeps the published identity stable when the backend selector is swapped", func() {
		// Simulates the P1 session-router insertion: flipping the published
		// Service's selector must not change anything a caller can observe
		// (the URL — i.e. name and namespace — stays identical).
		agent, nn := newAgent()
		Expect(k8sClient.Create(ctx, agent)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, agent) })

		reconcileAgent(ctx, newAgentReconciler(), nn)
		urlBefore := getAgent(ctx, nn).Status.URL

		published := getService(ctx, nn)
		published.Spec.Selector = map[string]string{"app.kubernetes.io/name": "flokoa-router"}
		Expect(k8sClient.Update(ctx, published)).To(Succeed())

		Expect(getAgent(ctx, nn).Status.URL).To(Equal(urlBefore))
		swapped := getService(ctx, nn)
		Expect(swapped.Spec.Selector["app.kubernetes.io/name"]).To(Equal("flokoa-router"))
	})
})
