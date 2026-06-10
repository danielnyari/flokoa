# 15 — SwarmRun & Loop Engineering (Design Sketch)

**Phase:** P2 — payoff, not a separate product (brief §6) · **Size:** XL · **Depends on:** 13/14 (a swarm *is* a session-tier sandbox), 07 (budgets), AgentTrigger (exists) · **Upstream gate:** harness sub-agents maturity; Teams when shipped

This is a design sketch to be hardened into a full spec when P1 lands and upstream sub-agents/Teams maturity is reassessed. Decisions below are directional.

## Model

**Swarm-in-a-box** (brief §6, decision 3): one session-tier sandbox runs a pydantic-ai program using harness sub-agents (Teams when shipped). In-process coordination — typed outputs, shared deps, one ordinary local filesystem at `/workspace`. **No A2A between swarm members.** A2A is the boundary: how the run is started, observed, and how results deliver (push notifications — `push_gateway.go` machinery exists).

## SwarmRun CRD (sketch)

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: SwarmRun
spec:
  requirement: |            # the high-level prompt ("loop engineering" input)
    Implement X per the linked issue; tests green.
  workspace:                # what lands in /workspace before the loop starts
    git: {url, ref, credentialsSecretRef}   # v1: git source; emptyDir scratch otherwise
  swarmRef: {name}          # an Agent CR (isolation: session) whose spec IS the orchestrator —
                            #   reference orchestrator or user-composed (capabilities: planning,
                            #   sub-agents, verification-loop, filesystem, shell…)
  budget: {maxTotalTokens, maxDuration, maxModelRequests}   # enforced by the injected guardrail
  termination: {onBudget: fail|deliver-partial, approval: none|required}
  delivery:                 # A2A push notification target(s) + artifact publication
    pushTo: {url|agentRef}
    artifacts: {git: {pushBranch}|none}     # v1: push branch back; richer sinks later
status:
  phase: Pending|Provisioning|Running|AwaitingApproval|Succeeded|Failed|Exceeded
  contextId: …              # the run IS a session; observability/persistence ride 12/13
  usage: {tokens, requests, duration}
  conditions: […]
```

A SwarmRun controller: claims a sandbox (14's machinery with `maxLifetime` semantics added), materializes the workspace, sends the requirement as the first A2A message, watches budget/termination via the guardrail's reported usage (13 store), delivers results, reaps. `kubectl get swarmruns` is the operator-legible loop surface; `AgentTrigger` gains a SwarmRun target (fire loops from GitHub events — the brief's headline demo).

## Reference orchestrator

Shipped as a flokoa-published **capability bundle + example Agent CR**, not new orchestration code: harness planning + sub-agents + verification-loop + filesystem + shell capabilities composed declaratively. Orchestration intelligence stays upstream (brief §10); flokoa contributes the *operational* loop: provisioning, budget, trigger, delivery, observability. If Teams ships before P2 starts, the reference swaps sub-agents for Teams — the CRD is agnostic to which.

## Hard questions to resolve in the full spec

1. **Human-in-the-loop**: harness tool-approval surfaces where? (Likely: `AwaitingApproval` phase + approval via CR annotation/subresource + CLI `flokoa swarmrun approve` — needs a real design pass.)
2. **Mid-run durability**: a reaped/evicted sandbox mid-loop loses workspace + plan state; conversation persists (13) but a coding loop is more than conversation. v1 stance: SwarmRuns fail on eviction and are retriable (idempotent workspace from git); checkpoint/resume is the v2 ambition alongside snapshot/restore. State this limit honestly.
3. **Long-run A2A semantics**: a loop is hours, not turns — push-notification cadence, intermediate status artifacts, cancellation propagation (A2A task cancel → guardrail abort hook) need protocol-level design against the pinned a2a-sdk.
4. **Budget truthfulness**: guardrail counts what pydantic-ai sees; shell/code tools can spend externally (e.g. calling APIs from generated code). Scope the claim: token/request budgets bound LLM spend only.
5. **Teams adoption criteria**: what maturity bar (released, versioned, in a Capability artifact that passes 08's checks) flips the reference orchestrator.

## Acceptance criteria (for the eventual implementation)

- `kubectl apply` of a SwarmRun against a repo produces a branch with the change, within budget, observable end-to-end (trace + usage in status), triggered manually or by an AgentTrigger event.
- Budget breach terminates per `termination` policy with usage evidence; nothing about the run requires kubectl-exec archaeology.
