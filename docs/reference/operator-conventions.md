# Operator Code Conventions & Architecture

Reference for working in `operator/`. The day-to-day map is [`operator/CLAUDE.md`](../../operator/CLAUDE.md);
the operating principles are in [`design-docs/core-beliefs.md`](../design-docs/core-beliefs.md);
this page holds the detail.

## Layered architecture (enforced)

The operator follows a layered architecture with a **mechanically enforced** dependency
direction (see [core-beliefs](../design-docs/core-beliefs.md) and the `depguard` rules in
`operator/.golangci.yml`):

| Layer | Package | Role | May depend on |
|-------|---------|------|---------------|
| **API** | `api/v1alpha1/` | CRD type definitions + kubebuilder markers | (nothing internal) |
| **Controller** | `internal/controller/` | Kubernetes reconciliation loops, provider handlers | app, domain, infra, spec |
| **Server** | `internal/server/` | gRPC API + OIDC auth | app, domain, infra |
| **Application** | `internal/app/` | Orchestration: compile → spec ConfigMap → Deployment/Services | domain, infra |
| **Domain** | `internal/domain/` | Pure domain models & functions (leaf) | (nothing internal) |
| **Infrastructure** | `internal/infra/` | Resource builders + repository pattern (K8s CRUD) | domain |

Enforced invariants (depguard, run by `make lint`):

- `api/v1alpha1` must not import any `internal/*` package — keep CRD types importable standalone.
- `internal/domain` is a **leaf**: it must not import `controller`, `server`, `app`, `infra`, or `webhook`.
- `internal/infra` sits below the others: it must not import `controller`, `server`, `app`, or `webhook`.

The depguard `desc` messages double as remediation instructions. To tighten the contract
further, add rules in `operator/.golangci.yml` and verify with `make lint`.

### Provider implementations

Each LLM provider (OpenAI, Anthropic, Google, Bedrock) has a handler in
`internal/domain/model/` that derives the pydantic-ai model prefix and the env projection
(API-key secret refs, base URLs) for runner pods.

### AgentWorkflow compilation

`AgentWorkflow` CRDs are compiled into Argo Workflow resources by
`internal/controller/agentworkflow_compiler.go`, which translates high-level task definitions
into Argo DAG templates that call agents via the [A2A executor plugin](../argo/executor-plugins.md).

### The runtime contract

[`docs/reference/runtime-contract.md`](runtime-contract.md) is **normative** for everything
operator↔runner: the compiled-spec ConfigMap, `${secret:NAME}` ↔ `FLOKOA_SECRET_*` projection,
skew detection, capability wheelhouses, and platform-injected capabilities. Changes to it are
PR-blocking review items; regenerate artifacts with `make runner-contract` in `sdk/python/`.

## Code conventions

### Kubebuilder markers

Use markers for code generation — RBAC, validation, defaults, webhooks — never hand-write the
equivalent:

```go
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:validation:Enum=redis;s3;postgres;memory
// +kubebuilder:default=0
// +optional
```

### Error handling

- Return explicit errors, never panic.
- Use structured logging via controller-runtime's `logf`:

  ```go
  log := logf.FromContext(ctx)
  log.Error(err, "Failed to reconcile", "agent", req.NamespacedName)
  ```

### Imports

Standard Go grouping: (1) standard library, (2) external packages, (3) internal packages.

### Generated code

- Files matching `zz_generated.*.go` are auto-generated — never edit them manually.
- Regenerate with `make generate`. CI enforces freshness via `make verify-codegen`
  (see [core-beliefs](../design-docs/core-beliefs.md) for the full codegen pipeline).

## Controller pattern

The reconciler skeleton:

```go
func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := logf.FromContext(ctx)

    // 1. Fetch the resource
    var agent agentv1alpha1.Agent
    if err := r.Get(ctx, req.NamespacedName, &agent); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. Reconcile desired state
    // ... your logic here

    // 3. Update status
    if err := r.Status().Update(ctx, &agent); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}
```

## Testing tiers

| Tier | Location | Command | What it covers |
|------|----------|---------|----------------|
| **Unit** (Ginkgo + Gomega) | `*_test.go` alongside code | `make test` | Reconcile logic against an embedded API server (`envtest`) |
| **Integration** (Docker-free) | `test/integration/` | `make test-integration` | Real manager + real `flokoa-runner` subprocess answering A2A; compile→ConfigMap→Deployment/Services→status, fleet recompiles, secret-rotation, last-good-spec, operator↔runner skew chain |
| **E2E** (Kind) | `test/e2e/` | `make test-e2e` | Image builds/pulls, pod scheduling, in-cluster webhooks, Argo Workflows |

Unit test shape:

```go
var _ = Describe("Agent Controller", func() {
    Context("When reconciling", func() {
        It("should do something", func() {
            Expect(result).To(BeNil())
        })
    })
})
```

Integration tests need `sdk/python/.venv` (`uv sync --all-packages`) and share the
`test/e2e/testdata` fixtures with the Kind suite. Skip CertManager in e2e with
`CERT_MANAGER_INSTALL_SKIP=true make test-e2e`.

## Linting

`make lint` runs golangci-lint (config: `operator/.golangci.yml`). Enabled linters include
`errcheck`, `govet`, `staticcheck`, `revive`, `gocyclo`, `misspell`, `dupl`, `goconst`,
`ineffassign`, `lll` (excluded for `api/` and `internal/`), `prealloc`, `unconvert`, `unparam`,
`unused`, `copyloopvar`, `nakedret`, `ginkgolinter`, and **`depguard`** (the layer-boundary
rules above). Formatters: `gofmt`, `goimports`. Run `make lint` before committing.

## Common issues

| Symptom | Fix |
|---------|-----|
| "CRD not found" errors | `make install` to install CRDs to your cluster |
| Envtest failures | `make setup-envtest` to download required binaries |
| Lint errors in `zz_generated.*` | Generated code is excluded; regenerate with `make generate` |
| Python model generation fails | Install `yq` (`brew install yq`); the Makefile creates the venv automatically |
| depguard "import not allowed" | You crossed a layer boundary — read the message; invert the dependency |

## Docker image

Multi-stage build on a distroless base: builder `golang:1.24`, runtime
`gcr.io/distroless/static:nonroot`, non-root user `65532:65532`, platforms
`linux/{amd64,arm64,s390x,ppc64le}`.
