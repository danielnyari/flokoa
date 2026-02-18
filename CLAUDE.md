# CLAUDE.md - Flokoa

This document provides guidance for AI assistants working with the Flokoa codebase.

## Project Overview

Flokoa is an open-source platform for managing AI Agents in Kubernetes clusters. It consists of:

1. **Kubernetes Operator** (Go) - Declarative deployment and lifecycle management of AI agents through CRDs, with a gRPC API server, Helm chart, and Argo Workflows executor plugin
2. **Python SDK** - Client library and CLI for building and running agents locally, with framework integrations (pydantic-ai, google-adk) and A2A protocol support

- **Domain**: `flokoa.ai`
- **Repository**: `github.com/danielnyari/flokoa`
- **License**: Apache 2.0
- **API Group**: `agent.flokoa.ai`
- **API Version**: `v1alpha1`

## Monorepo Structure

```
flokoa/
├── CLAUDE.md                      # This file (monorepo overview)
├── operator/                      # Kubernetes Operator (Go 1.24)
│   ├── CLAUDE.md                  # Operator-specific guidance
│   ├── api/v1alpha1/              # CRD type definitions (6 CRDs)
│   ├── cmd/                       # Entrypoints (operator, gRPC server)
│   ├── internal/
│   │   ├── controller/            # Reconciliation logic + provider implementations
│   │   ├── app/agent/             # Domain logic (reconcile, tools, instructions, models)
│   │   ├── infra/                 # Infrastructure layer (builder, repo, fakes)
│   │   ├── server/                # gRPC service implementations + auth
│   │   ├── telemetry/             # OpenTelemetry integration
│   │   └── webhook/               # Admission webhooks
│   ├── server/proto/              # Protobuf definitions (buf-managed)
│   ├── plugins/a2a/               # Argo Workflows A2A executor plugin
│   ├── charts/flokoa/             # Helm chart (templates, values, CRDs)
│   ├── config/                    # Kustomize manifests (CRDs, RBAC, manager, samples)
│   ├── test/e2e/                  # End-to-end tests (Kind cluster)
│   ├── Makefile                   # Build targets (build, test, deploy, docker, lint)
│   ├── Dockerfile                 # Multi-stage build (distroless runtime)
│   └── go.mod                     # Go 1.24.10 module
├── sdk/
│   └── python/                    # Python SDK (uv workspace, Python >= 3.13)
│       ├── pyproject.toml         # Workspace root (4 members)
│       ├── uv.lock                # Shared lockfile
│       ├── Makefile               # Workspace-level targets
│       ├── flokoa/                # Public SDK package (v0.0.5)
│       │   ├── CLAUDE.md          # SDK-specific guidance
│       │   ├── src/flokoa/        # Source: CLI, integrations, tools, utils
│       │   └── tests/             # pytest tests
│       ├── flokoa-types/          # Auto-generated Pydantic v2 models from CRD schemas
│       ├── flokoa-managed-agent/  # Operator-deployed pydantic-ai agent runtime
│       └── flokoa-managed-task/   # Operator-deployed Marvin task runtime (scaffold)
├── docs/                          # Documentation (Zensical/MkDocs site)
│   ├── *.md                       # Architecture, getting-started, CRD docs
│   └── examples/                  # 27 example YAML files (agents, tools, models, providers)
├── .github/workflows/             # CI/CD (5 workflows)
└── zensical.toml                  # Documentation site configuration
```

## Module-Specific Guidance

Each module has its own CLAUDE.md with detailed instructions:

- **Operator**: See `operator/CLAUDE.md` for Go/Kubebuilder development, CRDs, controllers, Argo plugins, gRPC server, and Helm chart
- **Python SDK**: See `sdk/python/flokoa/CLAUDE.md` for Python development, CLI, framework integrations, and generated types

**Always read the relevant module-level CLAUDE.md before working on that module.**

## Version Information

| Component | Version |
|-----------|---------|
| Operator (Makefile) | 0.0.6 |
| Helm Chart (appVersion) | 0.0.7 |
| Python SDK (`flokoa`) | 0.0.5 |
| API Version | v1alpha1 |
| Go | 1.24.10 |
| Python | >= 3.13 |

## Core CRDs

The operator manages six CRDs under `agent.flokoa.ai/v1alpha1`:

| CRD | Purpose | Key Fields |
|-----|---------|------------|
| **Agent** | AI agent deployment and lifecycle | `framework`, `runtime` (standard/template), `model`, `instruction`, `tools`, `card` (A2A metadata) |
| **AgentTool** | Tool definitions for agents | `type` (openapi), `source` (URL/ServiceRef), `schema`, `headers`, `timeout` |
| **AgentWorkflow** | Multi-agent workflows compiled to Argo Workflows | `tasks`, `params`, conditions, dependencies |
| **Model** | LLM model configuration | Provider-specific parameters (temperature, maxTokens, reasoning, etc.) |
| **ModelProvider** | Provider connection config | OpenAI, Anthropic, Google, Bedrock, API key refs, base URLs, TLS |
| **Instruction** | System prompt management | `content` (prompt text), creates ConfigMap |

## Cross-Module Concepts

### CRD-to-SDK Type Pipeline

Go CRD types flow to the Python SDK through an automated pipeline:

1. Edit Go types in `operator/api/v1alpha1/*_types.go`
2. Run `make manifests generate` in `operator/` to generate CRD YAML
3. Run `make generate-python-models` in `operator/` to generate Pydantic v2 models
4. Output lands in `sdk/python/flokoa-types/src/flokoa_types/`

**After modifying any CRD type, always run the full pipeline to keep Go and Python in sync.**

### A2A Protocol

The Agent-to-Agent (A2A) protocol is used across the platform:
- The Python SDK serves agents via A2A-compatible HTTP endpoints (FastAPI + a2a-sdk)
- The Argo Workflows executor plugin communicates with agents via A2A
- Agent cards (A2A metadata with skills) are defined on the Agent CRD

### Framework Integrations

Currently supported AI frameworks:
- **pydantic-ai** - Primary integration (`flokoa[pydantic-ai]`)
- **google-adk** - Google Agent Development Kit (`flokoa[google-adk]`)

The `flokoa-managed-agent` runtime (deployed by the operator) uses pydantic-ai.

## CI/CD Pipelines

| Workflow | File | Trigger | What It Does |
|----------|------|---------|--------------|
| Tests | `test.yml` | Push/PR | Go unit tests (`make test`), Docker build+push on main |
| Python SDK Tests | `test-python.yml` | Push/PR on `sdk/python/**` | `uv sync`, pytest with coverage, Codecov upload |
| E2E Tests | `test-e2e.yml` | Push/PR | Kind cluster creation, `make test-e2e` |
| Lint | `lint.yml` | Push/PR | golangci-lint v2.1.0 |
| Documentation | `docs.yml` | Push to main | Zensical build + GitHub Pages deploy |

## Quick Reference: Common Commands

### Operator (run from `operator/`)

```bash
# Build and test
make build                      # Build operator + server binaries
make test                       # Unit tests with envtest
make test-e2e                   # E2E tests with Kind cluster
make lint                       # golangci-lint

# Code generation (run after type changes)
make manifests generate         # CRDs + DeepCopy from Go types
make generate-python-models     # Pydantic models from CRD schemas
make buf-generate               # gRPC code from proto files

# Docker
make docker-build               # Build all images (operator, server, A2A plugin)
make docker-push                # Push to ghcr.io

# Deploy
make install                    # Install CRDs to cluster
make deploy                     # Deploy operator
make deploy-full                # Deploy everything (operator + Argo + plugins)
```

### Python SDK (run from `sdk/python/flokoa/`)

```bash
make install                    # Sync deps + pre-commit hooks
make test                       # pytest with coverage
make check                      # Lint (ruff) + type check (ty)
```

### Workspace-level (run from `sdk/python/`)

```bash
uv sync --all-packages --all-extras   # Sync all workspace members
uv lock                               # Update shared lockfile
```

## Docker Images

| Image | Registry | Build Target |
|-------|----------|-------------|
| Operator | `ghcr.io/danielnyari/flokoa-operator` | `make docker-build` |
| gRPC Server | `ghcr.io/danielnyari/flokoa-server` | `make docker-build` |
| A2A Plugin | `ghcr.io/danielnyari/flokoa-a2a-plugin` | `make docker-build-plugins` |
| Flokoa CLI | `ghcr.io/danielnyari/flokoa-cli` | `make docker-build-flokoa-cli` |

All images use multi-stage builds. The operator uses `gcr.io/distroless/static:nonroot` as runtime base. Python images use Alpine.

## Key Tech Stack

### Operator (Go)
- Kubebuilder v4, Operator SDK v1.42.0, controller-runtime v0.21.0
- Argo Workflows v3.7.9 (AgentWorkflow compilation + A2A executor plugin)
- gRPC with protobuf (buf-managed), grpc-gateway for REST
- Ginkgo v2 + Gomega for testing, envtest for unit tests, Kind for e2e
- OpenTelemetry for observability, go-oidc for auth
- Helm chart for deployment

### Python SDK
- uv workspace with 4 packages
- FastAPI + a2a-sdk for HTTP/A2A protocol
- pydantic-ai >= 1.44.0, google-adk >= 1.14.1 (optional)
- Ruff for linting, ty for type checking, pytest for tests
- Pre-commit hooks configured

## Working with the Codebase

### Adding a New Field to a CRD
1. Edit `operator/api/v1alpha1/*_types.go`
2. Add kubebuilder validation markers
3. Run `make manifests generate` in `operator/`
4. Update controller logic in `operator/internal/controller/`
5. Run `make generate-python-models` if the field affects SDK types
6. Add tests, run `make test`

### Adding a New CRD
1. Create `operator/api/v1alpha1/<name>_types.go`
2. Add kubebuilder markers and register in `groupversion_info.go`
3. Run `make manifests generate`
4. Create controller in `operator/internal/controller/`
5. Register controller in `operator/cmd/main.go`
6. Add sample CRs in `operator/config/samples/`
7. Run `make generate-python-models` if needed
8. Add tests

### Adding a New Framework Integration (Python SDK)
1. Create `sdk/python/flokoa/src/flokoa/integrations/<framework>/`
2. Implement `FlokoaAgentExecutor` subclass
3. Register in `integrations/__init__.py` (add to `IntegrationType` enum + `_EXTRA_NAMES` + `_try_load()`)
4. Add optional dependency in `sdk/python/flokoa/pyproject.toml`

## Project Status

This project is in early development. Key architectural components are in place:
- Six CRDs with controllers, webhooks, and gRPC services
- Python SDK with CLI, framework integrations, and OpenAPI tooling
- Argo Workflows integration with A2A executor plugin
- Helm chart for deployment
- CI/CD with unit tests, e2e tests, linting, and documentation
