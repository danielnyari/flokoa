# 12 — Cost Controls and Usage Limits

**Phase:** 2 · **Size:** M (per-run limits) (+L for aggregate budgets, separate decision) · **Depends on:** 08 (token metrics)

## Goal

Bound what an agent can spend. Two distinct mechanisms with very different costs — ship the first, design-gate the second:

1. **Per-run limits** (this unit): hard caps on a single invocation — request count and token ceilings. pydantic-ai enforces these natively via `UsageLimits` passed to `agent.run()`, so this is almost pure CRD plumbing.
2. **Aggregate budgets** (follow-up RFC): "this agent/namespace may use N tokens per day" — requires durable accounting and an enforcement decision; sketched below, not built here.

## Current state

- No limits of any kind: `PydanticAIAgentExecutor.execute()` calls `agent.run(request, **run_kwargs)` with `toolsets`/`model`/`instructions` only. A prompt-injected tool loop can burn tokens until the provider rate-limits.
- 08 gives *visibility* (`gen_ai.client.token.usage` histogram from `result.usage()`) but no enforcement.
- CRD has nowhere to declare limits; `Model` CRD carries generation params (maxTokens per response) which is not the same thing as run budgets.

## Target design (per-run limits)

### CRD

```go
// api/v1alpha1/agent_types.go
// AgentLimitsSpec bounds a single invocation of the agent.
type AgentLimitsSpec struct {
    // MaxModelRequestsPerRun caps LLM round-trips in one invocation (tool-loop bound).
    // +kubebuilder:validation:Minimum=1
    // +optional
    MaxModelRequestsPerRun *int32 `json:"maxModelRequestsPerRun,omitempty"`
    // MaxTotalTokensPerRun caps input+output tokens in one invocation.
    // +kubebuilder:validation:Minimum=1
    // +optional
    MaxTotalTokensPerRun *int64 `json:"maxTotalTokensPerRun,omitempty"`
}
```

`AgentSpec.Limits *AgentLimitsSpec` (+optional). Projection: `FLOKOA_LIMIT_MAX_REQUESTS` / `FLOKOA_LIMIT_MAX_TOTAL_TOKENS` env via the shared `runtime_env.go` (04/05/08 pattern).

### Runtime

- Executor builds `UsageLimits(request_limit=…, total_tokens_limit=…)` (verify exact field names on the pinned pydantic-ai ≥1.44) and passes `usage_limits=` in `run_kwargs`.
- On `UsageLimitExceeded`: emit a **failed** `TaskStatusUpdateEvent` with a structured error message naming the limit (callers must distinguish "budget hit" from "agent broke"), increment `flokoa.agent.requests{outcome="limit_exceeded"}` (08's counter gains the outcome value), and **do not append** the truncated run to the session (03's skip-on-failure rule already covers this).
- Session interaction: with history replay (03), input tokens grow per turn — document that `maxTotalTokensPerRun` includes replayed history, which is exactly why `FLOKOA_SESSION_MAX_TURNS` exists.

## Aggregate budgets — decision sketch (do not build without the RFC)

- **Accounting**: per-(agent, window) token counters need shared durable state. Natural home: the 04 store backend (Redis `INCRBY` + window keys) — *not* Prometheus (metrics are lossy and not transactional).
- **Enforcement point**: runtime checks budget before each run (soft-fail open vs hard-fail closed on store outage — policy decision), or a controller flips a `BudgetExhausted` condition + scales the Deployment to zero (coarse, but visible in `kubectl`). Lean toward runtime-check with fail-open + loud metric.
- **CRD**: `AgentBudgetSpec{window, maxTotalTokens}` or a namespace-level `TokenQuota` CRD mirroring `ResourceQuota` semantics. Namespace-level is the more Kubernetes-native shape; needs the RFC.
- **Pricing**: keep $ out of the control plane; tokens are the unit, dashboards translate (10's cost variables).

## Implementation plan (per-run scope)

1. CRD field + webhook sanity checks → `make manifests generate` + `make generate-python-models`.
2. Env projection + fakes tests.
3. Executor `usage_limits` wiring + limit-exceeded event/metric handling; unit tests with TestModel forcing a request-limit hit (TestModel's tool-call loop makes this deterministic).
4. Sample CR + `docs/guides/` snippet; e2e case: agent with `maxModelRequestsPerRun: 1` and a tool that the model would loop on → task fails with the structured limit error.

## Acceptance criteria

- An Agent with `spec.limits` fails fast and legibly when a run exceeds them; unset limits = current behavior; the limit outcome is observable in metrics.
- Budgets RFC exists as `docs/roadmap/12a-aggregate-budgets-rfc.md` with the decisions above resolved, before any aggregate-budget code.
