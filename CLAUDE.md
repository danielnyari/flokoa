# CLAUDE.md - Flokoa Kubernetes Operator

This document provides guidance for AI assistants working with the Flokoa codebase.

## Project Overview

Flokoa is a **Kubernetes Operator** for managing AI Agents in Kubernetes clusters. It enables declarative deployment and lifecycle management of AI agents through Custom Resource Definitions (CRDs).

- **Domain**: `flokoa.ai`
- **API Group**: `agent.flokoa.ai`
- **Repository**: `github.com/danielnyari/flokoa`
- **License**: Apache 2.0

## Tech Stack

| Component | Version |
|-----------|---------|
| Go | 1.24.0 |
| Kubebuilder | v4 layout |
| Operator SDK | v1.42.0 |
| controller-runtime | v0.21.0 |
| Kubernetes API | v0.33.0 |

**Testing**: Ginkgo v2 + Gomega (BDD-style testing)
**Linting**: golangci-lint v2.1.0
**Observability**: Prometheus, OpenTelemetry

## Directory Structure

```
flokoa/
└── operator/                      # Main operator code
    ├── api/v1alpha1/              # CRD type definitions
    │   ├── agent_types.go         # Agent CRD schema
    │   ├── groupversion_info.go   # API group registration
    │   └── zz_generated.deepcopy.go  # Generated (DO NOT EDIT)
    ├── cmd/
    │   └── main.go                # Operator entrypoint
    ├── internal/controller/       # Reconciliation logic
    │   ├── agent_controller.go    # Agent reconciler
    │   ├── agent_controller_test.go
    │   └── suite_test.go          # Test suite setup
    ├── config/                    # Kubernetes manifests
    │   ├── crd/                   # CRD definitions
    │   ├── rbac/                  # RBAC roles
    │   ├── manager/               # Operator deployment
    │   ├── samples/               # Example CRs
    │   └── ...
    ├── test/
    │   ├── e2e/                   # End-to-end tests
    │   └── utils/                 # Test utilities
    ├── hack/                      # Build scripts
    ├── .github/workflows/         # CI/CD pipelines
    ├── Makefile                   # Build targets
    ├── Dockerfile                 # Multi-stage build
    └── go.mod                     # Go dependencies
```

## Core API: Agent CRD

The `Agent` resource (`agent.flokoa.ai/v1alpha1`) manages AI agents with these key fields:

### AgentSpec (Desired State)
- **Runtime**: Container image, entrypoint (lambda-style: `module.handler`), framework detection
- **Tools**: References to Tool CRDs for agent capabilities
- **MCPServers**: MCP (Model Context Protocol) server connections
- **Resources**: CPU/memory limits + agent-specific limits (tokens, cost, tool calls, duration)
- **Scaling**: Knative-style autoscaling (min/max replicas, concurrency, scale-to-zero)
- **StateBackend**: Persistence (redis, s3, postgres, memory)
- **HealthCheck**: HTTP health check configuration

### AgentStatus (Observed State)
- **Phase**: Pending, Running, Failed
- **Backend**: Backend implementation in use
- **URL**: Agent invocation endpoint
- **Replicas**: Current/available replica counts
- **DetectedFramework**: Auto-detected framework (pydantic-ai, langchain, crewai, marvin, autogen, custom)

## Development Commands

All commands run from the `operator/` directory:

### Build & Run
```bash
make build              # Build manager binary
make run                # Run controller locally
make docker-build IMG=<tag>  # Build Docker image
```

### Code Generation
```bash
make manifests          # Generate CRDs, RBAC, webhooks
make generate           # Generate DeepCopy methods
make fmt                # Format code
make vet                # Run go vet
```

### Testing
```bash
make test               # Run unit tests with envtest
make test-e2e           # Run e2e tests in Kind cluster
make setup-envtest      # Download envtest binaries
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
```

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

### E2E Tests
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

## Linting Rules

Key enabled linters (`.golangci.yml`):
- `errcheck` - Error handling
- `govet` - Go vet checks
- `staticcheck` - Static analysis
- `ginkgolinter` - Ginkgo test conventions
- `revive` - Import shadowing, comment spacing
- `gocyclo` - Cyclomatic complexity
- `misspell` - Spelling errors

Run `make lint` before committing.

## CI/CD Pipelines

GitHub Actions workflows in `.github/workflows/`:

| Workflow | Trigger | Actions |
|----------|---------|---------|
| `test.yml` | Push/PR | `go mod tidy`, `make test` |
| `lint.yml` | Push/PR | golangci-lint checks |
| `test-e2e.yml` | Push/PR | Kind cluster + e2e tests |

## Working with the Codebase

### Adding a New Field to Agent CRD
1. Edit `api/v1alpha1/agent_types.go`
2. Add kubebuilder markers for validation
3. Run `make manifests generate`
4. Update controller logic in `internal/controller/`
5. Add tests

### Implementing Controller Logic
The reconciler is in `internal/controller/agent_controller.go`:

```go
func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := logf.FromContext(ctx)

    // 1. Fetch the Agent resource
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

## Project Status

This project is in early development (v0.0.1). The Agent CRD schema is defined but controller reconciliation logic contains TODO placeholders for implementation.

Key items to implement:
- [ ] Agent reconciliation logic
- [ ] Backend integration (Knative, native Deployment)
- [ ] State backend connections
- [ ] Tool injection
- [ ] MCP server integration
- [ ] Webhook validation (optional)
