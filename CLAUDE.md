# CLAUDE.md - Flokoa

This document provides guidance for AI assistants working with the Flokoa codebase.

## Project Overview

Flokoa is an open-source platform for managing AI Agents in Kubernetes clusters. It consists of:

1. **Kubernetes Operator** (Go) - Declarative deployment and lifecycle management of AI agents through CRDs, with a gRPC API server, Helm chart, and Argo Workflows executor plugin
2. **Python SDK** - Client library and CLI for building and running pydantic-ai agents locally, with A2A protocol support

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
│       ├── pyproject.toml         # Workspace root
│       ├── uv.lock                # Shared lockfile
│       ├── Makefile               # Workspace-level targets
│       ├── flokoa/                # Public SDK package
│       │   ├── CLAUDE.md          # SDK-specific guidance
│       │   ├── src/flokoa/        # Source: CLI, pydantic-ai integration, tools, utils
│       │   └── tests/             # pytest tests
│       ├── flokoa-types/          # Auto-generated Pydantic v2 models from CRD schemas
│       ├── flokoa-runner/         # Generic runner: compiled-spec hydration + A2A serving
│       ├── flokoa-codemode-mcp/   # Code-mode MCP server package
│       └── flokoa-common/         # Shared internal helpers
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

The structured knowledge base under [`docs/`](docs/) is the system of record; the operating
principles for the codebase are in [`docs/design-docs/core-beliefs.md`](docs/design-docs/core-beliefs.md).
`AGENTS.md` (at the root and in each module) is a symlink to the adjacent `CLAUDE.md`, so
non-Claude agents (Codex, Cursor, …) read the same maps.

## Version Information

Component versions are aligned and driven by the release process: pushing a
`v*` tag runs `.github/workflows/release.yml`, which derives the version from
the tag for all images, the Helm chart, and the PyPI packages (see
`CHANGELOG.md` for history). Don't hand-maintain version numbers in docs.

| Component | Version |
|-----------|---------|
| API Version | v1alpha1 |
| Go | 1.24 |
| Python | >= 3.13 |

## Core CRDs

The operator manages eight CRDs under `agent.flokoa.ai/v1alpha1`:

| CRD | Purpose | Key Fields |
|-----|---------|------------|
| **Agent** | Composition root: compiles refs + inline AgentSpec fragment into one resolved pydantic-ai spec | `spec` (inline fragment), `modelRef`, `instructionRefs`, `tools`, `capabilities`, `secretRefs`, `card` (A2A metadata), `runtime` (image/runnerVersion/isolation) |
| **Capability** | Versioned, digest-pinned, schema-published unit of agent behavior; admission machine-checks config schema, `requires` tuple, and dependency conflicts | `artifact` (digest-pinned OCI ref), `version`, `entrypoint`, `configSchema`, `schemaPolicy`, `requires`, `dependencies` |
| **AgentTool** | Declarative MCP endpoint (openapi type retired) | `url`/`serviceRef`+`path`, `transport`, `headers`, `headerSecrets`, `toolPrefix`, `allowedTools`, `timeoutSeconds` |
| **AgentWorkflow** | **Frozen** template-only A2A composition between deployed Agents (compiled to Argo WorkflowTemplates; no new features — see roadmap §7) | `tasks`, `params`, conditions, dependencies |
| **AgentTrigger** | Event-driven agent invocation via Argo Events | `eventSource`, `filter`, `agent`, `task`, `pushNotification`, `limits` |
| **Model** | Named, shareable model config compiling to AgentSpec `model`+`model_settings` | `model`, `providerRef`, `settings` (typed common fields + `extra` passthrough) |
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

### Framework

flokoa targets **pydantic-ai** exclusively (`flokoa[pydantic-ai]`). The
`flokoa-runner` generic runner (deployed by the operator) hydrates compiled
AgentSpecs via `Agent.from_spec` — see `docs/reference/runtime-contract.md`.

## CI/CD Pipelines

| Workflow | File | Trigger | What It Does |
|----------|------|---------|--------------|
| Tests | `test.yml` | Push/PR | Go unit tests (`make test`), Docker build+push on main |
| Python SDK Tests | `test-python.yml` | Push/PR on `sdk/python/**` | `uv sync`, pytest with coverage, Codecov upload |
| E2E Tests | `test-e2e.yml` | Push/PR | Kind cluster creation, `make test-e2e` |
| Lint | `lint.yml` | Push/PR | golangci-lint v2.1.0 |
| Documentation | `docs.yml` | Push to main | Zensical build + GitHub Pages deploy |
| Release | `release.yml` | `v*` tag | Test gate, image build matrix, Helm chart push (OCI), install.yaml bundle, GitHub Release, optional PyPI publish |

## Quick Reference: Common Commands

### Local dev — boot the whole stack on minikube (run from repo root)

```bash
echo "OPENAI_API_KEY=sk-..." > .env   # read automatically
make up                               # start minikube, build images in-cluster,
                                      # deploy operator+server+Argo+plugins+sample agent,
                                      # port-forward Flokoa/Argo/yakd UIs
make down                             # stop forwards + undeploy (ARGS=--stop-minikube to stop the VM)
make urls                             # print the UI URLs
```

`make up` delegates to `operator/` `make local-up` (script: `operator/hack/local-up.sh`).
It builds images **directly into minikube's docker daemon** (scoped `eval $(minikube
docker-env)` in a subshell) so locally-built images are used without a registry push or
`image load`. Non-`latest` tags (operator/server `:0.1.0`, runner `:0.2.0`) get the default
`IfNotPresent` policy; the A2A plugin's `:latest` image is pinned to `IfNotPresent` in
`operator/plugins/a2a/config/plugin.yaml`. The runner tag must match
`spec.DefaultRunnerVersion` in `operator/internal/spec/spec.go`. Knobs:
`WITH_TESTDATA=false`, `CONTAINER_TOOL=podman`, `FLOKOA_UI_PORT`/`ARGO_UI_PORT`/`YAKD_UI_PORT`.

### Per-module commands

Full command surfaces live in the module maps (run `make help` in each module):

- **Operator** (`operator/`) — build, test, codegen, lint, deploy, `verify-codegen`. See [`operator/CLAUDE.md`](operator/CLAUDE.md).
- **Python SDK** (`sdk/python/flokoa/`) — `make install` / `test` / `check`. See [`sdk/python/flokoa/CLAUDE.md`](sdk/python/flokoa/CLAUDE.md).
- **Workspace** (`sdk/python/`) — `uv sync --all-packages --all-extras`, `uv lock`.

## Docker Images

| Image | Registry | Build Target |
|-------|----------|-------------|
| Operator | `ghcr.io/danielnyari/flokoa-operator` | `make docker-build` |
| gRPC Server | `ghcr.io/danielnyari/flokoa-server` | `make docker-build` |
| A2A Plugin | `ghcr.io/danielnyari/flokoa-a2a-plugin` | `make docker-build-plugins` |
| Generic runner | `ghcr.io/danielnyari/flokoa-runner` | `make docker-build-runner` (sdk/python) |

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
- uv workspace (flokoa, flokoa-types, flokoa-runner, flokoa-codemode-mcp, flokoa-common)
- FastAPI + a2a-sdk for HTTP/A2A protocol
- pydantic-ai >= 1.44.0 (the only framework integration)
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


## Project Status

This project is in early development, executing the Pivot v2.1 roadmap
(`docs/roadmap/`). Phase 0, P0a (units 02–07: runtime contract, spec
compiler, generic runner, virtual endpoint, injected telemetry), and unit 08
(Capability CRD + admission) are done; the rest of P0b — 09 (capability
artifacts & delivery) and 10 (capability CLI & registry seeding) — is next.
Key architectural components:
- Eight CRDs with controllers, admission webhooks, and gRPC services
- Python SDK with CLI, pydantic-ai integration, and OpenAPI tooling
- Argo Workflows integration with A2A executor plugin
- Helm chart for deployment
- CI/CD with unit tests, e2e tests, linting, and documentation
