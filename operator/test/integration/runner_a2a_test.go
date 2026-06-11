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
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	agentdomain "github.com/danielnyari/flokoa/internal/domain/agent"
	"github.com/danielnyari/flokoa/internal/infra/builder"
)

// The data-plane leg of the suite: the spec the real operator compiled is
// hydrated and served by the real Python runner (as a subprocess instead of
// a container), and answered over real A2A HTTP. The agent uses pydantic-ai's
// built-in `test` model so no provider credentials or network are needed.
//
// Requires the SDK workspace venv (sdk/python/.venv). Without it the spec
// skips unless FLOKOA_INTEGRATION_STRICT=1 (the CI gate) makes that a
// failure.
var _ = Describe("Compiled spec served by the real runner over A2A", func() {
	It("answers message/send with the operator-compiled spec", func() {
		python := runnerPython()
		if python == "" {
			if os.Getenv("FLOKOA_INTEGRATION_STRICT") == "1" {
				Fail("sdk/python/.venv not found — run 'uv sync --all-packages' in sdk/python (strict mode)")
			}
			Skip("sdk/python/.venv not found — run 'uv sync --all-packages' in sdk/python to enable the runner leg")
		}

		By("creating a self-contained agent on the built-in test model")
		agent := &agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{Name: "golden-agent", Namespace: testNamespace},
			Spec: agentv1alpha1.AgentSpec{
				Card: agentv1alpha1.AgentCardOverride{
					Name:        "golden-agent",
					Description: "integration data-plane agent",
					Version:     "0.1.0",
					Skills: []agentv1alpha1.AgentSkill{{
						ID: "echo", Name: "Echo", Description: "Answers", Tags: []string{"test"},
					}},
				},
				Spec: &agentv1alpha1.AgentSpecFragment{
					Model:        "test",
					Instructions: []string{"You are the integration test agent."},
				},
			},
		}
		Expect(k8sClient.Create(ctx, agent)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, agent) })

		By("waiting for the operator to compile it")
		cm := &corev1.ConfigMap{}
		Eventually(func(g Gomega) {
			fetched := &agentv1alpha1.Agent{}
			g.Expect(k8sClient.Get(ctx, nn("golden-agent"), fetched)).To(Succeed())
			condition := meta.FindStatusCondition(fetched.Status.Conditions, agentdomain.ConditionTypeSpecValid)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue), condition.Message)
			g.Expect(k8sClient.Get(ctx, nn(builder.SpecConfigMapName("golden-agent")), cm)).To(Succeed())
		}, waitFor, tick).Should(Succeed())

		By("materializing the contract files exactly as the kubelet would")
		dir := GinkgoT().TempDir()
		specPath := filepath.Join(dir, "agent-spec.yaml")
		cardPath := filepath.Join(dir, "agent-card.json")
		Expect(os.WriteFile(specPath, []byte(cm.Data[builder.AgentSpecConfigMapKey]), 0o600)).To(Succeed())
		Expect(os.WriteFile(cardPath, []byte(cm.Data[builder.AgentCardConfigMapKey]), 0o600)).To(Succeed())

		port := freePort()
		publicURL := fmt.Sprintf("http://127.0.0.1:%d/", port)

		By("starting the real runner with the contract env (incl. skew checks)")
		cmd := exec.Command(python, "-m", "flokoa_runner")
		cmd.Dir = filepath.Join(repoRoot(), "sdk", "python")
		cmd.Env = append(os.Environ(),
			"FLOKOA_HOST=127.0.0.1",
			fmt.Sprintf("FLOKOA_PORT=%d", port),
			"FLOKOA_PUBLIC_URL="+publicURL,
			"FLOKOA_AGENT_SPEC_PATH="+specPath,
			"FLOKOA_AGENT_CARD_PATH="+cardPath,
			"FLOKOA_RUNNER_MANIFEST_PATH="+filepath.Join(repoRoot(), "sdk", "python", "flokoa-runner", "runner-manifest.json"),
			// The operator's annotations drive the runner's skew detection —
			// this asserts the operator↔manifest digest chain for real.
			"FLOKOA_EXPECTED_RUNNER_VERSION="+cm.Annotations["flokoa.ai/runner-version"],
			"FLOKOA_EXPECTED_SCHEMA_DIGEST="+cm.Annotations["flokoa.ai/schema-digest"],
		)
		cmd.Stdout = GinkgoWriter
		cmd.Stderr = GinkgoWriter
		Expect(cmd.Start()).To(Succeed())
		DeferCleanup(func() {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		})

		By("waiting for the A2A server to become healthy")
		httpClient := &http.Client{Timeout: 2 * time.Second}
		Eventually(func() error {
			resp, err := httpClient.Get(publicURL + "health")
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health: %d", resp.StatusCode)
			}
			return nil
		}, 60*time.Second, 500*time.Millisecond).Should(Succeed(), "runner never became healthy")

		By("fetching the agent card the operator rendered")
		resp, err := httpClient.Get(publicURL + ".well-known/agent-card.json")
		Expect(err).NotTo(HaveOccurred())
		var card map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&card)).To(Succeed())
		Expect(resp.Body.Close()).To(Succeed())
		Expect(card["name"]).To(Equal("golden-agent"))

		By("sending a real A2A message/send")
		request := map[string]any{
			"jsonrpc": "2.0",
			"id":      "integration-1",
			"method":  "message/send",
			"params": map[string]any{
				"message": map[string]any{
					"role":      "user",
					"kind":      "message",
					"messageId": "msg-integration-1",
					"parts":     []map[string]any{{"kind": "text", "text": "hello from the integration suite"}},
				},
			},
		}
		body, err := json.Marshal(request)
		Expect(err).NotTo(HaveOccurred())
		postResp, err := (&http.Client{Timeout: 30 * time.Second}).Post(publicURL, "application/json", bytes.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = postResp.Body.Close() }()

		var rpc struct {
			Error  map[string]any `json:"error"`
			Result struct {
				Status struct {
					State string `json:"state"`
				} `json:"status"`
				Artifacts []map[string]any `json:"artifacts"`
			} `json:"result"`
		}
		Expect(json.NewDecoder(postResp.Body).Decode(&rpc)).To(Succeed())
		Expect(rpc.Error).To(BeNil())
		Expect(rpc.Result.Status.State).To(Equal("completed"))
		Expect(rpc.Result.Artifacts).NotTo(BeEmpty())
	})
})

// runnerPython locates the SDK workspace interpreter with flokoa-runner
// installed; empty when the venv hasn't been synced.
func runnerPython() string {
	python := filepath.Join(repoRoot(), "sdk", "python", ".venv", "bin", "python")
	if _, err := os.Stat(python); err != nil {
		return ""
	}
	return python
}

func freePort() int {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	port := listener.Addr().(*net.TCPAddr).Port
	Expect(listener.Close()).To(Succeed())
	return port
}

func rawJSON(s string) *apiextensionsv1.JSON {
	return &apiextensionsv1.JSON{Raw: []byte(s)}
}
