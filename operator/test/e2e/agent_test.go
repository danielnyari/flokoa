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

package e2e

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// testManifests lists all the manifest files used in agent tests
var testManifests = []string{
	"test/e2e/testdata/tool-service.yaml",
	"test/e2e/testdata/modelprovider.yaml",
	"test/e2e/testdata/model.yaml",
	"test/e2e/testdata/instruction.yaml",
	"test/e2e/testdata/agenttool.yaml",
	"test/e2e/testdata/agent.yaml",
}

var _ = Describe("Agent", Ordered, func() {
	SetDefaultEventuallyTimeout(3 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Compiled Agent E2E Test", func() {
		BeforeAll(func() {
			By("ensuring OPENAI_API_KEY is available for real e2e run")
			skipIfNoOpenAIKey()
			err := ensureOpenAIAPIKeySecret(namespace)
			Expect(err).NotTo(HaveOccurred(), "Failed to configure openai-api-key secret")
		})

		It("should compile the composition into an agent-spec ConfigMap and run it on the generic runner", func() {
			By("deploying the tool service")
			err := applyManifestFile("test/e2e/testdata/tool-service.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to deploy tool service")

			By("waiting for tool service to be ready")
			err = waitForDeploymentReady("tool-service", namespace, 5*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "Tool service deployment not ready")

			By("creating/updating the OpenAI API key secret")
			err = ensureOpenAIAPIKeySecret(namespace)
			Expect(err).NotTo(HaveOccurred(), "Failed to configure openai-api-key secret")

			By("applying the ModelProvider")
			err = applyManifestFile("test/e2e/testdata/modelprovider.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to apply ModelProvider")

			By("applying the Model")
			err = applyManifestFile("test/e2e/testdata/model.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to apply Model")

			By("applying the Instruction")
			err = applyManifestFile("test/e2e/testdata/instruction.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to apply Instruction")

			By("verifying Instruction ConfigMap is created")
			Eventually(func(g Gomega) {
				instruction := &agentv1alpha1.Instruction{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "petstore-instruction",
					Namespace: namespace,
				}, instruction)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(instruction.Status.ConfigMapName).NotTo(BeEmpty(), "ConfigMap name should be set")
			}, 1*time.Minute).Should(Succeed())

			By("applying the AgentTool")
			err = applyManifestFile("test/e2e/testdata/agenttool.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to apply AgentTool")

			By("applying the Agent")
			err = applyManifestFile("test/e2e/testdata/agent.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to apply Agent")

			By("waiting for Agent to reach Ready condition")
			Eventually(func(g Gomega) {
				agent := &agentv1alpha1.Agent{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "petstore-agent",
					Namespace: namespace,
				}, agent)
				g.Expect(err).NotTo(HaveOccurred())

				// Check for Ready condition
				ready := false
				for _, cond := range agent.Status.Conditions {
					if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
						ready = true
						break
					}
				}
				g.Expect(ready).To(BeTrue(), "Agent should be Ready")
			}, 3*time.Minute).Should(Succeed())

			By("verifying Agent pod is running")
			Eventually(func(g Gomega) {
				podList := &corev1.PodList{}
				err := k8sClient.List(ctx, podList,
					client.InNamespace(namespace),
					client.MatchingLabels{"app.kubernetes.io/name": "petstore-agent"})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(podList.Items).To(HaveLen(1), "should have one agent pod")
				g.Expect(podList.Items[0].Status.Phase).To(Equal(corev1.PodRunning))
			}, 2*time.Minute).Should(Succeed())

			By("verifying Agent service is created")
			svc := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "petstore-agent",
				Namespace: namespace,
			}, svc)
			Expect(err).NotTo(HaveOccurred(), "Agent service should exist")
		})

		AfterAll(func() {
			By("cleaning up test resources")
			// Delete in reverse order of creation
			for i := len(testManifests) - 1; i >= 0; i-- {
				deleteManifestFile(testManifests[i])
			}
		})
	})
})
