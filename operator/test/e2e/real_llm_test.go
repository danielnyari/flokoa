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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The single spec that exercises a live model provider end to end:
// ModelProvider resolution -> API-key secret -> a real gpt-5-mini call ->
// A2A response. It is labeled "real-llm" so the merge-path e2e run
// (-ginkgo.label-filter '!real-llm') skips it; it runs in the nightly
// workflow with OPENAI_API_KEY set. Everything else runs deterministically on
// the built-in `test` model on every PR (see agent.yaml / capability-agent.yaml).
//
// The assertion is deliberately content-agnostic — a real model's wording is
// non-deterministic, so we assert the task reached `completed` with no JSON-RPC
// error, which proves the provider resolved, the key worked, the model
// answered, and the A2A round trip closed. Asserting specific text would be
// flaky, not stronger.
var _ = Describe("Real-LLM smoke", Label("real-llm"), Ordered, func() {
	realLLMManifests := []string{
		"test/e2e/testdata/modelprovider.yaml",
		"test/e2e/testdata/model.yaml",
		"test/e2e/testdata/instruction.yaml",
		"test/e2e/testdata/agent-real-llm.yaml",
	}

	BeforeAll(func() {
		skipIfNoOpenAIKey()
		Expect(ensureOpenAIAPIKeySecret(namespace)).To(Succeed(), "failed to create the openai-api-key secret")
		for _, m := range realLLMManifests {
			Expect(applyManifestFile(m)).To(Succeed(), "failed to apply %s", m)
		}
	})

	AfterAll(func() {
		for i := len(realLLMManifests) - 1; i >= 0; i-- {
			deleteManifestFile(realLLMManifests[i])
		}
	})

	It("answers a prompt with a live provider", func() {
		By("waiting for the real-llm agent to reach Ready")
		Expect(waitForAgentReady("real-llm-agent", namespace, 3*time.Minute)).To(Succeed(),
			"real-llm-agent did not become Ready; pod diagnostics:\n%s", describeAgentPods("real-llm-agent"))

		By("invoking the agent over A2A and asserting the task completes")
		httpClient, agentURL, err := agentA2AProxy("real-llm-agent")
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			body, err := sendA2AMessage(httpClient, agentURL, "Reply with the single word: pong")
			g.Expect(err).NotTo(HaveOccurred())
			_, _ = fmt.Fprintf(GinkgoWriter, "A2A response: %s\n", body)

			var rpc struct {
				Error  map[string]any `json:"error"`
				Result struct {
					Status struct {
						State string `json:"state"`
					} `json:"status"`
				} `json:"result"`
			}
			g.Expect(json.Unmarshal([]byte(body), &rpc)).To(Succeed())
			g.Expect(rpc.Error).To(BeNil(), "A2A error: %v", rpc.Error)
			g.Expect(rpc.Result.Status.State).To(Equal("completed"),
				"the live model call should drive the A2A task to completed")
		}, 3*time.Minute, 10*time.Second).Should(Succeed())
	})
})
