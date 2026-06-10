# 11 — Tool Gateway and Policy (Design Sketch)

**Phase:** 2 · **Size:** XL · **Depends on:** 07 (MCP), 05 (identity) · **Status:** design sketch — make the build/no-build decision before detailing

## Problem

After 07, every agent connects to tools directly with statically-provisioned credentials (`headerSecrets`, OpenAPI auth exchangers in `flokoa/tools/openapi/auth/`). At fleet scale this means: credentials sprayed across namespaces, no central "which agents may call which tools," and per-tool-call audit only as scattered client logs (`rest_api_tool.py` logs `TOOL CALL: …` locally). AgentCore's answer is a Gateway: one endpoint consolidating tool auth, policy, and per-call logging.

## Decision to make first

**In-runtime policy enforcement vs. a gateway data-plane component.** Recommendation: **start with policy-in-runtime (option A), keep the gateway (option B) as the scale follow-up** — A delivers 80% of the governance value with no new network hop or HA component, and its CRD surface is forward-compatible with B.

### Option A — `ToolPolicy` CRD + runtime enforcement (recommended first step)

- New namespaced CRD `ToolPolicy` (follows the house CRD conventions): selectors over agents (`agentSelector: metav1.LabelSelector`) and rules (`allow: [{toolRef|toolType|namePattern, operations: [...]}]`, default-deny when any policy selects the agent).
- Operator: a `PolicyReconciler` in `internal/app/agent/` projects the *resolved* policy for each agent into the existing config channel (`/etc/flokoa/policy.json`, new builder mount constant) — same projection pattern as tools/instructions. Agent reconcile watches `ToolPolicy` via a mapper like `findAgentsForAgentTool`.
- Runtime: a `PolicyEnforcingToolset` wrapper (pydantic-ai `WrapperToolset`) filtering tool listings and rejecting non-allowed calls; one wrapper around the 07 `_build_toolsets()` output. Audit: every allowed/denied call emits an OTel event/span attribute (`flokoa.tool.decision`) — rides 08's pipeline, queryable centrally without new storage.
- Effort: M once 07 lands.

### Option B — Gateway data plane (the AgentCore-shaped version)

- New deployment `flokoa-gateway` (Go, lives beside the A2A plugin in `operator/plugins/` or a new `gateway/` module): terminates MCP (streamable HTTP) and OpenAPI-proxy traffic from agents, re-resolves upstream credentials from Secrets *in the gateway's namespace* (agents never hold tool credentials), enforces `ToolPolicy`, emits per-call audit records (OTel + structured log), optional response caching.
- Agents authenticate to the gateway with their projected SA token (validated via OIDC per 05's deferred `kubernetes`-issuer pattern); the gateway threads the original caller identity (A2A metadata from 05) into audit records — end-user identity through to tool calls, the AgentCore Identity story.
- Operator changes: `MCPToolSpec`/`OpenApiToolSpec` gain `gatewayRef` (or operator policy flag rewrites tool endpoints to the gateway transparently — prefer explicit `gatewayRef` first).
- Open questions to resolve in a dedicated RFC: HA/scale model, streaming passthrough (MCP is stateful-ish over SSE/streamable HTTP), latency budget, whether OpenAPI proxying is in scope or MCP-only (recommend MCP-only v1 — OpenAPI tools can be fronted by an MCP adapter later).

## Suggested sequencing

1. Ship Option A (`ToolPolicy` + runtime enforcement + audit events) as the Phase 2 deliverable.
2. Write the Option B RFC referencing A's CRD; build only when a user has >dozens of agents/tools or a compliance requirement for credential centralization.

## Acceptance criteria (Option A)

- A `ToolPolicy` denying a tool makes it invisible to the agent's model and blocks direct invocation, with a deny event visible in traces; agents with no selecting policy behave as today.
- Policy changes propagate without pod restarts (config-channel + `ConfigCache` reload, same mechanism as tool updates).
