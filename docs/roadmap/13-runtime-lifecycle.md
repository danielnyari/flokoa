# 13 — Runtime Lifecycle: Probes, HPA, Hot Reload, Isolation

**Phase:** 2 (probes/HPA are Phase-1-adjacent quick wins; isolation is a research track) · **Size:** L overall, separable · **Depends on:** 04 (scale-out needs shared sessions)

This unit bundles the "boring platform" lifecycle gaps. The first three sections are concrete and independently shippable; the fourth is a decision document.

## 13.1 Probes (S — do first)

**Current**: the managed runtime mounts a `health_router` (`flokoa_managed_agent/__main__.py`), but `BuildDeployment` (`internal/infra/builder/deployment.go`) sets **no liveness/readiness probes** — Kubernetes can't tell a wedged agent from a healthy one, and rollouts can't gate on readiness.

**Target**: builder adds HTTP probes against the health route (confirm exact path from the `health_router` definition) on `FLOKOA_PORT` for template runtimes: readiness (period 5s) + liveness (period 15s, generous initial delay for model/MCP warmup). Readiness should reflect dependency health where cheap: 04's Redis ping and 07's MCP reachability belong in the readiness handler (extend the router in `flokoa-managed-agent`), not liveness (a down Redis must not restart-loop pods). `StandardRuntimeSpec` users keep authoring their own probes via the existing `corev1.Container` passthrough.

**Accept**: kill -STOP a runtime process in e2e → pod restarts; Redis down → pod NotReady, not restarting; `Agent.status.availableReplicas` (already surfaced) reflects it.

## 13.2 HPA for agents (M)

**Current**: `Agent.spec` exposes fixed replicas (via `DeploymentOverrides`); the chart's HPA template covers only the gRPC server (`charts/flokoa/templates/server/hpa.yaml`). Agent scaling is manual.

**Target**:

```go
// AgentAutoscalingSpec — minimal HPA projection.
type AgentAutoscalingSpec struct {
    MinReplicas *int32 `json:"minReplicas,omitempty"` // default 1
    MaxReplicas int32  `json:"maxReplicas"`
    // TargetCPUUtilizationPercent; CPU-only v1 (custom metrics need an adapter; defer).
    // +kubebuilder:default=80
    TargetCPUUtilizationPercent *int32 `json:"targetCPUUtilizationPercent,omitempty"`
}
```

`AgentSpec.Autoscaling *AgentAutoscalingSpec`; operator creates/owns an `autoscaling/v2` HPA targeting the agent Deployment (new `HPARepo` in `internal/infra/repo` + pure builder, `Owns()` registration in `SetupWithManager`); webhook rejects `autoscaling` combined with explicit replicas. **Hard prerequisite**: document that multi-replica without `spec.memory` (04) silently splits sessions — webhook *warning* when autoscaling is set and memory is absent/inMemory. Requests/limits already flow via `TemplatedRuntimeSpec.Resources`, so CPU targets are meaningful. Token-throughput-based scaling (off 08's metrics + prometheus-adapter) is the documented follow-up, not v1.

**Accept**: load test in Kind scales an agent 1→3 and back; sessions stay coherent (Redis-backed).

## 13.3 Hot reload of agent config (M — mostly verification + gap-closing)

**Current — closer than it looks**: the runtime already re-reads mounted config through `ConfigCache` (TTL + file-mtime invalidation, `FLOKOA_CACHE_TTL_SECONDS`): `FlokoaAgentExecutor.tool_definitions` and `model_config` reload when files change, `instruction` re-reads `/etc/flokoa/instruction.txt` per access, and the pydantic-ai executor rebuilds toolsets when definitions change (`_get_toolset` identity check). Kubelet propagates ConfigMap updates to mounts (~1 min). So tools/model/instruction edits *should* already apply without restarts — **but this is untested and undocumented**, and anything env-projected (memory 04, auth 05, limits 12) requires a rollout by nature.

**Target**: (a) e2e test pinning the live-reload behaviors that work (edit Instruction CR → next invocation uses new prompt; same for tool add/remove); (b) for env-projected config, operator triggers clean rollouts via a pod-template annotation `flokoa.ai/config-hash` = `hash.JSONStruct` over the projected env/spec (the AgentWorkflow compiler's SpecHash drift pattern, reused) so changes roll deterministically instead of "whenever the Deployment happens to diff"; (c) document the matrix (live-reload vs rollout) in `reference/runtime-contract.md`.

**Accept**: instruction edit visible in next invocation without restart; memory-config edit causes exactly one rolling restart.

## 13.4 Session isolation (decision document — no code)

**Current**: agents are long-lived shared Deployments; all sessions share a process. Fine for trusted-tool LLM agents; insufficient for agent-authored code execution.

**Options to evaluate** (write up as `13a-isolation-rfc.md` when this becomes a user demand):
1. **RuntimeClass passthrough** (gVisor/Kata on the agent Deployment) — pod-level hardening, near-zero flokoa code (verify `DeploymentOverrides` can carry `runtimeClassName`; add if not). Cheapest meaningful step.
2. **Sandboxed code-execution *tool*** — keep the runtime shared, isolate only code execution: an MCP code-interpreter server per session/job. Note `flokoa-codemode-mcp` already runs code in a `pydantic-monty` sandbox (14 decides that package's fate) — it could grow into this.
3. **Per-session pods** (Job or Knative-style scale-from-zero per contextId) — the true microVM-parity shape; large: needs a session router, cold-start budget, and rethinking the Deployment model.

Recommendation embedded in the docs until then: state pod-level isolation plainly (10's honesty rule) and route "untrusted code" use cases to option 2.

## Suggested split for implementation

Three PRs (13.1, 13.2, 13.3) in that order — each independently valuable; 13.4 is a markdown deliverable only.
