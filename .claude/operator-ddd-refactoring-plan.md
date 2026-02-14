# Refactor Flokoa Operator to Domain-Driven Design

## Context

The Flokoa Kubernetes Operator manages AI agent lifecycle through 5 CRDs. Currently, **all business logic lives in controllers** — most critically the `AgentReconciler` at 1553 lines. This controller mixes domain validation, reference resolution, Kubernetes resource construction, CRUD operations, hash computation, and status management into a single file. The result is code that can only be tested via envtest (slow, heavyweight) and is difficult to reason about or extend.

This plan refactors the operator into a layered DDD architecture using Go-idiomatic patterns: interfaces over inheritance, plain structs as value objects, functions as domain services, and composition over abstraction hierarchies. The goal is testability, clarity of responsibility, and maintainability — not ceremony.

---

## Target Architecture

```
controller  ──>  app  ──>  domain
     │            │           ▲
     │            ▼           │
     └────────> infra ────────┘
```

**Dependency rule**: `domain` never imports `infra`, `app`, or `controller`.

---

## Target Directory Structure

```
operator/internal/
├── domain/                          # Pure logic, no I/O, no client-go calls
│   ├── agent/
│   │   ├── validate.go              # ValidateSpec() — pure function
│   │   ├── phase.go                 # CalculatePhase() — pure function
│   │   ├── labels.go                # Labels() — pure function
│   │   └── status.go                # Condition constants + setCondition helper
│   ├── model/
│   │   ├── validate.go              # ValidateProviderParams() — pure function
│   │   ├── resolved_config.go       # ResolvedModelConfig, ProviderConfig value objects
│   │   ├── provider_handler.go      # ProviderHandler interface + registry
│   │   ├── provider_openai.go       # OpenAI strategy
│   │   ├── provider_anthropic.go    # Anthropic strategy
│   │   ├── provider_google.go       # Google strategy
│   │   └── provider_bedrock.go      # Bedrock strategy
│   ├── modelprovider/
│   │   └── validate.go              # ValidateProvider() — pure function
│   ├── tool/
│   │   └── validate.go              # ValidateSpec() — pure function (no ConfigMap I/O)
│   └── hash/
│       └── hash.go                  # ConfigMapData(), SecretVersions() — pure functions
│
├── infra/                           # K8s API interactions + resource construction
│   ├── repo/
│   │   ├── interfaces.go            # All repository interfaces
│   │   ├── configmap.go             # ConfigMapRepo impl (wraps client.Client)
│   │   ├── deployment.go            # DeploymentRepo impl
│   │   ├── service.go               # ServiceRepo impl
│   │   ├── agent.go                 # AgentReader/StatusWriter impl
│   │   ├── model.go                 # ModelReader impl
│   │   ├── modelprovider.go         # ModelProviderReader impl
│   │   ├── agenttool.go             # AgentToolReader/Writer impl
│   │   ├── instruction.go           # InstructionReader/Writer impl
│   │   ├── secret.go                # SecretReader impl
│   │   ├── owner.go                 # OwnerSetter impl (SetControllerReference wrapper)
│   │   └── fakes/                   # In-memory test doubles
│   │       ├── configmap.go
│   │       ├── deployment.go
│   │       └── ...
│   └── builder/
│       ├── deployment.go            # BuildDeployment(DeploymentParams) — pure function
│       ├── service.go               # BuildService() — pure function
│       └── configmap.go             # ConfigMap construction helpers
│
├── app/                             # Orchestration / use cases
│   ├── agent/
│   │   ├── reconcile.go             # Service.Reconcile() — main orchestrator
│   │   ├── tool_reconciler.go       # Tool resolution sub-use-case
│   │   ├── model_reconciler.go      # Model resolution sub-use-case
│   │   └── instruction_reconciler.go
│   ├── agenttool/
│   │   └── reconcile.go
│   ├── model/
│   │   └── reconcile.go
│   ├── modelprovider/
│   │   └── reconcile.go
│   └── instruction/
│       └── reconcile.go
│
├── controller/                      # Thin adapters (~80-100 lines each)
│   ├── agent_controller.go          # Fetch, finalizer, delegate to app, status update
│   ├── agenttool_controller.go
│   ├── model_controller.go
│   ├── modelprovider_controller.go
│   ├── instruction_controller.go
│   ├── watchers.go                  # All 6 findAgentsFor* mapFuncs
│   └── *_test.go                    # Existing envtest integration tests (retained)
│
├── converter/                       # UNCHANGED
├── config/                          # UNCHANGED
└── server/                          # UNCHANGED
```

---

## Domain Model

### Bounded Contexts

| Context | Aggregates | Responsibility |
|---------|-----------|----------------|
| Agent Management | Agent, AgentTool, Instruction | Agent composition, tool/instruction resolution, deployment orchestration |
| Model Resolution | Model, ModelProvider | Provider validation, parameter alignment, config generation |

### What Goes Where

| Current Location | Logic | Target |
|---|---|---|
| `agent_controller.go:validateAgent()` | Pure spec validation | `domain/agent/validate.go` |
| `agent_controller.go:calculatePhase()` | Phase from deployment status | `domain/agent/phase.go` |
| `agent_controller.go:buildLabels()` | Standard K8s labels | `domain/agent/labels.go` |
| `agent_controller.go:setCondition()` | Condition helper + constants | `domain/agent/status.go` |
| `agent_controller.go:hashConfigMapData()` | Deterministic data hashing | `domain/hash/hash.go` |
| `agent_controller.go:computeSecretRefsHash()` | Pure hash part extracts, I/O part stays in app | `domain/hash/hash.go` (pure) + `app/agent/model_reconciler.go` (I/O) |
| `model_provider.go` (entire file) | ProviderHandler interface, registry, config building | `domain/model/` |
| `provider_*.go` (4 files) | Provider strategy implementations | `domain/model/` |
| `model_controller.go:validateProviderParams()` | Parameter/provider type alignment | `domain/model/validate.go` |
| `modelprovider_controller.go:validateProvider()` | Exactly-one-provider validation | `domain/modelprovider/validate.go` |
| `agenttool_controller.go:validateSpec()` | Spec validation (non-I/O parts) | `domain/tool/validate.go` |
| `agent_controller.go:buildDeployment()` (180 lines) | K8s Deployment construction | `infra/builder/deployment.go` |
| `agent_controller.go:buildService()` | K8s Service construction | `infra/builder/service.go` |
| `agent_controller.go:buildStandardContainerSpec()` | Container spec for standard runtime | `infra/builder/deployment.go` |
| `agent_controller.go:buildManagedContainerSpec()` | Container spec for template runtime | `infra/builder/deployment.go` |
| `agent_controller.go:reconcileDeployment()` | Get-or-create Deployment | `infra/repo/deployment.go` |
| `agent_controller.go:reconcileService()` | Get-or-create Service | `infra/repo/service.go` |
| `agent_controller.go:reconcileAgentCardConfigMap()` | ConfigMap CRUD | `app/agent/reconcile.go` → `infra/repo/configmap.go` |
| `agent_controller.go:reconcileModelConfigMap()` | ConfigMap CRUD | `app/agent/model_reconciler.go` → `infra/repo/configmap.go` |
| `agent_controller.go:reconciletemplateConfigMap()` | ConfigMap CRUD | `app/agent/reconcile.go` → `infra/repo/configmap.go` |
| `agent_controller.go:reconcileTools()` | Tool orchestration (inline + ref) | `app/agent/tool_reconciler.go` |
| `agent_controller.go:reconcileModel()` | Model resolution chain | `app/agent/model_reconciler.go` |
| `agent_controller.go:reconcileInstruction()` | Instruction resolution | `app/agent/instruction_reconciler.go` |
| `agent_controller.go:Reconcile()` | Top-level orchestration | `app/agent/reconcile.go` (Service.Reconcile) |
| `agent_controller.go:findAgentsFor*()` (6 funcs) | Watch mapFuncs | `controller/watchers.go` |

---

## Key Interface Designs

### Repository Interfaces (`infra/repo/interfaces.go`)

```go
type ConfigMapRepo interface {
    GetConfigMap(ctx context.Context, key types.NamespacedName) (*corev1.ConfigMap, error)
    EnsureConfigMap(ctx context.Context, desired *corev1.ConfigMap) error
    DeleteConfigMap(ctx context.Context, key types.NamespacedName) error
}

type DeploymentRepo interface {
    EnsureDeployment(ctx context.Context, desired *appsv1.Deployment) (*appsv1.Deployment, error)
}

type ServiceRepo interface {
    EnsureService(ctx context.Context, desired *corev1.Service) (*corev1.Service, error)
}

type AgentToolReader interface {
    GetAgentTool(ctx context.Context, key types.NamespacedName) (*agentv1alpha1.AgentTool, error)
}

type AgentToolWriter interface {
    EnsureAgentTool(ctx context.Context, desired *agentv1alpha1.AgentTool) error
}

type ModelReader interface {
    GetModel(ctx context.Context, key types.NamespacedName) (*agentv1alpha1.Model, error)
}

type ModelProviderReader interface {
    GetModelProvider(ctx context.Context, key types.NamespacedName) (*agentv1alpha1.ModelProvider, error)
}

type InstructionReader interface {
    GetInstruction(ctx context.Context, key types.NamespacedName) (*agentv1alpha1.Instruction, error)
}

type InstructionWriter interface {
    EnsureInstruction(ctx context.Context, desired *agentv1alpha1.Instruction) error
}

type SecretReader interface {
    GetSecret(ctx context.Context, key types.NamespacedName) (*corev1.Secret, error)
}

type OwnerSetter interface {
    SetOwner(owner, controlled metav1.Object) error
}
```

Each `Ensure*` method performs get-or-create-or-update logic internally, moving that boilerplate out of business logic.

### Builder Value Objects (`infra/builder/deployment.go`)

```go
type DeploymentParams struct {
    AgentName         string
    AgentNamespace    string
    Labels            map[string]string
    Runtime           agentv1alpha1.RuntimeSpec
    ToolConfigMaps    []ToolMount
    AgentCardCM       string
    ModelInfo         *ModelMount
    TemplateCMName    string
    InstructionCMName string
}

type ToolMount struct {
    ToolName      string
    ConfigMapName string
    DataHash      string
}

type ModelMount struct {
    ConfigMapName  string
    EnvVars        []corev1.EnvVar
    SecretEnvVars  []corev1.EnvVar
    SecretRefsHash string
}

func BuildDeployment(params DeploymentParams) *appsv1.Deployment { ... }
func BuildService(name, namespace string, labels map[string]string, runtime agentv1alpha1.RuntimeSpec) *corev1.Service { ... }
```

### Application Service (`app/agent/reconcile.go`)

```go
type Deps struct {
    AgentTools   repo.AgentToolReader
    AgentToolW   repo.AgentToolWriter
    Models       repo.ModelReader
    Providers    repo.ModelProviderReader
    Instructions repo.InstructionReader
    InstructionW repo.InstructionWriter
    ConfigMaps   repo.ConfigMapRepo
    Deployments  repo.DeploymentRepo
    Services     repo.ServiceRepo
    Secrets      repo.SecretReader
    OwnerSetter  repo.OwnerSetter
}

type Service struct { deps Deps; /* sub-reconcilers */ }

func NewService(deps Deps) *Service { ... }

// Reconcile takes an already-fetched Agent, performs all orchestration,
// mutates agent.Status in place, and returns a result.
// The controller is responsible for persisting the status update.
func (s *Service) Reconcile(ctx context.Context, agent *agentv1alpha1.Agent) ReconcileResult { ... }
```

### Thin Controller (`controller/agent_controller.go` — target ~80 lines)

```go
type AgentReconciler struct {
    client.Client
    Scheme     *runtime.Scheme
    AppService *agentapp.Service
}

func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch agent (6 lines)
    // 2. Handle deletion + finalizer (12 lines)
    // 3. Delegate: result := r.AppService.Reconcile(ctx, agent) (1 line)
    // 4. Persist status: r.Status().Update(ctx, agent) (4 lines)
    // 5. Return result (3 lines)
}
```

---

## Migration Steps (Incremental — operator stays functional at each step)

### Step 1: Extract domain hash utilities
- **Create**: `internal/domain/hash/hash.go` + `hash_test.go`
- **Move**: `hashConfigMapData()` → `hash.ConfigMapData()`, pure part of `computeSecretRefsHash()` → `hash.SecretVersions()`
- **Update**: `agent_controller.go` calls `hash.ConfigMapData()` instead of local function
- **Risk**: Minimal — pure function extraction
- **Verify**: `make test` passes

### Step 2: Extract domain validation functions
- **Create**: `internal/domain/agent/validate.go`, `domain/model/validate.go`, `domain/modelprovider/validate.go`, `domain/tool/validate.go` + tests
- **Move**: `validateAgent()` → `agent.ValidateSpec()`, `validateProviderParams()` → `model.ValidateProviderParams()`, `validateProvider()` → `modelprovider.ValidateProvider()`, tool spec validation → `tool.ValidateSpec()`
- **Update**: All 4 controllers call domain validation functions
- **Risk**: Low — pure functions with no I/O
- **Verify**: `make test` passes, add table-driven unit tests for each validator

### Step 3: Extract domain labels and phase
- **Create**: `internal/domain/agent/labels.go`, `domain/agent/phase.go` + tests
- **Move**: `buildLabels()` → `agent.Labels()`, `calculatePhase()` → `agent.CalculatePhase()`
- **Update**: `agent_controller.go` calls domain functions
- **Risk**: Minimal
- **Verify**: `make test` passes

### Step 4: Move provider handlers to domain
- **Move**: `model_provider.go` → `domain/model/resolved_config.go` + `provider_handler.go`
- **Move**: `provider_openai.go`, `provider_anthropic.go`, `provider_google.go`, `provider_bedrock.go` → `domain/model/`
- **Update**: Import paths in `agent_controller.go`
- **Risk**: Low — package move, no logic changes
- **Verify**: `make test` passes

### Step 5: Create repository interfaces and implementations
- **Create**: `internal/infra/repo/interfaces.go` — all interfaces
- **Create**: `internal/infra/repo/configmap.go`, `deployment.go`, `service.go`, `agent.go`, `model.go`, `modelprovider.go`, `agenttool.go`, `instruction.go`, `secret.go`, `owner.go`
- **Create**: `internal/infra/repo/fakes/` — in-memory test doubles
- **No existing files modified** — purely additive
- **Risk**: None — no callers yet
- **Verify**: `make build` compiles, fake tests pass

### Step 6: Extract builders to infrastructure layer
- **Create**: `internal/infra/builder/deployment.go`, `service.go`, `configmap.go` + tests
- **Extract**: `buildDeployment()`, `buildService()`, `buildStandardContainerSpec()`, `buildManagedContainerSpec()`, `getDeploymentOverrides()` → builder package with `DeploymentParams` value object
- **Update**: `agent_controller.go` calls `builder.BuildDeployment(params)` and `builder.BuildService()`
- **Risk**: Medium — the deployment builder is 180+ lines of volume/mount/env wiring. Existing envtest tests are the safety net.
- **Verify**: `make test` passes, add unit tests for builder covering standard, template, with-model, with-tools, with-instruction scenarios

### Step 7: Create application services and wire thin controllers
- **Create**: `internal/app/agent/reconcile.go`, `tool_reconciler.go`, `model_reconciler.go`, `instruction_reconciler.go` + tests
- **Create**: `internal/app/agenttool/reconcile.go`, `app/model/reconcile.go`, `app/modelprovider/reconcile.go`, `app/instruction/reconcile.go` + tests
- **Refactor**: Each controller shrinks to ~80-100 lines: fetch → finalizer → delegate to app service → persist status
- **Update**: `cmd/main.go` to construct repo impls, build app services, inject into controllers
- **Risk**: Medium-High — this is the core restructuring. Do all 5 controllers in this step to avoid inconsistent states.
- **Verify**: `make test` passes, `make test-e2e` passes, add unit tests for app services using fakes

### Step 8: Extract watchers and clean up
- **Create**: `internal/controller/watchers.go` — `AgentWatchers` struct with all 6 `FindAgentsFor*` methods
- **Refactor**: `SetupWithManager` uses `AgentWatchers`
- **Clean up**: Remove dead code, empty functions, unused imports
- **Move**: `setCondition` helper + condition constants to `domain/agent/status.go`
- **Risk**: Low — structural reorganization
- **Verify**: `make test` and `make test-e2e` pass

---

## Verification Plan

After each step:
1. `make build` — compiles successfully
2. `make test` — all unit + envtest tests pass
3. `make lint` — no new lint issues

After the full migration:
4. `make test-e2e` — full e2e tests pass in Kind cluster (agent lifecycle, Argo integration)
5. Manual verification: apply a sample Agent CR, confirm Deployment + Service + ConfigMaps are created correctly
6. Verify gRPC server still works (it depends on `internal/converter/` which is unchanged, and `internal/server/` which is unchanged)

---

## Critical Files

| File | Role in Migration |
|---|---|
| `internal/controller/agent_controller.go` | Source of all extracted logic (1553 lines → ~80 lines) |
| `internal/controller/model_provider.go` | Cleanest existing abstraction; moves to `domain/model/` in Step 4 |
| `internal/controller/agent_controller_test.go` | Primary regression safety net (envtest) — must pass at every step |
| `cmd/main.go` | Updated in Step 7 to wire repo impls → app services → controllers |
| `api/v1alpha1/agent_types.go` | CRD types — unchanged but referenced by every layer |

## What Does NOT Change

- `api/v1alpha1/` — CRD types stay exactly where they are
- `internal/converter/` — gRPC converters are unchanged
- `internal/server/` — gRPC service implementations are unchanged
- `internal/config/` — config loading is unchanged
- `server/proto/` — proto definitions are unchanged
- `plugins/a2a/` — Argo plugin is unchanged
- `config/` — K8s manifests, RBAC, CRDs are unchanged
- `charts/flokoa/` — Helm chart is unchanged
