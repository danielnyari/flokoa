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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/danielnyari/flokoa/test/utils"
)

// namespace where the project is deployed in
const namespace = "flokoa-system"

// serviceAccountName created for the project
const serviceAccountName = "flokoa-controller"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "flokoa-controller-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "flokoa-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager and server")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage), fmt.Sprintf("SERVER_IMG=%s", serverImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager and server")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		// E2E test suite for the Flokoa operator
		// Tests include:
		// 1. Controller pod deployment and health
		// 2. Metrics endpoint availability
		// 3. ModelProvider CR lifecycle (creation, reconciliation, deletion)
		// 4. Model CR lifecycle (creation, reconciliation, Ready condition, deletion)
		// 5. AgentTool CR lifecycle (creation, reconciliation, status updates, deletion)
		// 6. Agent CR lifecycle (creation, Deployment/Service creation, status updates, deletion)
		// 7. Agent finalizer handling and cleanup

		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "app.kubernetes.io/name=flokoa,app.kubernetes.io/component=controller",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("flokoa-controller"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("waiting for the metrics endpoint to be ready")
			verifyMetricsEndpointReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "endpoints", metricsServiceName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("8443"), "Metrics endpoint is not ready")
			}
			Eventually(verifyMetricsEndpointReady).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("controller-runtime.metrics\tServing metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted).Should(Succeed())

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccount": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			metricsOutput := getMetricsOutput()
			Expect(metricsOutput).To(ContainSubstring(
				"controller_runtime_reconcile_total",
			))
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		It("should successfully create and reconcile a ModelProvider resource", func() {
			By("creating a Secret for API credentials")
			secretYAML := `
apiVersion: v1
kind: Secret
metadata:
  name: test-openai-credentials
  namespace: ` + namespace + `
type: Opaque
stringData:
  api-key: "test-key-12345"
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(secretYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Secret")

			By("creating a ModelProvider resource")
			providerYAML := `
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: test-openai-provider
  namespace: ` + namespace + `
spec:
  apiKeySecretRef:
    name: test-openai-credentials
    key: api-key
  openai:
    timeoutSeconds: 120
`
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(providerYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ModelProvider")

			By("verifying the ModelProvider was created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "modelprovider", "test-openai-provider", "-n", namespace)
				_, err := utils.Run(cmd)
				return err
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying reconciliation in metrics")
			Eventually(func() string {
				return getMetricsOutput()
			}, 2*time.Minute, 5*time.Second).Should(ContainSubstring("controller_runtime_reconcile_total"))

			By("cleaning up test resources")
			cmd = exec.Command("kubectl", "delete", "modelprovider", "test-openai-provider", "-n", namespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "secret", "test-openai-credentials", "-n", namespace)
			_, _ = utils.Run(cmd)
		})

		It("should successfully create and reconcile a Model resource", func() {
			By("creating a Secret for API credentials")
			secretYAML := `
apiVersion: v1
kind: Secret
metadata:
  name: test-model-credentials
  namespace: ` + namespace + `
type: Opaque
stringData:
  api-key: "test-key-67890"
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(secretYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Secret")

			By("creating a ModelProvider for the Model")
			providerYAML := `
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: test-model-provider
  namespace: ` + namespace + `
spec:
  apiKeySecretRef:
    name: test-model-credentials
    key: api-key
  openai: {}
`
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(providerYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ModelProvider")

			By("creating a Model resource")
			modelYAML := `
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: test-gpt4-model
  namespace: ` + namespace + `
spec:
  model: gpt-4
  providerRef:
    name: test-model-provider
  parameters:
    temperature: "0.7"
    maxTokens: 2048
`
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(modelYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Model")

			By("verifying the Model was created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "model", "test-gpt4-model", "-n", namespace)
				_, err := utils.Run(cmd)
				return err
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying Model has Ready condition")
			Eventually(func() string {
				cmd := exec.Command("kubectl", "get", "model", "test-gpt4-model", "-n", namespace, "-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				if err != nil {
					return ""
				}
				return output
			}, 2*time.Minute, 5*time.Second).Should(Equal("True"))

			By("cleaning up test resources")
			cmd = exec.Command("kubectl", "delete", "model", "test-gpt4-model", "-n", namespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "modelprovider", "test-model-provider", "-n", namespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "secret", "test-model-credentials", "-n", namespace)
			_, _ = utils.Run(cmd)
		})

		It("should successfully create and reconcile an AgentTool resource", func() {
			By("creating an AgentTool resource")
			toolYAML := `
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: test-search-tool
  namespace: ` + namespace + `
spec:
  type: http-api
  description: "Search API for testing"
  httpApi:
    url: "https://api.example.com/search"
    method: GET
    timeoutSeconds: 30
  inputSchema:
    type: object
    properties:
      query:
        type: string
        description: "Search query"
    required:
      - query
  outputSchema:
    type: object
    properties:
      results:
        type: array
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(toolYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create AgentTool")

			By("verifying the AgentTool was created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "agenttool", "test-search-tool", "-n", namespace)
				_, err := utils.Run(cmd)
				return err
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying AgentTool status is updated")
			Eventually(func() string {
				cmd := exec.Command("kubectl", "get", "agenttool", "test-search-tool", "-n", namespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				if err != nil {
					return ""
				}
				return output
			}, 2*time.Minute, 5*time.Second).Should(Equal("Ready"))

			By("cleaning up test resources")
			cmd = exec.Command("kubectl", "delete", "agenttool", "test-search-tool", "-n", namespace)
			_, _ = utils.Run(cmd)
		})

		It("should successfully create and reconcile an Agent resource", func() {
			By("creating an Agent resource")
			agentYAML := `
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: test-agent
  namespace: ` + namespace + `
spec:
  framework: pydantic-ai
  runtime:
    type: standard
    spec:
      replicas: 1
      container:
        name: agent
        image: nginx:1.25
        ports:
        - containerPort: 8080
          name: http
          protocol: TCP
        resources:
          requests:
            cpu: "100m"
            memory: "128Mi"
          limits:
            cpu: "200m"
            memory: "256Mi"
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(agentYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Agent")

			By("verifying the Agent was created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "agent", "test-agent", "-n", namespace)
				_, err := utils.Run(cmd)
				return err
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying Agent Deployment was created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "deployment", "test-agent", "-n", namespace)
				_, err := utils.Run(cmd)
				return err
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying Agent Service was created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "service", "test-agent", "-n", namespace)
				_, err := utils.Run(cmd)
				return err
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying Agent status phase is set")
			Eventually(func() string {
				cmd := exec.Command("kubectl", "get", "agent", "test-agent", "-n", namespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				if err != nil {
					return ""
				}
				return output
			}, 2*time.Minute, 5*time.Second).Should(Or(Equal("Pending"), Equal("Running")))

			By("cleaning up test resources")
			cmd = exec.Command("kubectl", "delete", "agent", "test-agent", "-n", namespace)
			_, _ = utils.Run(cmd)
		})

		It("should handle Agent deletion with finalizers properly", func() {
			By("creating an Agent resource")
			agentYAML := `
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: test-agent-finalizer
  namespace: ` + namespace + `
spec:
  framework: langchain
  runtime:
    type: standard
    spec:
      replicas: 1
      container:
        name: agent
        image: nginx:1.25
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(agentYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Agent")

			By("verifying the Agent has a finalizer")
			Eventually(func() string {
				cmd := exec.Command("kubectl", "get", "agent", "test-agent-finalizer", "-n", namespace, "-o", "jsonpath={.metadata.finalizers}")
				output, err := utils.Run(cmd)
				if err != nil {
					return ""
				}
				return output
			}, 2*time.Minute, 5*time.Second).Should(ContainSubstring("agent.flokoa.ai/finalizer"))

			By("deleting the Agent")
			cmd = exec.Command("kubectl", "delete", "agent", "test-agent-finalizer", "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete Agent")

			By("verifying the Agent is eventually deleted")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "agent", "test-agent-finalizer", "-n", namespace)
				_, err := utils.Run(cmd)
				return err
			}, 2*time.Minute, 5*time.Second).ShouldNot(Succeed())
		})
	})
})

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() string {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	metricsOutput, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
	Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
	return metricsOutput
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
