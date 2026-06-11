# 14 — Isolation Tiers & Warm Pools

**Phase:** P1 · **Size:** L · **Depends on:** 11 (sizing math), 12/13 (claim/registry) · **Enables:** 15 (a swarm is a session-tier sandbox)

## Goal

Implement `runtime.isolation: session` end-to-end: sandbox pods per A2A context with honest, tiered hardening (runc-hardened default; RuntimeClass passthrough where clusters provide gVisor/Kata), warm pools owned by a controller reconciling a Pool CR, TTL lifecycle, and the injected **budget-guardrail** capability. Plus the small lifecycle hygiene the session tier forces: probes.

## Design

### Isolation tiers (brief §5, honest version)

- `shared` (default): unchanged.
- `session`: the operator stops managing a long-lived Deployment for the agent; instead the **pool controller** manages sandbox pods (one per claimed context) from the same pod template the builder already produces (runner + capability initContainers + injected env).
- Hardening tiers on the pod template:
  - **Tier 0 (always)**: `runAsNonRoot`, `readOnlyRootFilesystem` (writable `/workspace` emptyDir — the session filesystem), `allowPrivilegeEscalation: false`, dropped capabilities, `seccompProfile: RuntimeDefault`, per-sandbox NetworkPolicy (egress to providers/MCP endpoints + router ingress only — template in chart).
  - **Tier 1 (declared)**: `runtime.sandbox.runtimeClassName` passthrough. The operator **detects** referenced RuntimeClasses (existence check) and surfaces `SandboxRuntimeAvailable` conditions; per-cloud guidance docs (GKE Sandbox/gVisor; EKS self-managed runsc; AKS Kata) — flokoa never installs runtimes.
  - Language rule (binding): "defense-in-depth isolation via standard Kubernetes RuntimeClasses" — never "secure" — until a threat model is published (separate doc, tracked).

### Pool CR + controller

```go
type AgentPoolSpec struct {   // name TBD: AgentPool
    AgentRef NamespacedRef `json:"agentRef"`
    WarmSize int32         `json:"warmSize"`        // default 0 (brief §5)
    MaxSandboxes *int32    `json:"maxSandboxes,omitempty"` // hard cap; claims beyond → router 503s
    IdleTTL  metav1.Duration `json:"idleTTL"`        // park/reap threshold
}
```

- Created by the agent controller for session-tier agents (owned; `runtime.pool.{warmSize,maxSandboxes,idleTTL}` on the Agent is the user surface — the Pool CR is machinery, visible for ops).
- Reconciles: maintain `warmSize` hydrated-and-ready sandboxes (per-agent pools — generic pools rejected: claim-time hydration costs seconds and per-agent is the brief's default-0 model anyway); replace claimed ones; reap per TTL using registry `last_activity` (13); honor `maxSandboxes`.
- Claim protocol: router calls registry `ResolveOrClaim` (13) which transitions a `warm` sandbox to `claimed`; the controller observes via the registry (not pod labels) and refills. Controllers own lifecycle; the router only routes (tenet 5).
- Reap = delete pod + mark session `parked` (history persists via 13; a new message on a parked context claims a fresh sandbox and rehydrates). Sizing math from the 11 report goes in the Pool docs.

### Probes (forced by this unit, fixes an old gap)

The builder adds readiness/liveness HTTP probes against the runner's health route for all runner pods (shared and sandbox) — readiness gates pool "warm" status; the runner's health endpoint gains a `ready-after-bootstrap` semantic (bootstrap pipeline completion, 05).

### Injected capability: `flokoa.platform/budget-guardrail`

- Per-run enforcement on the runner (07 mechanism): pydantic-ai usage limits (request count, total tokens per run) from `Agent.spec.limits` (new small typed block), structured limit-exceeded task errors, `outcome=limit_exceeded` metrics.
- Session-scope budgets (token ceiling per context, enforced via hook-checked counters kept in the 13 store) — the primitive SwarmRun budgets (15) build on. Aggregate/window budgets stay a later RFC.

## Implementation plan

1. CRD: `isolation` enum unlock + `runtime.sandbox`/`runtime.pool`/`spec.limits` fields; AgentPool CRD; webhooks (session tier requires state backend configured; runtimeClass existence warning).
2. Pool controller (new `internal/controller/agentpool_controller.go` + app/infra layering as established) against the 13 registry; chaos tests (kill warm pods, kill claimed pods, registry hiccups).
3. Hardened pod template + NetworkPolicy + probes in the builder (pure-function tests).
4. Budget-guardrail capability + limits plumbing.
5. E2E (Kind): session-tier agent with warmSize=1 — first message claims warm (fast), second context cold-claims; idle TTL reaps; parked context resumes with history; budget breach returns the structured error.

## Acceptance criteria

- `isolation: session` works end-to-end on vanilla Kind at tier 0 with measured claim latencies consistent with the 11 report; RuntimeClass declared but absent → loud condition, not silent runc.
- Pools refill, cap, and reap exactly per spec; killing any sandbox loses at most that session's filesystem, never its conversation.
- Budget limits enforce in-band with legible errors.

## Out of scope

- Snapshot/restore (v2+). Threat model doc (tracked, pre-"secure"-claims). Scale-from-zero generic pools (revisit post-11 only if per-agent economics fail). Per-session containers-in-shared-pod fallback (only if 11 forces it).
