# CLAUDE.md - Flokoa Kubernetes Operator

This document provides guidance for AI assistants working with the Flokoa Operator.

## Overview

The Flokoa Operator is a Kubernetes Operator for managing AI Agents through Custom Resource Definitions (CRDs).

- **API Group**: `agent.flokoa.ai`
- **Language**: Go 1.24.0

## Tech Stack

| Component | Version |
|-----------|---------|
| Go | 1.24.10 |
| Kubebuilder | v4 layout |
| Operator SDK | v1.42.0 |
| controller-runtime | v0.21.0 |
| Kubernetes API | v0.33.1 |
| Argo Workflows | 3.7.9 |
| gRPC | v1.72.2 |
| grpc-gateway | v2.26.3 |
| OpenTelemetry | v1.36.0 |

**Testing**: Ginkgo v2 + Gomega (BDD-style testing)
**Linting**: golangci-lint v2.1.0
**Observability**: Prometheus, OpenTelemetry

## Directory Structure

```
operator/
├── api/v1alpha1/                  # CRD type definitions
│   ├── agent_types.go             # Agent CRD schema
│   ├── agenttool_types.go         # AgentTool CRD schema
│   ├── agentworkflow_types.go     # AgentWorkflow CRD schema
│   ├── model_types.go             # Model CRD schema (multi-provider)
│   ├── modelprovider_types.go     # ModelProvider CRD schema
│   ├── instruction_types.go       # Instruction CRD schema
│   ├── groupversion_info.go       # API group registration
│   ├── *_webhook.go               # Webhook definitions
│   └── zz_generated.deepcopy.go   # Generated (DO NOT EDIT)
├── cmd/
│   ├── main.go                    # Operator entrypoint
│   └── server/main.go             # gRPC server entrypoint
├── internal/
│   ├── controller/                # Reconciliation logic
│   │   ├── agent_controller.go    # Agent reconciler (primary)
│   │   ├── agenttool_controller.go
│   │   ├── agentworkflow_controller.go
│   │   ├── agentworkflow_compiler.go  # Argo Workflow compilation
│   │   ├── instruction_controller.go
│   │   ├── model_controller.go
│   │   ├── modelprovider_controller.go
│   │   ├── provider_openai.go     # Provider implementations
│   │   ├── provider_anthropic.go
│   │   ├── provider_google.go
│   │   ├── provider_bedrock.go
│   │   └── *_test.go              # Tests
│   ├── app/agent/                 # Application layer
│   │   ├── reconcile.go           # Orchestration: compile → spec ConfigMap → Deployment/Services
│   │   └── compiler/              # The spec compiler (resolve → merge → inject → validate → emit)
│   ├── spec/                      # Embedded AgentSpec JSON Schemas + validator + secret-env rule
│   ├── infra/                     # Infrastructure layer
│   │   ├── builder/               # Kubernetes resource construction (Deployment, Service)
│   │   ├── repo/                  # Data access layer (interfaces, CRUD for K8s resources)
│   │   └── fakes/                 # Test doubles
│   ├── server/                    # gRPC service implementations
│   │   ├── server.go              # Server initialization
│   │   ├── agent_service.go       # Agent CRUD service
│   │   ├── auth.go                # OIDC authentication
│   │   ├── interceptors.go        # gRPC interceptors
│   │   └── *_service.go           # Other resource services
│   ├── telemetry/                 # OpenTelemetry integration
│   └── webhook/v1alpha1/          # Admission webhooks
├── server/                        # gRPC/Protobuf definitions
│   ├── proto/                     # .proto files (buf-managed)
│   ├── gen/                       # Generated Go code
│   ├── buf.yaml                   # Buf configuration
│   └── Dockerfile                 # gRPC server image
├── plugins/
│   └── a2a/                       # Argo A2A executor plugin
│       ├── main.go                # Plugin HTTP server
│       ├── plugin/                # Plugin logic
│       │   ├── plugin.go          # ExecuteTemplate handler
│       │   ├── resolver.go        # Agent endpoint resolution
│       │   └── types.go           # A2ASpec, ProgressState
│       ├── config/plugin.yaml     # ExecutorPlugin CR definition
│       └── Dockerfile             # Plugin image build
├── charts/flokoa/                 # Helm chart
│   ├── Chart.yaml                 # Chart metadata (appVersion 0.0.7)
│   ├── values.yaml                # Configuration values
│   └── templates/                 # 40+ templates (controller, server, A2A, RBAC, Dex)
├── config/                        # Kustomize manifests
│   ├── crd/bases/                 # Generated CRD YAML files
│   ├── rbac/                      # RBAC roles (admin, editor, viewer per CRD)
│   ├── manager/                   # Operator deployment
│   ├── server/                    # gRPC server deployment
│   ├── samples/                   # Example CRs (16 samples)
│   └── schemas/                   # JSON schemas
├── test/
│   ├── e2e/                       # End-to-end tests (Kind cluster; needs Docker)
│   │   ├── testdata/              # Test fixtures (CRs + Argo workflows) — shared with test/integration
│   │   └── kind-config.yaml       # Kind cluster configuration
│   ├── integration/               # Docker-free e2e: envtest + manager + runner subprocess over A2A
│   └── utils/                     # Test utilities
├── hack/                          # Build scripts (boilerplate template)
├── Makefile                       # Build targets
├── Dockerfile                     # Multi-stage build (distroless)
├── .golangci.yml                  # Linter configuration
└── go.mod                         # Go 1.24.10 dependencies
```

## Core CRDs

The operator manages seven CRDs under `agent.flokoa.ai/v1alpha1`:

### Agent
The **composition root**: an inline pydantic-ai AgentSpec fragment plus `modelRef`, `instructionRefs`, `tools` (AgentTool refs), and `secretRefs` compile into one resolved AgentSpec (see `internal/app/agent/compiler`), validated against the runner's pinned AgentSpec JSON Schema (`internal/spec`) and delivered as the `<agent>-agent-spec` ConfigMap. `runtime` holds image/runnerVersion/isolation + pod-level overrides; `card` is the A2A metadata.

### AgentTool
A declarative MCP endpoint: `url` or `serviceRef`+`path`, `transport`, static `headers` plus secret-backed `headerSecrets` (delivered as `${secret:…}` placeholders), `toolPrefix`, `allowedTools`, `timeoutSeconds`. Compiles to an MCP capability entry. The `openapi` type is retired (webhook rejects it with a migration pointer).

### Model
Named, shareable model configuration: `model` identifier + `providerRef` + typed `settings` (maxTokens, temperature, topP, … with an `extra` passthrough for provider-specific knobs). Compiles to AgentSpec `model` + `model_settings`; rotating a Model recompiles every referencing Agent.

### ModelProvider
Provider connection config. Supports OpenAI, Anthropic, Google, Bedrock with API key secret references, custom base URLs, and TLS configuration.

### Instruction
System prompt management. Content field holds the prompt text; controller creates a ConfigMap.

### AgentWorkflow
**Frozen** (template-only, per the v2.1 pivot): static A2A composition between deployed Agents, compiled to Argo WorkflowTemplates. Supports tasks with agent references, parameters, conditions, and dependencies. The `agentTask` task type is unsupported — the admission webhook rejects new usage. No new features until SwarmRun ships (see `docs/roadmap/`).

### AgentTrigger
Event-driven agent invocation built on Argo Events. References an EventSource/EventBus, compiles to a Sensor that POSTs matching events to flokoa-server's invoke endpoint, with filters, rate/budget limits, session-key extraction, and A2A push-notification delivery. See `docs/agenttrigger.md`.

## Development Commands

All commands run from the `operator/` directory:

### Code Generation (IMPORTANT - run after any type changes)
```bash
make generate           # Generate DeepCopy methods from api/v1alpha1/*_types.go
make manifests          # Generate CRDs, RBAC, webhooks from kubebuilder markers
make generate-python-models  # Generate Pydantic v2 models for Python SDK from CRD schemas
make buf-generate       # Generate gRPC code from proto files
```

**After modifying any `*_types.go` file, always run `make manifests generate`.**

### Build & Run
```bash
make build              # Build manager and server binaries (runs manifests, generate, buf-generate, fmt, vet)
make run                # Run controller locally
make docker-build       # Build Docker images (operator, server, A2A plugin)
make docker-push        # Push all images to ghcr.io
make docker-build-plugins  # Build plugin images only
make docker-push-plugins   # Push plugin images only
make docker-build-flokoa-cli  # Build Python SDK CLI image
make docker-push-flokoa-cli   # Push CLI image
```

### Testing
```bash
make test               # Run unit tests with envtest (runs manifests, generate, fmt, vet first)
make test-integration   # Docker-free integration suite: envtest + real manager + real Python
                        # runner subprocess answering A2A (requires sdk/python/.venv via
                        # `uv sync --all-packages`; shares fixtures with the Kind e2e suite)
make test-e2e           # Run e2e tests in Kind cluster
make setup-test-e2e     # Create Kind cluster if not exists
make cleanup-test-e2e   # Delete Kind cluster
make deploy-e2e-testdata  # Deploy test fixtures (requires OPENAI_API_KEY)
```

### Linting
```bash
make lint               # Run golangci-lint
make lint-fix           # Auto-fix lint issues
make lint-config        # Verify lint configuration
```

### Deployment
```bash
make install            # Install CRDs to cluster
make deploy IMG=<tag>   # Deploy operator to cluster
make uninstall          # Remove CRDs
make undeploy           # Remove operator
make deploy-full        # Deploy everything (operator + Argo Workflows + plugins)
make undeploy-full      # Remove everything
```

### Argo Workflows
```bash
make deploy-argo-workflows     # Deploy Argo Workflows with executor plugins enabled
make undeploy-argo-workflows   # Remove Argo Workflows
make deploy-executor-plugins   # Build and deploy A2A executor plugin
make undeploy-executor-plugins # Remove executor plugins
```

### Docker Images

| Image | Make Target | Registry |
|-------|------------|----------|
| Operator | `docker-build` / `docker-push` | `ghcr.io/danielnyari/flokoa-operator` |
| Server | `docker-build` / `docker-push` | `ghcr.io/danielnyari/flokoa-server` |
| A2A Plugin | `docker-build-plugins` / `docker-push-plugins` | `ghcr.io/danielnyari/flokoa-a2a-plugin` |
| Generic runner | `make docker-build-runner` (in `sdk/python/`) | `ghcr.io/danielnyari/flokoa-runner` |

Version is controlled by `VERSION` in the Makefile (currently `0.0.6`).

## Testing Conventions

### Unit Tests (Ginkgo + Gomega)
- Located alongside code in `*_test.go` files
- Use `envtest` for embedded Kubernetes API server
- Run with: `make test`

```go
var _ = Describe("Agent Controller", func() {
    Context("When reconciling", func() {
        It("should do something", func() {
            Expect(result).To(BeNil())
        })
    })
})
```

### Integration Tests (Docker-free)
- Located in `test/integration/`; run with `make test-integration`
- A real API server (envtest) + the real controller manager (real watches) +
  the shared `test/e2e/testdata` fixtures + the real `flokoa-runner` as a
  subprocess (pydantic-ai `test` model) answering live A2A `message/send`
- Covers compile→ConfigMap→Deployment/Services→status, fleet recompiles on
  Instruction edits, secret-rotation rollouts, last-good-spec semantics, and
  the operator↔runner skew/digest chain
- Does NOT cover: image builds/pulls, kubelet/pod scheduling, in-cluster
  webhooks, Argo Workflows — that's what the Kind suite is for

### E2E Tests (Kind)
- Located in `test/e2e/`
- Use Kind cluster for isolation
- Run with: `make test-e2e`
- Skip CertManager with: `CERT_MANAGER_INSTALL_SKIP=true make test-e2e`

## Code Conventions

### Kubebuilder Markers
Use markers for code generation:

```go
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:validation:Enum=redis;s3;postgres;memory
// +kubebuilder:default=0
// +optional
```

### Error Handling
- Return explicit errors, never panic
- Use structured logging via controller-runtime's logf

```go
log := logf.FromContext(ctx)
log.Error(err, "Failed to reconcile", "agent", req.NamespacedName)
```

### Imports
Standard Go import grouping:
1. Standard library
2. External packages
3. Internal packages

### Generated Code
- Files matching `zz_generated.*.go` are auto-generated
- Never edit these files manually
- Regenerate with `make generate`

## Argo Workflows Executor Plugins

The operator includes an A2A (Agent-to-Agent) executor plugin for Argo Workflows. This allows Argo workflows to call Flokoa agents.

### Architecture
- **Plugin type**: Sidecar executor plugin (HTTP server on port 4355)
- **API endpoint**: `POST /api/v1/template.execute`
- **Auth**: Bearer token from `/var/run/argo/token`
- **Protocol**: A2A (Agent-to-Agent) via `a2a-go` client library

### How It Works
1. Argo injects the plugin as a sidecar container into workflow pods
2. When a template has a `plugin.a2a` spec, Argo calls the plugin's HTTP API
3. The plugin resolves the agent endpoint (via Agent CR or convention-based naming)
4. Sends an A2A message and polls for task completion with requeue
5. Returns outputs: `result` (text) and `taskResponse` (full JSON)

### Plugin Spec (A2ASpec)
```yaml
plugin:
  a2a:
    agent: my-agent         # Agent CR name (required)
    namespace: default      # Agent namespace (optional, defaults to workflow namespace)
    message: "Do something" # Message to send (required)
    timeout: 5m             # Task timeout (optional, default 5m)
```

### Workflow Examples

Simple workflow:
```yaml
apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  generateName: a2a-test-
spec:
  entrypoint: call-agent
  serviceAccountName: flokoa-workflow
  automountServiceAccountToken: true
  templates:
    - name: call-agent
      plugin:
        a2a:
          agent: my-agent
          message: "What are the benefits of Kubernetes?"
          timeout: 2m
```

Parameterized WorkflowTemplate:
```yaml
apiVersion: argoproj.io/v1alpha1
kind: WorkflowTemplate
metadata:
  name: a2a-agent-template
spec:
  entrypoint: call-agent
  serviceAccountName: flokoa-workflow
  automountServiceAccountToken: true
  arguments:
    parameters:
      - name: agent-name
      - name: agent-namespace
      - name: message
      - name: timeout
        value: "2m"
  templates:
    - name: call-agent
      plugin:
        a2a:
          agent: "{{workflow.parameters.agent-name}}"
          namespace: "{{workflow.parameters.agent-namespace}}"
          message: "{{workflow.parameters.message}}"
          timeout: "{{workflow.parameters.timeout}}"
```

### Writing a New Executor Plugin

To create a new Argo executor plugin for Flokoa:

1. **Create plugin directory**: `plugins/<name>/`
2. **Implement HTTP server** listening on port 4355:
   - `POST /api/v1/template.execute` - Main execution endpoint
   - `GET /healthz` - Health check
3. **Handle authorization**: Read token from `/var/run/argo/token`, validate `Authorization: Bearer <token>` header
4. **Define plugin spec type** (the YAML under `plugin.<name>` in workflow templates)
5. **Return proper responses**:
   - Empty `{}` for templates this plugin doesn't handle
   - `ExecuteTemplateReply` with `Node.Phase` and optional `Node.Outputs`
   - Use `Requeue` with a `metav1.Duration` for long-running tasks
6. **Create `config/plugin.yaml`** (ExecutorPlugin CR):
   ```yaml
   apiVersion: argoproj.io/v1alpha1
   kind: ExecutorPlugin
   metadata:
     name: <plugin-name>
   spec:
     sidecar:
       automountServiceAccountToken: true
       container:
         name: <plugin-name>-executor-plugin
         image: ghcr.io/danielnyari/flokoa-<plugin-name>-plugin:latest
         command: ["/plugin-binary"]
         ports:
           - containerPort: 4355
         resources:
           requests: { memory: "64Mi", cpu: "100m" }
           limits: { memory: "128Mi", cpu: "500m" }
         securityContext:
           runAsNonRoot: false
           allowPrivilegeEscalation: false
           capabilities: { drop: [ALL] }
           readOnlyRootFilesystem: true
   ```
7. **Build and deploy**:
   ```bash
   cd plugins/<name>/config && argo executor-plugin build .
   kubectl -n argo apply -f <name>-executor-plugin-configmap.yaml
   ```

### Plugin Response Lifecycle
- **Synchronous**: Return `NodeSucceeded`/`NodeFailed` immediately
- **Asynchronous**: Return `NodeRunning` with `Requeue` duration; Argo calls back after the interval. Track state via in-memory map keyed by workflow UID + template name.

## Python Model Generation

The operator generates Pydantic v2 models from CRD schemas for the Python SDK:

```bash
make generate-python-models
```

This extracts JSON schemas from generated CRDs and uses `datamodel-codegen` to produce:
- `sdk/python/flokoa-types/src/flokoa_types/agenttool.py` - AgentToolSpec (MCP endpoint shape)
- `sdk/python/flokoa-types/src/flokoa_types/agentcard.py` - AgentCard
- `sdk/python/flokoa-types/src/flokoa_types/modelsettings.py` - ModelSettings
- `sdk/python/flokoa-types/src/flokoa_types/agentworkflow.py` - AgentWorkflow

**Prerequisite**: Requires `yq` installed on the system. The target automatically creates a Python venv with `datamodel-code-generator`.

## Linting Rules

Key enabled linters (`.golangci.yml`):
- `errcheck` - Error handling
- `govet` - Go vet checks
- `staticcheck` - Static analysis
- `ginkgolinter` - Ginkgo test conventions
- `revive` - Import shadowing, comment spacing
- `gocyclo` - Cyclomatic complexity
- `misspell` - Spelling errors
- `dupl` - Code duplication detection
- `goconst` - Repeated strings that could be constants
- `ineffassign` - Ineffectual assignments
- `lll` - Line length limits (excluded for api/ and internal/)
- `prealloc` - Slice preallocation hints
- `unconvert` - Unnecessary type conversions
- `unparam` - Unused function parameters
- `unused` - Unused code detection
- `copyloopvar` - Loop variable copy issues
- `nakedret` - Naked returns in long functions

Formatters: `gofmt`, `goimports`

Run `make lint` before committing.

## CI/CD Pipelines

GitHub Actions workflows in `.github/workflows/`:

| Workflow | Trigger | Actions |
|----------|---------|---------|
| `test.yml` | Push/PR | `go mod tidy`, `make test`; `make test-integration` (uv-synced runner leg); Docker build+push on main |
| `lint.yml` | Push/PR | golangci-lint checks |
| `test-e2e.yml` | Push/PR | Kind cluster + e2e tests |

## Working with the Codebase

### Adding a New Field to an Existing CRD
1. Edit the appropriate `api/v1alpha1/*_types.go` file
2. Add kubebuilder markers for validation
3. Run `make manifests generate`
4. Update controller logic in `internal/controller/`
5. Run `make generate-python-models` if the change affects Python SDK types
6. Add tests
7. Run `make test`

### Adding a New CRD
1. Create `api/v1alpha1/<name>_types.go` with type definitions
2. Add kubebuilder markers (rbac, validation, subresource, printcolumn)
3. Register types in `groupversion_info.go`
4. Run `make manifests generate`
5. Create controller in `internal/controller/`
6. Add RBAC markers to the controller
7. Register controller in `cmd/main.go`
8. Add sample CR in `config/samples/`
9. Add tests
10. Run `make generate-python-models` if Python SDK needs the new types

### Implementing Controller Logic
The reconciler pattern:

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

### Running Locally Against a Cluster
```bash
# 1. Ensure kubectl points to your cluster
kubectl cluster-info

# 2. Install CRDs
make install

# 3. Run operator locally
make run

# 4. In another terminal, create an Agent
kubectl apply -f config/samples/agent_v1alpha1_agent.yaml
```

## Docker Image

Multi-stage build using distroless base:
- Builder: `golang:1.24`
- Runtime: `gcr.io/distroless/static:nonroot`
- Runs as non-root user (65532:65532)
- Platforms: linux/amd64, linux/arm64, linux/s390x, linux/ppc64le

## RBAC

The operator requires these permissions:
- Full CRUD on `agents` resource
- Update on `agents/status`
- Update on `agents/finalizers`

Additional roles in `config/rbac/`:
- `agent_admin_role.yaml` - Admin access
- `agent_editor_role.yaml` - Edit access
- `agent_viewer_role.yaml` - View-only access

## Common Issues

### "CRD not found" errors
Run `make install` to install CRDs to your cluster.

### Envtest failures
Run `make setup-envtest` to download required binaries.

### Lint failures on generated code
Generated code is excluded from linting. If you see lint errors in `zz_generated.*` files, regenerate with `make generate`.

### Python model generation fails
Ensure `yq` is installed (`brew install yq` on macOS). The Makefile creates a Python venv automatically.

## Architecture Notes

### Layered Architecture
The operator follows a layered architecture pattern:
- **API layer** (`api/v1alpha1/`) - CRD type definitions with kubebuilder markers
- **Controller layer** (`internal/controller/`) - Kubernetes reconciliation loops
- **Domain layer** (`internal/app/agent/`) - Business logic for agent reconciliation (tools, instructions, models)
- **Infrastructure layer** (`internal/infra/`) - Kubernetes resource builders and repository pattern for CRUD operations
- **Server layer** (`internal/server/`) - gRPC API with OIDC auth

### AgentWorkflow Compilation
AgentWorkflow CRDs are compiled into Argo Workflow resources by `agentworkflow_compiler.go`. The compiler translates high-level task definitions into Argo DAG templates that call agents via the A2A executor plugin.

### Provider Implementations
Each LLM provider (OpenAI, Anthropic, Google, Bedrock) has a handler in `internal/domain/model/` deriving the pydantic-ai model prefix and the env projection (API-key secret refs, base URLs) for runner pods.

### The Runtime Contract
`docs/reference/runtime-contract.md` is normative for everything operator↔runner: the compiled-spec ConfigMap, `${secret:NAME}` ↔ `FLOKOA_SECRET_*` projection, skew detection, capability wheelhouses, and platform-injected capabilities. Changes to it are PR-blocking review items; regenerate artifacts with `make runner-contract` in `sdk/python/`.
