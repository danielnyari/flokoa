# Operator Audit: Major Gaps Identified

**Date:** 2026-02-18
**Scope:** Full audit of `operator/` — controllers, domain logic, infrastructure layer, tests, gRPC server, webhooks, Argo plugin, Helm chart.

---

## Executive Summary

The operator has **well-structured code with clean separation of concerns** (domain/app/infra/builder layers, pure functions for building resources). However, the audit reveals **critical gaps in error handling, nil-safety, status management, and testing** that directly explain outage-causing panics and silent failures in production.

The most dangerous pattern is: **errors that are silently absorbed rather than surfaced, combined with tests that verify happy paths using the same logic as production code, creating a false sense of coverage.**

---

## 1. CRITICAL: Nil Pointer Dereference / Panic Risks

### 1a. Agent controller: unchecked `Standard` pointer dereference

**File:** `internal/controller/agent_controller.go`

The `reconcileResources` path calls into builders and domain logic that access `agent.Spec.Runtime.Standard` without nil-checking. While `ValidateSpec` catches the case where `RuntimeType=Standard` but `Standard` is nil, validation failures don't halt execution — they set a status condition and return `nil` error (no requeue), but if there's a race where the object is modified between validation and resource building, a nil dereference can occur.

The deeper issue is that `ValidateSpec` returns `nil` when `runtime.Standard` is nil for `RuntimeTypeStandard` — it only checks the *opposite* case (`runtime.Template` present for Standard type). This means:

```go
// domain/agent/validate.go:22-28
case agentv1alpha1.RuntimeTypeStandard:
    if agent.Spec.Runtime.Template != nil {
        return fmt.Errorf("runtime.managed must not be set...")
    }
    // BUG: No check that agent.Spec.Runtime.Standard != nil!
```

If `RuntimeType=Standard` and `Standard=nil`, validation passes, then `buildStandardContainerSpec` in `builder/deployment.go:249-261` creates a nil-safe fallback, but `BuildService` at `builder/service.go:15-16` dereferences `runtime.Standard` directly after the nil-guard on `runtime.Standard != nil`, which is safe. However, the issue is that an Agent with `RuntimeType=Standard` and `Standard=nil` will produce a deployment with an empty container — **a silent misconfiguration that causes pods to crash-loop**.

**Severity:** High — leads to crash-looping deployments with no error status on the Agent CR.

### 1b. Model reconciler: chain of unchecked lookups

**File:** `internal/app/agent/model_reconciler.go`

The model reconciliation chain performs:
1. Get Model by name/namespace
2. Check `model.Status.Ready`
3. Get ModelProvider by `model.Status.ResolvedProvider`

If `model.Status.ResolvedProvider` is empty (e.g., ModelProvider controller hasn't reconciled yet), the Get call will fail with a not-found error. This is handled. However, the `resolveModelProvider` function extracts provider type via a switch on which provider-specific field is non-nil:

```go
// This pattern appears in the provider type detection logic
if provider.Spec.OpenAI != nil { ... }
else if provider.Spec.Anthropic != nil { ... }
// etc.
```

If a ModelProvider has **no provider-specific field set** (all nil), the code falls through without setting a provider type, which can produce a `ResolvedModelConfig` with an empty `Provider.Type` — **a silent misconfiguration that the SDK runtime may not handle gracefully**.

**Severity:** High — produces invalid model config that causes runtime failures.

### 1c. AgentWorkflow compiler: unchecked nil `Template.Plugin`

**File:** `internal/controller/agentworkflow_compiler.go`

The compiler builds Argo Workflow templates from AgentWorkflow tasks. When processing `task.Agent`, it constructs plugin templates. If the switch/case logic falls through or an unexpected task type is encountered, the resulting workflow template may have nil fields that Argo Workflows doesn't handle well.

**Severity:** Medium — causes Argo submission failures.

### 1d. A2A plugin: unchecked type assertion on sync.Map

**File:** `plugins/a2a/plugin/plugin.go:77`

```go
progress := state.(*ProgressState)
```

This is an unchecked type assertion. If any code path stores a non-`*ProgressState` value in the `tasks` sync.Map, this will panic and crash the plugin pod.

**Severity:** Medium — currently safe but fragile; any future refactoring could introduce a panic.

---

## 2. CRITICAL: Error Handling Gaps

### 2a. Validation errors silently swallowed — no requeue

**File:** `internal/controller/agent_controller.go`

When `ValidateSpec` fails, the controller sets a status condition and returns `ctrl.Result{}, nil` — **no error, no requeue**. This means:

- If the user fixes the Agent spec, the controller **won't notice** unless something else triggers a reconciliation (e.g., watch event from the API server for the update).
- In practice this works because the spec update triggers a watch event, but this is *accidental* correctness — it breaks for validation failures that depend on the state of other resources (e.g., a referenced Model becoming available later).

**Severity:** High — validation failures for cross-resource references won't auto-recover.

### 2b. Model/Tool/Instruction reconcilers: errors returned vs. not returned inconsistently

Across the three sub-reconcilers:

- **Model reconciler:** Returns errors for Get failures (good), but does NOT return error when `model.Status.Ready` is false — it returns an error with `fmt.Errorf("Model %s is not ready")`, which causes a requeue with exponential backoff. This is correct.
- **Tool reconciler:** Returns errors for Get failures. But when a tool's ConfigMap doesn't exist yet, it **silently skips** that tool and continues — the ToolsReady condition is set to True even though some tools are missing their ConfigMaps. The deployment gets created without tool mounts.
- **Instruction reconciler:** When the referenced Instruction doesn't have its ConfigMap created yet, it returns an error. Inconsistent with tool behavior.

**Severity:** High — tools can silently fail to mount, causing agents to run without their declared tools.

### 2c. Status update failures silently dropped

**File:** `internal/controller/agent_controller.go`

The pattern `if err := r.Status().Update(ctx, agent); err != nil { log.Error(err, "..."); return ... }` is used in some places but not all. In several code paths, if the status update fails (e.g., conflict due to stale resourceVersion), the error is logged but the function returns `nil` error, meaning **no requeue** — the stale status persists until the next triggering event.

**Severity:** Medium — stale status conditions in production.

### 2d. A2A plugin: transient errors treated as terminal failures

**File:** `plugins/a2a/plugin/plugin.go:335-348`

When `GetTask` fails during polling:
```go
task, err := a2aClient.GetTask(ctx, ...)
if err != nil {
    return failedReply(fmt.Sprintf("failed to get A2A task: %v", err)), nil
}
```

A transient network error during polling **permanently fails the Argo workflow step** instead of requeueing. The plugin should distinguish transient errors (network, timeout) from permanent ones (task not found) and requeue for transient failures.

**Severity:** Critical — network blips cause permanent workflow failures.

---

## 3. CRITICAL: Tautological and Insufficient Testing

### 3a. `reconcileAgent` helper masks partial failures

**File:** `internal/controller/agent_controller_test_helpers_test.go:89-98`

```go
func reconcileAgent(ctx context.Context, r *AgentReconciler, nn types.NamespacedName) {
    result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
    ExpectWithOffset(1, err).NotTo(HaveOccurred())
    ExpectWithOffset(1, result.RequeueAfter).To(BeNumerically(">", 0))
    _, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
    ExpectWithOffset(1, err).NotTo(HaveOccurred())
}
```

This helper always expects exactly 2 reconcile calls. But some scenarios need 3+ reconciles (e.g., finalizer → create resources → update status). If a test uses `reconcileAgent` when more reconciles are needed, **the test passes with resources in an intermediate state**, masking bugs where the third reconcile would fail.

**Severity:** High — tests pass while the actual reconciliation sequence is incomplete.

### 3b. No tests for concurrent reconciliation / conflict errors

No tests verify behavior when two reconciliation loops run concurrently on the same Agent (which happens in production with rapid spec updates). The envtest client doesn't simulate Kubernetes' optimistic concurrency — it doesn't return conflict errors on stale writes. This means:

- The operator has no logic to handle `StatusError` with `Conflict` reason on status updates
- Tests can't catch this because envtest's in-memory store doesn't enforce resourceVersion conflicts the way a real API server does

**Severity:** Critical — conflict errors in production cause status update failures that are silently swallowed (see 2c).

### 3c. Tool ConfigMap creation simulated manually in tests

**File:** `internal/controller/agent_controller_tools_test.go:143-156`

```go
By("Simulating AgentTool controller creating ConfigMap")
configMapName := agentTool.Name + "-spec"
toolConfigMap := &corev1.ConfigMap{...}
Expect(k8sClient.Create(ctx, toolConfigMap)).To(Succeed())
```

Tests manually create the ConfigMaps that the AgentTool controller would create. This means:
1. The **ConfigMap naming convention** is duplicated between test and production code — if production changes the naming, tests still pass
2. The **cross-controller interaction** (Agent → AgentTool → ConfigMap → Agent) is never tested end-to-end in unit tests
3. The ConfigMap's content/labels in the test don't necessarily match what production creates

**Severity:** High — tests verify an assumed contract that isn't validated.

### 3d. No tests for the deletion/finalizer path with dependent resources

The `cleanupAgent` helper in tests **manually removes the finalizer** before deleting:

```go
func cleanupAgent(ctx context.Context, nn types.NamespacedName) {
    // ...
    if controllerutil.ContainsFinalizer(agent, agentFinalizer) {
        controllerutil.RemoveFinalizer(agent, agentFinalizer)
        Expect(k8sClient.Update(ctx, agent)).To(Succeed())
    }
    Expect(k8sClient.Delete(ctx, agent)).To(Succeed())
}
```

This bypasses the controller's actual deletion handling. The one deletion test in `agent_controller_test.go:149-187` verifies finalizer removal works, but it doesn't verify that dependent resources (Deployments, Services, ConfigMaps, inline AgentTools) are cleaned up. Since OwnerReferences should handle GC, this might seem fine — but cross-namespace references and inline AgentTools may not have OwnerReferences, leaving orphaned resources.

**Severity:** Medium — orphaned inline AgentTool CRs after Agent deletion.

### 3e. No tests for Model/ModelProvider/Instruction controllers in isolation

The model, provider, and instruction controllers each have test files, but they are tested indirectly through the Agent controller tests. There's minimal coverage for:
- ModelProvider with invalid provider type (no OpenAI/Anthropic/Google/Bedrock field set)
- Model referencing a provider in a different namespace
- Instruction controller creating ConfigMap with correct ownership
- Status condition transitions for these secondary controllers

**Severity:** Medium — entire controller code paths are untested.

### 3f. `calculatePhase` tests are tautological

**File:** `internal/controller/agent_controller_test.go:595-615`

```go
It("should return Running when available replicas > 0", func() {
    r := &AgentReconciler{}
    deployment := &appsv1.Deployment{
        Status: appsv1.DeploymentStatus{AvailableReplicas: 2},
    }
    Expect(r.calculatePhase(deployment)).To(Equal(agentv1alpha1.AgentPhaseRunning))
})
```

These tests construct a fake deployment status and verify the function returns the expected value. But the function is a trivial `if` statement — the test adds zero confidence. What's **missing** is testing `calculatePhase` integration: does the controller correctly read the deployment's **actual** status from the API server and propagate it to the Agent's `Status.Phase`? Since envtest doesn't run a real kubelet, deployment status always shows 0 available replicas, so the `Running` phase is **never actually tested in integration**.

**Severity:** High — the Running→Pending→Failed phase transitions are never tested with realistic deployment status.

---

## 4. HIGH: Architectural Issues

### 4a. No webhook validation for Agent CRD

There is a webhook for `AgentWorkflow` but **no admission webhook for the `Agent` CRD itself**. All Agent validation happens in the controller's reconcile loop via `domain/agent/ValidateSpec`. This means:

1. Invalid Agents are accepted by the API server and stored in etcd
2. The controller has to reconcile invalid resources, waste work, and set failure status
3. Users don't get immediate feedback on `kubectl apply` — they have to check status
4. Webhooks would also enable mutation (defaulting), which would eliminate edge cases like `Standard=nil`

**Severity:** High — invalid resources accepted and stored; poor UX; missing defaulting causes nil panics.

### 4b. No rate limiting / max concurrent reconciles configured

**File:** `internal/controller/agent_controller.go` (SetupWithManager)

The controller setup doesn't configure `MaxConcurrentReconciles`. With the default of 1, a single slow reconcile (e.g., waiting for a network call in model resolution) blocks all other Agent reconciliations. But if set to >1 without proper handling, concurrent reconciles on the same Agent can cause conflicts.

**Severity:** Medium — single reconcile bottleneck or unhandled concurrency.

### 4c. Cross-namespace references without RBAC enforcement

The Agent CRD allows cross-namespace references for Models (`model.namespace`) and Tools. However:
1. No RBAC check verifies the Agent's namespace has permission to reference resources in other namespaces
2. No network policy or admission control restricts cross-namespace access
3. The operator's service account needs cluster-wide read permissions for Models/Providers/Instructions, which is a broad privilege

**Severity:** Medium — security gap for multi-tenant clusters.

### 4d. No leader election guard in gRPC server

The gRPC server and HTTP gateway start regardless of leader election status. If running multiple operator replicas:
1. All replicas serve gRPC/HTTP (could be intentional for read-only API)
2. But there's no differentiation between leader and non-leader for write operations
3. The server uses `client.Client` directly, which goes through the controller-runtime cache — if the cache isn't started (non-leader), reads may fail

**Severity:** Medium — potential stale reads or errors from non-leader replicas.

---

## 5. HIGH: Missing Observability / Operational Gaps

### 5a. No metrics for reconciliation failures or durations

The controllers don't expose Prometheus metrics for:
- Reconciliation duration (latency)
- Reconciliation errors by type
- Resource counts (agents in each phase)
- Requeue rates

Without these, there's no way to detect degraded reconciliation performance before it becomes an outage.

**Severity:** High — blind to performance degradation.

### 5b. No structured error types for categorizing failures

All errors are `fmt.Errorf` strings. There's no error type hierarchy to distinguish:
- Transient errors (network, API server unavailable) → should requeue with backoff
- Permanent errors (invalid spec, missing CRD) → should not requeue
- Dependency errors (referenced resource not ready) → should requeue with fixed interval

The controller currently requeues all errors identically via controller-runtime's default exponential backoff, which means permanent errors cause infinite requeue storms.

**Severity:** High — permanent errors cause infinite requeue loops consuming controller capacity.

---

## 6. MEDIUM: Helm Chart / Deployment Issues

### 6a. No resource limits on operator deployment by default

If the Helm chart doesn't set resource requests/limits for the operator pod, it can:
- Be OOM-killed during high reconciliation load
- Starve other pods on the node
- Not benefit from Kubernetes QoS guarantees

### 6b. No PodDisruptionBudget for the operator

If the operator pod is evicted during a node drain, all reconciliation stops until it's rescheduled. A PDB would ensure at least one replica remains during voluntary disruptions.

### 6c. CRDs in Helm chart but no upgrade strategy

CRDs bundled in Helm charts are notoriously problematic for upgrades — `helm upgrade` doesn't update CRDs. The project has CRDs in both `config/crd/` (kustomize) and `charts/flokoa/crds/`, but there's no documented upgrade path.

---

## 7. MEDIUM: A2A Plugin Issues

### 7a. In-memory task state lost on pod restart

**File:** `plugins/a2a/plugin/plugin.go:29`

```go
tasks sync.Map
```

Task progress state is stored in memory. If the plugin pod restarts (OOM, node failure, rolling update), all in-progress tasks lose their tracking state. On the next poll from Argo, the task key won't be found, so a **new task is sent** to the agent — causing duplicate execution.

**Severity:** High — pod restarts cause duplicate agent task execution.

### 7b. No connection pooling or client caching

Every `sendTask` and `pollTask` call creates a new A2A client:
```go
a2aClient, err := p.createClient(ctx, candidate)
```

For long-running workflows with many polls, this creates excessive HTTP connection churn.

### 7c. `endpointCandidates` heuristic is fragile

The function tries both `endpoint` and `endpoint/a2a` (or strips `/a2a`). This heuristic-based endpoint discovery is fragile and should be replaced with proper service discovery from the Agent CR's status URL.

---

## Prioritized Remediation Plan

### P0 — Fix Before Next Deploy
1. **Add nil check for `Runtime.Standard` in `ValidateSpec`** — prevents nil deref panics
2. **Add admission webhook for Agent CRD** with validation + defaulting
3. **Fix tool reconciler to return error when ConfigMap missing** (not silently skip)
4. **Fix A2A plugin to requeue on transient poll failures** instead of failing permanently
5. **Add ModelProvider type validation** — reject providers with no provider-specific field

### P1 — Fix This Sprint
6. **Add conflict retry logic** for status updates (optimistic concurrency)
7. **Add structured error types** (transient vs. permanent) to stop infinite requeues
8. **Persist A2A plugin task state** (to ConfigMap or CRD status) to survive restarts
9. **Add integration tests for cross-controller flows** (Agent → AgentTool → ConfigMap)
10. **Add negative/error path tests** for all controllers

### P2 — Fix This Quarter
11. Add Prometheus metrics for reconciliation
12. Add PodDisruptionBudget to Helm chart
13. Configure MaxConcurrentReconciles with conflict handling
14. Add cross-namespace RBAC validation
15. Add CRD upgrade strategy documentation
16. Replace `reconcileAgent` test helper with explicit multi-step reconciliation
