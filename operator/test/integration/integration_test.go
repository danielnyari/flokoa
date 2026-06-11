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

package integration

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiyaml "sigs.k8s.io/yaml"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	agentdomain "github.com/danielnyari/flokoa/internal/domain/agent"
	"github.com/danielnyari/flokoa/internal/infra/builder"
)

const (
	waitFor = 30 * time.Second
	tick    = 250 * time.Millisecond
)

// loadFixture decodes a single-document YAML fixture from the Kind e2e
// testdata (the suites share fixtures by design) into obj, forcing the
// integration namespace.
func loadFixture(name string, obj interface {
	SetNamespace(string)
}) {
	GinkgoHelper()
	raw, err := os.ReadFile(fixturePath(name))
	Expect(err).NotTo(HaveOccurred())
	Expect(apiyaml.Unmarshal(raw, obj)).To(Succeed())
	obj.SetNamespace(testNamespace)
}

var _ = Describe("Operator integration (manager-driven)", Ordered, func() {
	BeforeAll(func() {
		By("creating the provider API key secret")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "openai-api-key", Namespace: testNamespace},
			StringData: map[string]string{"api-key": "sk-test"},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		By("applying the shared e2e fixtures")
		provider := &agentv1alpha1.ModelProvider{}
		loadFixture("modelprovider.yaml", provider)
		Expect(k8sClient.Create(ctx, provider)).To(Succeed())

		model := &agentv1alpha1.Model{}
		loadFixture("model.yaml", model)
		Expect(k8sClient.Create(ctx, model)).To(Succeed())

		instruction := &agentv1alpha1.Instruction{}
		loadFixture("instruction.yaml", instruction)
		Expect(k8sClient.Create(ctx, instruction)).To(Succeed())

		tool := &agentv1alpha1.AgentTool{}
		loadFixture("agenttool.yaml", tool)
		Expect(k8sClient.Create(ctx, tool)).To(Succeed())

		agent := &agentv1alpha1.Agent{}
		loadFixture("agent.yaml", agent)
		Expect(k8sClient.Create(ctx, agent)).To(Succeed())
	})

	getAgent := func() *agentv1alpha1.Agent {
		agent := &agentv1alpha1.Agent{}
		ExpectWithOffset(1, k8sClient.Get(ctx, nn("petstore-agent"), agent)).To(Succeed())
		return agent
	}

	It("compiles the full composition end-to-end via real watches", func() {
		By("waiting for SpecValid=True")
		Eventually(func(g Gomega) {
			agent := getAgent()
			condition := meta.FindStatusCondition(agent.Status.Conditions, agentdomain.ConditionTypeSpecValid)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue), condition.Message)
		}, waitFor, tick).Should(Succeed())

		agent := getAgent()
		Expect(agent.Status.SpecHash).NotTo(BeEmpty())
		Expect(agent.Status.RunnerVersion).NotTo(BeEmpty())
		Expect(agent.Status.InjectedCapabilities).To(ContainElement("flokoa.platform/telemetry"))
		Expect(agent.Status.URL).To(Equal("http://petstore-agent.integration.svc.cluster.local:80/"))

		By("verifying the compiled-spec ConfigMap carries the whole composition")
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, nn(builder.SpecConfigMapName("petstore-agent")), cm)).To(Succeed())
		compiled := cm.Data[builder.AgentSpecConfigMapKey]
		Expect(compiled).To(ContainSubstring("model: openai:gpt-5-mini"), "model ref resolved + provider-prefixed")
		Expect(compiled).To(ContainSubstring("Petstore API assistant"), "instruction ref content inlined")
		Expect(compiled).To(
			ContainSubstring("http://tool-service.integration.svc.cluster.local:8080/mcp"),
			"AgentTool compiled to MCP url")
		Expect(compiled).To(ContainSubstring("flokoa.platform/telemetry"), "platform capability injected")
		Expect(cm.Data).To(HaveKey(builder.AgentCardConfigMapKey))

		By("verifying the Deployment runs the pinned runner with contract env")
		deployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, nn("petstore-agent"), deployment)).To(Succeed())
		container := deployment.Spec.Template.Spec.Containers[0]
		Expect(container.Image).To(HavePrefix(builder.DefaultRunnerImageRepository + ":"))
		Expect(envValue(container.Env, "FLOKOA_PUBLIC_URL")).To(Equal(agent.Status.URL))
		Expect(envValue(container.Env, "FLOKOA_EXPECTED_RUNNER_VERSION")).To(Equal(agent.Status.RunnerVersion))
		Expect(envValue(container.Env, "ENVIRONMENT")).To(Equal("petstore-e2e"), "user env preserved")
		Expect(deployment.Spec.Template.Annotations["flokoa.ai/spec-hash"]).To(Equal(agent.Status.SpecHash))

		By("verifying the virtual endpoint pair")
		published := &corev1.Service{}
		Expect(k8sClient.Get(ctx, nn("petstore-agent"), published)).To(Succeed())
		runtime := &corev1.Service{}
		Expect(k8sClient.Get(ctx, nn(builder.RuntimeServiceName("petstore-agent")), runtime)).To(Succeed())
		Expect(published.Spec.Selector).To(Equal(runtime.Spec.Selector))
	})

	It("recompiles the fleet when a referenced Instruction changes (watch-driven)", func() {
		before := getAgent().Status.SpecHash
		Expect(before).NotTo(BeEmpty())

		instruction := &agentv1alpha1.Instruction{}
		Expect(k8sClient.Get(ctx, nn("petstore-instruction"), instruction)).To(Succeed())
		instruction.Spec.Content += "\n5. Always answer in English."
		Expect(k8sClient.Update(ctx, instruction)).To(Succeed())

		// No manual reconcile: the Instruction watch mapper must re-enqueue
		// the agent, recompile, and roll the spec hash on its own.
		Eventually(func() string {
			return getAgent().Status.SpecHash
		}, waitFor, tick).ShouldNot(Equal(before))

		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, nn(builder.SpecConfigMapName("petstore-agent")), cm)).To(Succeed())
		Expect(cm.Data[builder.AgentSpecConfigMapKey]).To(ContainSubstring("Always answer in English."))
	})

	It("rolls the secrets hash when the provider API key rotates (watch-driven)", func() {
		deployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, nn("petstore-agent"), deployment)).To(Succeed())
		before := deployment.Spec.Template.Annotations["flokoa.ai/secrets-hash"]
		Expect(before).NotTo(BeEmpty())

		secret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, nn("openai-api-key"), secret)).To(Succeed())
		secret.StringData = map[string]string{"api-key": "sk-rotated"}
		Expect(k8sClient.Update(ctx, secret)).To(Succeed())

		Eventually(func() string {
			d := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, nn("petstore-agent"), d); err != nil {
				return before
			}
			return d.Spec.Template.Annotations["flokoa.ai/secrets-hash"]
		}, waitFor, tick).ShouldNot(Equal(before))
	})

	It("keeps the last good spec running when the composition turns invalid", func() {
		goodHash := getAgent().Status.SpecHash

		By("breaking the fragment with a field the AgentSpec schema forbids")
		agent := getAgent()
		agent.Spec.Spec.Extra = rawJSON(`{"system_prompt": "not an AgentSpec field"}`)
		Expect(k8sClient.Update(ctx, agent)).To(Succeed())

		Eventually(func(g Gomega) {
			condition := meta.FindStatusCondition(getAgent().Status.Conditions, agentdomain.ConditionTypeSpecValid)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(condition.Reason).To(Equal(agentdomain.ReasonSpecInvalid))
		}, waitFor, tick).Should(Succeed())

		By("verifying the Deployment still runs the last good spec")
		deployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, nn("petstore-agent"), deployment)).To(Succeed())
		Expect(deployment.Spec.Template.Annotations["flokoa.ai/spec-hash"]).To(Equal(goodHash))

		By("repairing the composition")
		agent = getAgent()
		agent.Spec.Spec.Extra = nil
		Expect(k8sClient.Update(ctx, agent)).To(Succeed())
		Eventually(func(g Gomega) {
			condition := meta.FindStatusCondition(getAgent().Status.Conditions, agentdomain.ConditionTypeSpecValid)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		}, waitFor, tick).Should(Succeed())
	})

	It("cleans up owned resources when the Agent is deleted", func() {
		agent := getAgent()
		Expect(k8sClient.Delete(ctx, agent)).To(Succeed())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, nn("petstore-agent"), &agentv1alpha1.Agent{})
			return apierrors.IsNotFound(err)
		}, waitFor, tick).Should(BeTrue())
		// envtest has no garbage collector; ownership is asserted via
		// ownerReferences on the children instead.
		cm := &corev1.ConfigMap{}
		if err := k8sClient.Get(ctx, nn(builder.SpecConfigMapName("petstore-agent")), cm); err == nil {
			Expect(cm.OwnerReferences).NotTo(BeEmpty())
		}
	})
})

func envValue(env []corev1.EnvVar, name string) string {
	for _, e := range env {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}
