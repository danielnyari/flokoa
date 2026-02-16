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
	"fmt"
	"os/exec"
	"time"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/test/utils"
)

var _ = Describe("AgentWorkflow with A2A Plugin", Ordered, func() {
	SetDefaultEventuallyTimeout(5 * time.Minute)
	SetDefaultEventuallyPollingInterval(2 * time.Second)

	Context("A2A Plugin Integration", func() {
		BeforeAll(func() {
			By("building the A2A plugin image")
			cmd := exec.Command("docker", "build",
				"-f", "plugins/a2a/Dockerfile",
				"-t", a2aPluginImage,
				".")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to build A2A plugin image")

			By("loading the A2A plugin image into Kind")
			err = utils.LoadImageToKindClusterWithName(a2aPluginImage)
			Expect(err).NotTo(HaveOccurred(), "Failed to load A2A plugin image")

			By("installing Argo Workflows")
			err = utils.InstallArgoWorkflows(ctx, k8sClient, namespace)
			Expect(err).NotTo(HaveOccurred(), "Failed to install Argo Workflows")

			By("installing the A2A executor plugin")
			err = utils.InstallA2AExecutorPlugin(ctx, k8sClient, a2aPluginImage)
			Expect(err).NotTo(HaveOccurred(), "Failed to install A2A executor plugin")

			By("applying RBAC for Argo Workflows")
			err = applyManifestFile("test/e2e/testdata/argo/rbac.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to apply Argo RBAC")

			By("creating the plugin service account token secret")
			err = applyManifestFile("test/e2e/testdata/secret.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to create plugin token secret")

			By("deploying the tool service")
			err = applyManifestFile("test/e2e/testdata/tool-service.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to deploy tool service")

			By("waiting for tool service to be ready")
			err = waitForDeploymentReady("tool-service", namespace, 5*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "Tool service deployment not ready")
		})

		It("should deploy agent and execute AgentWorkflow with A2A plugin", func() {
			var err error

			By("creating/updating the OpenAI API key secret from OPENAI_API_KEY")
			err = ensureOpenAIAPIKeySecret(namespace)
			Expect(err).NotTo(HaveOccurred(), "Failed to create OpenAI API key secret")

			By("applying the ModelProvider")
			err = applyManifestFile("test/e2e/testdata/modelprovider.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to apply ModelProvider")

			By("applying the Model")
			err = applyManifestFile("test/e2e/testdata/model.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to apply Model")

			By("applying the Instruction with template")
			err = applyManifestFile("test/e2e/testdata/instruction.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to apply Instruction")

			By("applying the AgentTool")
			err = applyManifestFile("test/e2e/testdata/agenttool.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to apply AgentTool")

			By("applying the Agent with templated instruction")
			err = applyManifestFile("test/e2e/testdata/agent.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to apply Agent")

			By("waiting for Agent to reach Ready condition")
			err = waitForAgentReady("petstore-agent", namespace, 3*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "Agent not ready")

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
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "petstore-agent", Namespace: namespace}, svc)
			Expect(err).NotTo(HaveOccurred(), "Agent service should exist")

			By("creating the AgentWorkflow CR")
			err = applyManifestFile("test/e2e/testdata/argo/agentworkflow.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to create AgentWorkflow")
			_, _ = fmt.Fprintf(GinkgoWriter, "Created AgentWorkflow: e2e-petstore-workflow\n")

			By("waiting for AgentWorkflow to complete")
			err = waitForAgentWorkflowPhase("e2e-petstore-workflow", namespace, agentv1alpha1.WorkflowPhaseSucceeded, 5*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "AgentWorkflow did not succeed")

			By("verifying AgentWorkflow status")
			awf := &agentv1alpha1.AgentWorkflow{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "e2e-petstore-workflow", Namespace: namespace}, awf)
			Expect(err).NotTo(HaveOccurred(), "Failed to get AgentWorkflow")
			_, _ = fmt.Fprintf(GinkgoWriter, "AgentWorkflow phase: %s\n", awf.Status.Phase)

			Expect(awf.Status.ArgoWorkflowName).NotTo(BeEmpty(), "Should have created an Argo Workflow")
			Expect(awf.Status.StartTime).NotTo(BeNil(), "Should have a start time")
			Expect(awf.Status.CompletionTime).NotTo(BeNil(), "Should have a completion time")
			_, _ = fmt.Fprintf(GinkgoWriter, "Argo Workflow name: %s\n", awf.Status.ArgoWorkflowName)

			By("verifying AgentWorkflow conditions")
			compiled := findCondition(awf.Status.Conditions, "Compiled")
			Expect(compiled).NotTo(BeNil(), "Should have Compiled condition")
			Expect(compiled.Status).To(Equal(metav1.ConditionTrue))

			submitted := findCondition(awf.Status.Conditions, "Submitted")
			Expect(submitted).NotTo(BeNil(), "Should have Submitted condition")
			Expect(submitted.Status).To(Equal(metav1.ConditionTrue))

			ready := findCondition(awf.Status.Conditions, "Ready")
			Expect(ready).NotTo(BeNil(), "Should have Ready condition")
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))

			By("verifying Argo Workflow was created with expected properties")
			argoWf, err := getWorkflow(awf.Status.ArgoWorkflowName, namespace)
			Expect(err).NotTo(HaveOccurred(), "Failed to get Argo Workflow")
			Expect(argoWf.Labels).To(HaveKeyWithValue("agent.flokoa.ai/agentworkflow-name", "e2e-petstore-workflow"))
			Expect(argoWf.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "flokoa-operator"))
			Expect(argoWf.Status.Phase).To(Equal(wfv1.WorkflowSucceeded))

			By("verifying workflow outputs")
			var taskResponse string
			for _, node := range argoWf.Status.Nodes {
				if node.Outputs != nil && len(node.Outputs.Parameters) > 0 {
					for _, param := range node.Outputs.Parameters {
						if param.Name == "taskResponse" && param.Value != nil {
							taskResponse = param.Value.String()
							_, _ = fmt.Fprintf(GinkgoWriter, "Workflow taskResponse: %s\n", taskResponse)
						}
					}
				}
			}
			Expect(taskResponse).NotTo(BeEmpty(), "Should have taskResponse output from agent")
			Expect(taskResponse).To(ContainSubstring(`"state":"completed"`), "Workflow response should indicate completed state")

			By("verifying AgentWorkflow task statuses")
			Expect(awf.Status.TaskStatuses).NotTo(BeEmpty(), "Should have task statuses")
			var foundTaskStatus bool
			for _, ts := range awf.Status.TaskStatuses {
				_, _ = fmt.Fprintf(GinkgoWriter, "Task status: name=%s phase=%s\n", ts.Name, ts.Phase)
				if ts.Name == "list-pets" {
					foundTaskStatus = true
					Expect(ts.Phase).To(Equal(agentv1alpha1.WorkflowPhaseSucceeded))
					Expect(ts.StartTime).NotTo(BeNil())
					Expect(ts.CompletionTime).NotTo(BeNil())
				}
			}
			Expect(foundTaskStatus).To(BeTrue(), "Should have task status for 'list-pets'")
		})

		AfterAll(func() {
			By("cleaning up AgentWorkflow")
			deleteManifestFile("test/e2e/testdata/argo/agentworkflow.yaml")

			By("cleaning up agent test resources")
			deleteManifestFile("test/e2e/testdata/agent.yaml")
			deleteManifestFile("test/e2e/testdata/agenttool.yaml")
			deleteManifestFile("test/e2e/testdata/instruction.yaml")
			deleteManifestFile("test/e2e/testdata/model.yaml")
			deleteManifestFile("test/e2e/testdata/modelprovider.yaml")
			deleteManifestFile("test/e2e/testdata/secret.yaml")
			deleteManifestFile("test/e2e/testdata/tool-service.yaml")

			By("cleaning up Argo RBAC")
			deleteManifestFile("test/e2e/testdata/argo/rbac.yaml")

			By("uninstalling A2A executor plugin")
			utils.UninstallA2AExecutorPlugin(ctx, k8sClient)

			By("uninstalling Argo Workflows")
			utils.UninstallArgoWorkflows(ctx, k8sClient)
		})
	})

	Context("AgentWorkflow Failure Handling", func() {
		BeforeAll(func() {
			if !utils.IsArgoWorkflowsInstalled(ctx, k8sClient) {
				Skip("Argo Workflows not installed, skipping failure handling tests")
			}
		})

		It("should handle missing agent gracefully", func() {
			By("creating an AgentWorkflow targeting a non-existent agent")
			err := applyManifestFile("test/e2e/testdata/argo/agentworkflow-fail.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to create AgentWorkflow")

			By("waiting for AgentWorkflow to fail")
			err = waitForAgentWorkflowPhase("e2e-fail-workflow", namespace, agentv1alpha1.WorkflowPhaseFailed, 2*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "AgentWorkflow should have failed")

			By("verifying AgentWorkflow failure status")
			awf := &agentv1alpha1.AgentWorkflow{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "e2e-fail-workflow", Namespace: namespace}, awf)
			Expect(err).NotTo(HaveOccurred())
			Expect(awf.Status.Phase).To(Equal(agentv1alpha1.WorkflowPhaseFailed))
			Expect(awf.Status.ArgoWorkflowName).NotTo(BeEmpty(), "Should have attempted to create Argo Workflow")

			By("cleaning up failed AgentWorkflow")
			deleteManifestFile("test/e2e/testdata/argo/agentworkflow-fail.yaml")
		})
	})
})
