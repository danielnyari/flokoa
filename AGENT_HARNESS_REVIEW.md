# Can Flokoa Be Advertised as an Agent Harness?

**Date:** 2026-06-10
**Benchmark:** [Amazon Bedrock AgentCore harness (Preview)](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/harness.html)
**Scope:** Gap analysis between Flokoa v0.1.0 (operator) / v0.0.5 (SDK) and the capability set the industry now associates with the term "agent harness."

---

## 1. What "agent harness" means (the bar AgentCore set)

AWS defines the harness as *the system that lets an agent actually run*: the orchestration loop (call the model, pick a tool, pass results back, manage context, handle failures) **plus** the infrastructure underneath it — compute, sandboxed code execution, secure tool connections, persistent storage, memory, identity, and observability. The developer **declares** what the agent does (model, tools, instructions); the harness turns that config into a running agent.

The AgentCore harness feature set, distilled from the docs and launch coverage:

| # | Capability | What AgentCore provides |
|---|------------|------------------------|
| H1 | **Declarative config** | `agentcore.json` declares model, instructions, tools, memory, environment — no orchestration code |
| H2 | **Managed runtime** | Isolated microVM per session with its own filesystem and shell |
| H3 | **Stateful sessions** | Stateful by default; session ID scoping; idle-timeout / max-lifetime lifecycle controls |
| H4 | **Memory** | Attachable Memory instance; conversation auto-saved per session; short- and long-term memory survives session expiry |
| H5 | **Sandboxed execution & built-in tools** | Agent can write/execute code; built-in Code Interpreter and Browser |
| H6 | **Tool connectivity** | MCP servers, AgentCore Gateway (centralized auth, policy, per-tool-call logging), OpenAPI via Gateway |
| H7 | **Model agnosticism** | Bedrock, OpenAI, Google Gemini; switch providers mid-session without losing context |
| H8 | **Identity** | Inbound OAuth (JWT) on the agent endpoint; end-user identity threaded to downstream tools (scoped credentials, not shared service accounts); IAM on harness + runtime resources |
| H9 | **Observability & cost controls** | Every invocation emits traces/logs/metrics; model calls, tool invocations, memory ops, shell commands visible with timing; tags for cost allocation |
| H10 | **Developer experience** | CLI: scaffold → local dev → deploy → invoke → `logs` / `traces`; pre-built skills for coding assistants |

Sources: [harness overview](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/harness.html), [get started](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/harness-get-started.html), [environment & skills](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/harness-environment.html), [memory](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/harness-memory.html), [tools](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/harness-tools.html), [security](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/harness-security.html), [observability & cost](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/harness-operations.html), [AWS launch blog](https://aws.amazon.com/blogs/machine-learning/get-to-your-first-working-agent-in-minutes-announcing-new-features-in-amazon-bedrock-agentcore/), [DevelopersIO preview write-up](https://dev.classmethod.jp/en/articles/bedrock-agentcore-managed-harness-preview/), [clawaws harness explainer](https://clawaws.com/blog/agentcore-harness-explained/).

---

## 2. Where Flokoa stands today, capability by capability

| # | Capability | Flokoa status | Evidence |
|---|------------|:---:|----------|
| H1 | Declarative config | ✅ **Strong** | 6 CRDs (`Agent`, `Model`, `ModelProvider`, `Instruction`, `AgentTool`, `AgentWorkflow`) under `agent.flokoa.ai/v1alpha1` — this *is* the harness config model, expressed as Kubernetes resources. Admission webhooks validate cross-references and DAG cycles (`operator/api/v1alpha1/*_webhook.go`) |
| H2 | Managed runtime | 🟡 **Partial** | `template` runtime: operator deploys a managed pydantic-ai runtime from ConfigMap config (`sdk/python/flokoa-managed-agent/`); `standard` runtime for BYO images. But: shared long-lived pods, **no per-session isolation**, no filesystem/shell environment, no hot-reload |
| H3 | Stateful sessions | ❌ **Missing** | Only `AgentTrigger.sessionKeyFrom` (event→session hashing, `operator/internal/server/trigger_session.go`). Agents themselves are stateless; no session lifecycle, no idle-timeout/max-lifetime controls |
| H4 | Memory | ❌ **Missing** | No conversation persistence, no memory backend (Redis/Postgres), no checkpointing. The only persisted state is the A2A plugin's task state in a ConfigMap |
| H5 | Sandboxed execution & built-in tools | ❌ **Missing** | No code interpreter, no browser, no sandbox. (Likely out of scope for v1 — see §4) |
| H6 | Tool connectivity | 🟡 **Partial** | OpenAPI tools are solid: spec parsing, OAuth2/service-account credential exchange, SSRF validation (`sdk/python/flokoa/src/flokoa/tools/openapi/`). **MCP is not integrated** — `flokoa-codemode-mcp` is ~70% done with zero tests and no entrypoint. No central gateway/policy layer |
| H7 | Model agnosticism | ✅ **Strong** | `ModelProvider` supports OpenAI, Anthropic, Google, Bedrock with secret refs, base URLs, TLS (`operator/api/v1alpha1/modelprovider_types.go`). No mid-session switching (no sessions yet), but config-level agnosticism is real |
| H8 | Identity | 🟡 **Partial** | OIDC (Dex) on the control-plane gRPC/REST API (`operator/internal/server/auth.go`); secrets via `SecretKeySelector`; Argo token validation in the A2A plugin; 27 K8s RBAC roles. But: **agent A2A endpoints are unauthenticated**, the gRPC server is plaintext (no TLS), and there is no API-level authorization (any authenticated user can do anything) |
| H9 | Observability & cost | 🟡 **Partial** | OTel tracing in operator + A2A plugin (W3C traceparent propagation, `operator/internal/telemetry/`); optional FastAPI + pydantic-ai instrumentation in the SDK (`flokoa/telemetry.py`). But: no GenAI semantic conventions, no token/cost metrics, app-level Prometheus metrics unpopulated, no cost controls/quotas |
| H10 | Developer experience | 🟡 **Weak** | `flokoa run -m module:agent` serves an agent locally over A2A — good. But no `init`/scaffold, no `deploy`/`invoke`/`logs`/`traces` commands, root README is 8 bytes, SDK pyproject description is boilerplate, docs cover CRDs but not the SDK or API |

**Flokoa capabilities AgentCore's harness does *not* lead with** (differentiators to keep in the pitch):

- **Multi-agent workflows**: `AgentWorkflow` → Argo compilation with DAG dependencies, conditions, retries, output passing, and a 4,000-line test suite (`operator/internal/controller/agentworkflow_compiler.go`). AgentCore has nothing equivalent in the harness itself.
- **A2A protocol end-to-end**: SDK serves A2A, the Argo executor plugin speaks A2A, agent cards on the CRD.
- **Kubernetes-native + open source**: runs in *your* cluster, GitOps-friendly CRDs, Apache 2.0 — the structural counter-position to a proprietary, 4-region AWS preview.
- **Control-plane API**: 6 gRPC services + grpc-gateway REST + SSE watch + an agent playground endpoint (AG-UI streaming).

---

## 3. The honest verdict

**Flokoa is already an agent *deployment platform* with the right declarative shape, but it is not yet an agent *harness* by the AgentCore definition, because the two words doing the most work in that definition — "stateful" and "secure" — are the two biggest gaps.** A harness claim invites the immediate questions "where does conversation state live?" and "who can call my agent?", and today the answers are "nowhere" and "anyone with network reach."

There are two claims with different price tags:

1. **"Kubernetes-native agent harness"** (declarative config → running, observable, secured agent): achievable in roughly **Phase 0 + Phase 1** below.
2. **Full AgentCore parity** (per-session sandboxes, built-in code interpreter/browser, memory service): a much larger investment (Phase 2), and arguably not the fight to pick — Kubernetes-native positioning lets you define the category differently.

---

## 4. Roadmap: what it would take

### Phase 0 — Table stakes before advertising *anything* (release engineering, ~days–2 weeks)

These come straight from `RELEASE_REVIEW.md` and the audit; a harness claim on top of an uninstallable product backfires.

- [ ] Add the AgentWorkflow CRD to the Helm chart; make webhooks enableable via Helm (P0 blockers).
- [ ] Publish the managed runtime / task images (default AgentTask image currently 403s) and push the Helm chart to an OCI registry.
- [ ] Align versions (operator 0.1.0 vs chart appVersion 0.0.7 vs SDK 0.0.5) and cut a coherent release.
- [ ] Real root README + landing docs that state the positioning; fix the 3 broken doc links; replace the placeholder SDK package description.
- [ ] Keep CI green and get e2e running in CI (the `OPENAI_API_KEY` secret gap).

### Phase 1 — Minimum credible harness (the actual feature work, ~1–2 quarters)

**1. Sessions + memory (H3/H4) — the defining gap.**
- Add a session concept to the managed-agent runtime: `contextId`/session ID on A2A requests scopes conversation state (A2A already carries context IDs — lean on the protocol).
- Pluggable memory backend behind an interface: start with the operator's natural primitive (a per-agent PVC or an in-cluster store), add Redis/Postgres drivers. Expose as a `memory:` block on the Agent CRD (e.g. `memory: {type: redis, secretRef: ...}`) so it stays declarative.
- Persist transcripts per session; load history before reasoning on subsequent invocations — this is the exact behavior AgentCore advertises.
- Add idle-timeout / TTL semantics for sessions (CRD fields, mirroring `--idle-timeout` / `--max-lifetime`).

**2. Agent endpoint identity (H8).**
- Inbound auth on agent A2A endpoints: JWT validation against a configured issuer, declared on the Agent CRD (`auth: {oidc: {issuerURL, audience}}`). You already have the go-oidc machinery in the server — reuse it in the managed runtime / an auth sidecar.
- TLS for the gRPC server; mTLS or NetworkPolicy-by-default between operator-managed components (templates already exist in `operator/config/network-policy/`, just not deployed).
- Basic authorization on the control-plane API (even namespace-scoped role checks) so "authenticated = admin" stops being true.

**3. MCP tool support (H6).**
- Let agents consume MCP servers: `AgentTool` gains `type: mcp` with a server URL/transport, and the managed runtime wires pydantic-ai's native MCP client support. This is comparatively cheap because pydantic-ai already ships MCP support — most of the work is CRD plumbing + the type-generation pipeline.
- Decide the fate of `flokoa-codemode-mcp`: finish it as a differentiator or park it; don't ship it half-built.

**4. GenAI observability (H9).**
- Adopt OTel GenAI semantic conventions in the managed runtime (model name, token counts, tool-call spans) — pydantic-ai's instrumentation gets you most of the way; ensure it's on by default in the template runtime, not optional.
- Populate Prometheus metrics: invocations, latency, token usage per Agent. Token metrics are the prerequisite for any future cost-control story.
- One Grafana dashboard + docs page showing a full trace of agent → model → tool → sub-agent. This single artifact does more for the "harness" perception than any feature.

**5. Harness-grade DX (H10).**
- `flokoa init` (scaffold an agent project), `flokoa deploy` (render + apply the CRDs), `flokoa invoke` (call an agent over A2A), `flokoa logs` — mirroring the `agentcore` CLI verbs developers now expect.
- A "first agent in 5 minutes" quickstart that goes config → running agent → traced invocation.

### Phase 2 — Parity/differentiation (post-claim, pick deliberately)

- **Per-session isolation / sandboxed execution (H2/H5):** session-scoped pods or Kata/gVisor-class sandboxes, code-interpreter tool. Big lift; only pursue if untrusted-code execution becomes a target use case. Until then, *say clearly in docs that Flokoa agents share pod-level isolation* — honesty here protects the harness claim.
- **Tool gateway (H6):** centralized policy ("which agents may call which tools"), per-tool-call audit logging. A natural extension of the existing AgentTool + OIDC machinery.
- **Cost controls (H9):** per-Agent/namespace token budgets and quotas, enforced in the runtime.
- **Hot-reload of agent config**, canary/versioned Agent rollouts, HPA for agent runtimes (the chart only has HPA for the gRPC server today).
- **Second runtime framework** in the managed path (google-adk exists in the SDK but the template runtime is pydantic-ai only); finish or cut `flokoa-managed-task`.

---

## 5. Suggested positioning (once Phase 0+1 land)

> **Flokoa — the open-source agent harness for Kubernetes.** Declare your agent's model, instructions, tools, and memory as CRDs; Flokoa runs it: a managed runtime with sessions and memory, MCP and OpenAPI tool connectivity, OIDC-secured A2A endpoints, OpenTelemetry GenAI tracing, and Argo-powered multi-agent workflows. Any cluster, any model provider, no lock-in.

Every clause in that sentence must be true before it ships. Today, "sessions and memory," "MCP," "OIDC-secured A2A endpoints," and "GenAI tracing" are not — that's the Phase 1 list, verbatim.

---

## 6. Capability scorecard (summary)

| Harness capability | Flokoa today | After Phase 1 |
|---|:---:|:---:|
| H1 Declarative config | ✅ | ✅ |
| H2 Managed runtime | 🟡 | 🟡 (pod-level isolation, documented) |
| H3 Stateful sessions | ❌ | ✅ |
| H4 Memory | ❌ | ✅ (pluggable backends) |
| H5 Sandbox / built-in tools | ❌ | ❌ (deliberate, Phase 2) |
| H6 MCP + tool gateway | 🟡 | ✅ MCP / 🟡 gateway |
| H7 Model agnosticism | ✅ | ✅ |
| H8 Identity | 🟡 | ✅ |
| H9 Observability / cost | 🟡 | ✅ obs / 🟡 cost |
| H10 DX (CLI, docs) | 🟡 | ✅ |
| Multi-agent workflows (Flokoa edge) | ✅ | ✅ |
