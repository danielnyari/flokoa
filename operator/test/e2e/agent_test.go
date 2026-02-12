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
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/test/utils"
)

// testManifests lists all the manifest files used in agent tests
var testManifests = []string{
	"test/e2e/testdata/llm-stub.yaml",
	"test/e2e/testdata/tool-service.yaml",
	"test/e2e/testdata/secret.yaml",
	"test/e2e/testdata/modelprovider.yaml",
	"test/e2e/testdata/model.yaml",
	"test/e2e/testdata/instruction.yaml",
	"test/e2e/testdata/agenttool.yaml",
	"test/e2e/testdata/agent.yaml",
}

var _ = Describe("Agent", Ordered, func() {
	SetDefaultEventuallyTimeout(3 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Template Agent E2E Test", func() {
		BeforeAll(func() {
			By("building the LLM stub image")
			cmd := exec.Command("docker", "build",
				"-f", "test/e2e/fixtures/Dockerfile.llm-stub",
				"-t", "localhost/llm-stub:test",
				"test/e2e/fixtures")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to build LLM stub image")

			By("loading the LLM stub image into Kind")
			err = utils.LoadImageToKindClusterWithName("localhost/llm-stub:test")
			Expect(err).NotTo(HaveOccurred(), "Failed to load LLM stub image")

			By("building the tool service image")
			cmd = exec.Command("docker", "build",
				"-f", "test/e2e/fixtures/Dockerfile.tool-service",
				"-t", "localhost/tool-service:test",
				"test/e2e/fixtures")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to build tool service image")

			By("loading the tool service image into Kind")
			err = utils.LoadImageToKindClusterWithName("localhost/tool-service:test")
			Expect(err).NotTo(HaveOccurred(), "Failed to load tool service image")

			By("building the template agent image")
			cmd = exec.Command("docker", "build",
				"-f", "test/e2e/fixtures/Dockerfile",
				"-t", "localhost/template-agent:test",
				"..")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to build template agent image")

			By("loading the template agent image into Kind")
			err = utils.LoadImageToKindClusterWithName("localhost/template-agent:test")
			Expect(err).NotTo(HaveOccurred(), "Failed to load template agent image")
		})

		It("should deploy CRs and create a templated agent from Python SDK", func() {
			By("deploying the LLM stub service")
			err := applyManifestFile("test/e2e/testdata/llm-stub.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to deploy LLM stub")

			By("waiting for LLM stub to be ready")
			Eventually(func(g Gomega) {
				pod := &corev1.Pod{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "llm-stub",
					Namespace: namespace,
				}, pod)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(pod.Status.Phase).To(Equal(corev1.PodRunning))
			}, 2*time.Minute).Should(Succeed())

			By("deploying the tool service")
			err = applyManifestFile("test/e2e/testdata/tool-service.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to deploy tool service")

			By("waiting for tool service to be ready")
			Eventually(func(g Gomega) {
				pod := &corev1.Pod{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "tool-service",
					Namespace: namespace,
				}, pod)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(pod.Status.Phase).To(Equal(corev1.PodRunning))
			}, 2*time.Minute).Should(Succeed())

			By("creating the API key secret for ModelProvider")
			err = applyManifestFile("test/e2e/testdata/secret.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to create secret")

			By("applying the ModelProvider")
			err = applyManifestFile("test/e2e/testdata/modelprovider.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to apply ModelProvider")

			By("applying the Model")
			err = applyManifestFile("test/e2e/testdata/model.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to apply Model")

			By("applying the Instruction with template")
			err = applyManifestFile("test/e2e/testdata/instruction.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to apply Instruction")

			By("verifying Instruction ConfigMap is created")
			Eventually(func(g Gomega) {
				instruction := &agentv1alpha1.Instruction{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "template-instruction",
					Namespace: namespace,
				}, instruction)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(instruction.Status.ConfigMapName).NotTo(BeEmpty(), "ConfigMap name should be set")
			}, 1*time.Minute).Should(Succeed())

			By("applying the AgentTool")
			err = applyManifestFile("test/e2e/testdata/agenttool.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to apply AgentTool")

			By("applying the Agent with templated instruction")
			err = applyManifestFile("test/e2e/testdata/agent.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to apply Agent")

			By("waiting for Agent to reach Ready condition")
			Eventually(func(g Gomega) {
				agent := &agentv1alpha1.Agent{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "template-agent",
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
					client.MatchingLabels{"app.kubernetes.io/name": "template-agent"})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(podList.Items).To(HaveLen(1), "should have one agent pod")
				g.Expect(podList.Items[0].Status.Phase).To(Equal(corev1.PodRunning))
			}, 2*time.Minute).Should(Succeed())

			By("verifying Agent service is created")
			svc := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "template-agent",
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
