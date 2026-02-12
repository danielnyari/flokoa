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
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/danielnyari/flokoa/test/utils"
)

var _ = Describe("Argo Workflows with A2A Plugin", Ordered, func() {
	SetDefaultEventuallyTimeout(5 * time.Minute)
	SetDefaultEventuallyPollingInterval(2 * time.Second)

	var workflowName string

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
			err = utils.InstallArgoWorkflows(ctx, k8sClient)
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

			By("applying the A2A workflow template")
			err = applyManifestFile("test/e2e/testdata/argo/workflow-template.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to apply workflow template")
		})

		It("should deploy agent and execute workflow with A2A plugin", func() {
			By("deploying the tool service")
			err := applyManifestFile("test/e2e/testdata/tool-service.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to deploy tool service")

			By("waiting for tool service to be ready")
			err = waitForDeploymentReady("tool-service", namespace, 2*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "Tool service deployment not ready")

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

			By("creating the Argo workflow from workflow template")
			wf := &wfv1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "a2a-e2e-",
					Namespace:    namespace,
				},
				Spec: wfv1.WorkflowSpec{
					WorkflowTemplateRef: &wfv1.WorkflowTemplateRef{Name: "a2a-agent-test"},
				},
			}
			err = k8sClient.Create(ctx, wf)
			Expect(err).NotTo(HaveOccurred(), "Failed to create workflow from template")
			workflowName = wf.Name
			Expect(workflowName).NotTo(BeEmpty(), "Workflow name should not be empty")
			_, _ = fmt.Fprintf(GinkgoWriter, "Created workflow: %s\n", workflowName)

			By("waiting for workflow to complete")
			err = waitForWorkflowPhase(workflowName, namespace, wfv1.WorkflowSucceeded, 5*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "Workflow did not succeed")

			By("verifying workflow outputs")
			workflowResult, err := getWorkflow(workflowName, namespace)
			Expect(err).NotTo(HaveOccurred(), "Failed to get workflow")
			_, _ = fmt.Fprintf(GinkgoWriter, "Workflow phase: %s\n", workflowResult.Status.Phase)

			var taskResponse string
			for _, node := range workflowResult.Status.Nodes {
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
		})

		AfterAll(func() {
			By("cleaning up workflow")
			if workflowName != "" {
				deleteWorkflow(workflowName, namespace)
			}

			By("cleaning up agent test resources")
			deleteManifestFile("test/e2e/testdata/agent.yaml")
			deleteManifestFile("test/e2e/testdata/agenttool.yaml")
			deleteManifestFile("test/e2e/testdata/instruction.yaml")
			deleteManifestFile("test/e2e/testdata/model.yaml")
			deleteManifestFile("test/e2e/testdata/modelprovider.yaml")
			deleteManifestFile("test/e2e/testdata/secret.yaml")
			deleteManifestFile("test/e2e/testdata/tool-service.yaml")
			deleteManifestFile("test/e2e/testdata/argo/workflow-template.yaml")

			By("cleaning up Argo RBAC")
			deleteManifestFile("test/e2e/testdata/argo/rbac.yaml")

			By("uninstalling A2A executor plugin")
			utils.UninstallA2AExecutorPlugin(ctx, k8sClient)

			By("uninstalling Argo Workflows")
			utils.UninstallArgoWorkflows(ctx, k8sClient)
		})
	})

	Context("Workflow Failure Handling", func() {
		BeforeAll(func() {
			// Skip if Argo is not installed
			if !utils.IsArgoWorkflowsInstalled(ctx, k8sClient) {
				Skip("Argo Workflows not installed, skipping failure handling tests")
			}
		})

		It("should handle missing agent gracefully", func() {
			By("creating a workflow targeting a non-existent agent")
			pluginPayload := fmt.Sprintf(`{"a2a":{"agent":"nonexistent-agent","namespace":"%s","message":"This should fail","timeout":"30s"}}`, namespace)
			wf := &wfv1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "a2a-fail-test-",
					Namespace:    namespace,
				},
				Spec: wfv1.WorkflowSpec{
					Entrypoint:         "call-nonexistent",
					ServiceAccountName: "argo-workflow",
					Templates: []wfv1.Template{
						{
							Name: "call-nonexistent",
							Plugin: &wfv1.Plugin{
								Object: wfv1.Object{
									Value: json.RawMessage(pluginPayload),
								},
							},
						},
					},
				},
			}

			err := k8sClient.Create(ctx, wf)
			Expect(err).NotTo(HaveOccurred(), "Failed to create workflow")

			failWorkflowName := wf.Name
			Expect(failWorkflowName).NotTo(BeEmpty())

			By("waiting for workflow to fail")
			err = waitForWorkflowPhase(failWorkflowName, namespace, wfv1.WorkflowFailed, 2*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "Workflow should have failed")

			By("cleaning up failed workflow")
			deleteWorkflow(failWorkflowName, namespace)
		})
	})
})
