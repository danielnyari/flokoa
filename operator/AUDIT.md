# Operator Audit: Major Gaps Identified

**Date:** 2026-02-18
**Scope:** Full audit of `operator/` — controllers, domain logic, infrastructure layer, tests, gRPC server, webhooks, Argo plugin, Helm chart.

---

## Executive Summary

The operator has **well-structured code with clean separation of concerns** (domain/app/infra/builder layers, pure functions for building resources). However, the audit reveals **critical gaps in error handling, nil-safety, status management, and testing** that directly explain outage-causing panics and silent failures in production.

The most dangerous pattern is: **errors that are silently absorbed rather than surfaced, combined with tests that verify happy paths using the same logic as production code, creating a false sense of coverage.**

---

## 2. CRITICAL: Error Handling Gaps

### 2e. Status update errors explicitly swallowed with `_ =` across 3 controllers

Across `agenttool_controller.go`, `agentworkflow_controller.go`, and `instruction_controller.go`, status updates in error paths are **intentionally discarded** using `_ =`:

**`agenttool_controller.go`:**
```go
// Line 107: validation failure path
_ = r.Status().Update(ctx, agentTool)  // error discarded
// Line 116: ConfigMap reconciliation failure path
_ = r.Status().Update(ctx, agentTool)  // error discarded
```

**`agentworkflow_controller.go`:**
```go
// Line 184: dependency resolution failure
_ = r.Status().Update(ctx, awf)  // error discarded
// Line 197: workflow compilation failure
_ = r.Status().Update(ctx, awf)  // error discarded
// Line 218: Argo Workflow creation failure
_ = r.Status().Update(ctx, awf)  // error discarded
// Line 253: Argo Workflow not found
_ = r.Status().Update(ctx, awf)  // error discarded
```

**`instruction_controller.go`:**
```go
// Line 102: ConfigMap reconciliation failure
_ = r.Status().Update(ctx, instruction)  // error discarded
```

**Total:** 7 instances of explicitly swallowed status update errors. If the API server is under pressure (exactly when you need status conditions most), these status updates fail silently, leaving stale/missing conditions that break observability.

**Severity:** High — status conditions lost during cluster instability when they're needed most.

### 2f. Finalizer removal blocked by cleanup failures in 3 controllers

When dependent resource deletion fails during the finalizer cleanup path, all three secondary controllers return the error **without removing the finalizer**, permanently blocking deletion:

**`agenttool_controller.go:82-88`:**
```go
if err := r.deleteConfigMap(ctx, agentTool); err != nil {
    logger.Error(err, "Failed to delete ConfigMap")
    return ctrl.Result{}, err  // Finalizer never removed → stuck in Terminating
}
controllerutil.RemoveFinalizer(agentTool, agentToolFinalizer)  // unreachable on error
```

**`instruction_controller.go:75-82`:** Same pattern — ConfigMap deletion failure blocks finalizer.

**`agentworkflow_controller.go:120`:** Same pattern — Argo Workflow deletion failure blocks finalizer.

If the dependent resource (ConfigMap or Argo Workflow) is in a broken state or the API server is temporarily unreachable, the CRD resource gets **permanently stuck in Terminating** state. The fix is to distinguish "resource already gone" (proceed with finalizer removal) from "transient API failure" (retry).

**Severity:** High — resources stuck in Terminating state requiring manual intervention.

### 2g. Agent controller masks reconciliation errors with status update errors

**File:** `internal/controller/agent_controller.go:80-92`

```go
result := r.AppService.Reconcile(ctx, agent)

if err := r.Status().Update(ctx, agent); err != nil {
    logger.Error(err, "Failed to update Agent status")
    return ctrl.Result{}, err  // Returns status update error
}

if result.Error != nil {
    return ctrl.Result{}, result.Error  // Never reached if status update failed
}
```

If both `r.Status().Update()` fails AND `result.Error` is non-nil, only the status update error surfaces — the actual reconciliation error is lost. This makes debugging harder because the logged error doesn't reflect the root cause.

**Severity:** Medium — root cause errors masked during debugging.

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

### 4b. Watcher mappers list ALL namespaces without filtering

**File:** `internal/controller/agent_watchers.go:91,110`

The `findAgentsForModelProvider` mapper lists all Models across all namespaces:

```go
modelList := &agentv1alpha1.ModelList{}
if err := r.List(ctx, modelList); err != nil {  // ALL namespaces
    return nil
}
```

Then lists all Agents across all namespaces:

```go
agentList := &agentv1alpha1.AgentList{}
if err := r.List(ctx, agentList); err != nil {  // ALL namespaces
    return nil
}
```

In a cluster with thousands of agents/models, a single ModelProvider update triggers two unfiltered list calls that scan every resource. This causes latency spikes on every provider change.

**Severity:** Medium — performance degradation at scale.

### 4c. AgentWorkflow ObservedGeneration only set during Compiling phase

**File:** `internal/controller/agentworkflow_controller.go:171`

```go
awf.Status.Phase = agentv1alpha1.WorkflowPhaseCompiling
awf.Status.ObservedGeneration = awf.Generation  // Only set here
```

ObservedGeneration is only updated when entering `Compiling` phase. During the monitoring phase (Running/Succeeded/Failed), if the spec changes, ObservedGeneration stays stale. Users can't tell whether the current status reflects the latest spec or an older generation.

**Severity:** Medium — stale ObservedGeneration misleads status consumers.

### 4d. No timeout for stuck AgentWorkflow monitoring

**File:** `internal/controller/agentworkflow_controller.go:303-304`

```go
if awf.Status.Phase == agentv1alpha1.WorkflowPhaseRunning {
    return ctrl.Result{RequeueAfter: workflowPollInterval}, nil  // 15s forever
}
```

A workflow stuck in `Running` state (e.g., Argo controller is down, or the workflow itself hangs) causes **indefinite 15-second requeue cycles** with no timeout. Over hours/days, hundreds of stuck workflows accumulate and consume the entire controller's reconcile capacity.

**Severity:** Medium — resource exhaustion from stuck workflows.

### 4e. No rate limiting / max concurrent reconciles configured

**File:** `internal/controller/agent_controller.go` (SetupWithManager)

The controller setup doesn't configure `MaxConcurrentReconciles`. With the default of 1, a single slow reconcile (e.g., waiting for a network call in model resolution) blocks all other Agent reconciliations. But if set to >1 without proper handling, concurrent reconciles on the same Agent can cause conflicts.

**Severity:** Medium — single reconcile bottleneck or unhandled concurrency.

### 4f. Cross-namespace references without RBAC enforcement

The Agent CRD allows cross-namespace references for Models (`model.namespace`) and Tools. However:
1. No RBAC check verifies the Agent's namespace has permission to reference resources in other namespaces
2. No network policy or admission control restricts cross-namespace access
3. The operator's service account needs cluster-wide read permissions for Models/Providers/Instructions, which is a broad privilege

**Severity:** Medium — security gap for multi-tenant clusters.

### 4g. No leader election guard in gRPC server

The gRPC server and HTTP gateway start regardless of leader election status. If running multiple operator replicas:
1. All replicas serve gRPC/HTTP (could be intentional for read-only API)
2. But there's no differentiation between leader and non-leader for write operations
3. The server uses `client.Client` directly, which goes through the controller-runtime cache — if the cache isn't started (non-leader), reads may fail

**Severity:** Medium — potential stale reads or errors from non-leader replicas.

### 4h. Webhooks missing cross-resource reference validation

**Files:** `api/v1alpha1/agent_webhook.go`, `api/v1alpha1/model_webhook.go`

All webhooks validate structural correctness (exactly-one-of checks, required fields) but **none** validate that referenced resources actually exist:

- **Agent webhook** (lines 120-142): validates tools have one of `template`/`toolRef`, but doesn't check `toolRef.Name` points to an existing AgentTool
- **Agent webhook** (lines 105-118): validates instruction has one of `template`/`instructionRef`, but doesn't check `instructionRef.Name` points to an existing Instruction
- **Model webhook** (lines 64-112): validates anthropic thinking parameters, but doesn't check `providerRef` points to an existing ModelProvider

This means all cross-resource reference errors are only caught during reconciliation, not at admission time. Users get no `kubectl apply` feedback when referencing non-existent resources.

**Severity:** High — broken references accepted at admission, fail silently during reconciliation.

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

### 6a. Server deployment uses controller's ServiceAccount (privilege escalation)

**File:** `charts/flokoa/templates/server/deployment.yaml:36`

```yaml
serviceAccountName: {{ include "flokoa.controller.serviceAccountName" . }}
```

The gRPC server deployment uses the **controller's** service account instead of its own. The server only needs read-only access to CRDs, but inherits the controller's full RBAC (create, update, delete on deployments, services, configmaps, etc.). If the server is compromised via the gRPC/HTTP endpoint, the attacker gets controller-level write access.

**Severity:** High — violates least privilege; server compromise = full cluster write access.

### 6b. A2A plugin auth silently disabled if token file missing

**File:** `plugins/a2a/main.go:43-49,128-134`

```go
token, err := os.ReadFile(tokenPath)
if err != nil {
    log.Printf("Warning: failed to read Argo token from %s: %v", tokenPath, err)
    // Continues with empty argoToken!
}

// Later in auth check:
if argoToken != "" {
    // Auth check only runs if token was loaded
}
```

If the Argo-injected token file is missing or unreadable, the plugin logs a warning and **silently disables authentication**. Any HTTP client can then execute arbitrary A2A tasks. This should be a fatal error when running in an Argo context.

**Severity:** High — security: unauthenticated task execution.

### 6c. HTTP server missing ReadHeaderTimeout (slowloris risk)

**File:** `internal/server/server.go:269-272`

```go
s.httpServer = &http.Server{
    Addr:    fmt.Sprintf(":%d", s.httpPort),
    Handler: c.Handler(mux),
    // No ReadHeaderTimeout, ReadTimeout, or WriteTimeout
}
```

The HTTP gateway has no timeouts configured. A slow client (or slowloris attack) can hold connections open indefinitely, exhausting server resources.

**Severity:** Medium — DoS risk on the HTTP gateway.

### 6d. gRPC error mapping leaks internal Kubernetes details

**File:** `internal/server/errors.go:33`

```go
return status.Errorf(codes.Internal, "internal error: %s", err.Error())
```

The default case in `mapKubernetesError` returns the full Kubernetes error string to gRPC clients. This can expose internal API paths, resource versions, field names, and cluster structure.

**Severity:** Medium — information disclosure to API clients.

### 6e. No PodDisruptionBudget for the operator

If the operator pod is evicted during a node drain, all reconciliation stops until it's rescheduled. A PDB would ensure at least one replica remains during voluntary disruptions.

### 6f. CRDs in Helm chart but no upgrade strategy

CRDs bundled in Helm charts are notoriously problematic for upgrades — `helm upgrade` doesn't update CRDs. The project has CRDs in both `config/crd/` (kustomize) and `charts/flokoa/crds/`, but there's no documented upgrade path.

### 6g. ConfigMap update drops annotations

**File:** `internal/infra/repo/configmap.go:39-40`

```go
existing.Data = desired.Data
existing.Labels = desired.Labels
// Annotations are NOT preserved
```

`EnsureConfigMap` overwrites Data and Labels but drops all existing annotations. Any annotations set by other controllers, backup tools (Velero), or monitoring (Prometheus) are silently lost on every reconciliation.

**Severity:** Medium — silent metadata loss.

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

### 7c. Missing nil check on SendMessage result before type assertion

**File:** `plugins/a2a/plugin/plugin.go:274-292`

```go
switch r := result.(type) {
case *a2a.Task:
    taskID = r.ID       // panics if r is nil (typed nil interface)
    contextID = r.ContextID
    if r.Status.State.Terminal() {  // panic chain
```

If `a2aClient.SendMessage` returns a typed nil (e.g., `(*a2a.Task)(nil)`), the type switch matches `*a2a.Task` but dereferencing `r.ID` panics. A nil check after the switch case is needed.

**Severity:** Medium — panic on malformed A2A response.

### 7d. `endpointCandidates` heuristic is fragile

The function tries both `endpoint` and `endpoint/a2a` (or strips `/a2a`). This heuristic-based endpoint discovery is fragile and should be replaced with proper service discovery from the Agent CR's status URL.

### 7e. Plugin JSON marshaling error silently discarded

**File:** `plugins/a2a/plugin/plugin.go:442`

```go
taskJSON, _ := json.Marshal(task)
```

If marshaling fails, `taskJSON` is nil and the Argo output parameter `taskResponse` becomes an empty string. The workflow's downstream steps that depend on parsing `taskResponse` will fail with confusing errors instead of a clear marshaling failure.

**Severity:** Low — incomplete workflow outputs.

---

## Prioritized Remediation Plan

### P0 — Fix Before Next Deploy (prevents panics, security holes, and data loss)
1. **Add nil check for `Runtime.Standard` in `ValidateSpec`** — prevents nil deref panics
2. **Fix A2A plugin to requeue on transient poll failures** instead of failing permanently
3. **Fix A2A plugin auth to fatal on missing token** — prevents unauthenticated task execution
4. **Fix server deployment to use its own ServiceAccount** — prevents privilege escalation
5. **Fix finalizer cleanup in 3 controllers** — don't block finalizer removal on cleanup failures
6. **Replace all 7 `_ = r.Status().Update()` calls** with proper error handling
7. **Add checked type assertions** to all 6 watcher mapper functions
8. **Fix tool reconciler to return error when ConfigMap missing** (not silently skip)
9. **Add ModelProvider type validation** — reject providers with no provider-specific field
10. **Add nil check on A2A SendMessage result** before field dereference

### P1 — Fix This Sprint (correctness, testing, observability)
11. **Add admission webhook for Agent CRD** with validation + defaulting
12. **Add cross-resource reference validation to webhooks** (tool, instruction, provider refs)
13. **Add conflict retry logic** for status updates (optimistic concurrency)
14. **Add structured error types** (transient vs. permanent) to stop infinite requeues
15. **Persist A2A plugin task state** (to ConfigMap or CRD status) to survive restarts
16. **Add integration tests for cross-controller flows** (Agent → AgentTool → ConfigMap)
17. **Add negative/error path tests** for all controllers
18. **Fix agent controller error masking** — preserve both status update and reconciliation errors
19. **Add workflow monitoring timeout** — fail or alert after configurable duration
20. **Update ObservedGeneration on all status paths** in AgentWorkflow controller
21. **Add ReadHeaderTimeout to HTTP server** — prevent slowloris
22. **Sanitize gRPC error messages** — don't leak internal Kubernetes details
23. **Fix ConfigMap repo to preserve annotations** on update

### P2 — Fix This Quarter (hardening, scalability, operational readiness)
24. Add Prometheus metrics for reconciliation
25. Add PodDisruptionBudget to Helm chart
26. Configure MaxConcurrentReconciles with conflict handling
27. Add cross-namespace RBAC validation
28. Add CRD upgrade strategy documentation
29. Replace `reconcileAgent` test helper with explicit multi-step reconciliation
30. Add namespace filtering to watcher list operations for performance at scale
31. Add server NetworkPolicy to Helm chart
32. Add audit logging for CUD operations in gRPC server
