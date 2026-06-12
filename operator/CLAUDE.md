# CLAUDE.md — Flokoa Kubernetes Operator

Map for AI assistants and engineers working in `operator/`. This is a table of contents, not an
encyclopedia: it points to the system of record under [`../docs/`](../docs/) for detail.
(`AGENTS.md` in this directory is a symlink to this file.)

The Flokoa Operator manages AI Agents through CRDs under the `agent.flokoa.ai` API group (Go 1.24).

## Read next

- **Operating principles** → [`../docs/design-docs/core-beliefs.md`](../docs/design-docs/core-beliefs.md)
  (golden principles + the registry of enforced invariants)
- **Conventions, architecture, testing** → [`../docs/reference/operator-conventions.md`](../docs/reference/operator-conventions.md)
- **Runtime contract (normative)** → [`../docs/reference/runtime-contract.md`](../docs/reference/runtime-contract.md)
- **Argo executor plugins** → [`../docs/argo/executor-plugins.md`](../docs/argo/executor-plugins.md)
- **CRD-to-Python type pipeline** → the `generate-python-types` skill

## Tech stack

| Component | Version | | Component | Version |
|-----------|---------|-|-----------|---------|
| Go | 1.24.10 | | controller-runtime | v0.21.0 |
| Kubebuilder | v4 layout | | Kubernetes API | v0.33.1 |
| Operator SDK | v1.42.0 | | Argo Workflows | 3.7.9 |
| gRPC / grpc-gateway | v1.72.2 / v2.26.3 | | OpenTelemetry | v1.36.0 |

Testing: Ginkgo v2 + Gomega (envtest for unit, Kind for e2e). Linting: golangci-lint v2.1.0.

## Layout

```
operator/
├── api/v1alpha1/        # CRD type definitions + webhooks (depends on nothing internal)
├── cmd/                 # Entrypoints: main.go (operator), server/main.go (gRPC)
├── internal/
│   ├── controller/      # Reconcilers + provider handlers + agentworkflow_compiler.go
│   ├── app/             # Orchestration: compile → spec ConfigMap → Deployment/Services
│   │   └── agent/compiler/   # Spec compiler (resolve → merge → inject → validate → emit)
│   ├── domain/          # Pure domain models & functions (leaf layer)
│   ├── infra/           # builder/ (K8s resource construction) + repo/ (data access)
│   ├── server/          # gRPC services + OIDC auth
│   ├── spec/            # Embedded AgentSpec JSON Schemas + validator
│   ├── telemetry/       # OpenTelemetry
│   └── webhook/v1alpha1/ # Admission webhooks
├── server/proto/        # Protobuf (buf-managed) → server/gen/
├── plugins/a2a/         # Argo A2A executor plugin  (docs/argo/executor-plugins.md)
├── charts/flokoa/       # Helm chart
├── config/              # Kustomize manifests (crd/, rbac/, manager/, server/, samples/)
└── test/                # e2e/ (Kind), integration/ (Docker-free), utils/
```

The layering and its **enforced** dependency directions are in
[operator-conventions](../docs/reference/operator-conventions.md#layered-architecture-enforced).

## Core CRDs

Eight CRDs under `agent.flokoa.ai/v1alpha1` (full schemas in [`../docs/`](../docs/)):

- **Agent** — composition root; compiles an inline AgentSpec fragment + `modelRef`/`instructionRefs`/`tools`/`secretRefs` into one resolved, schema-validated AgentSpec (`internal/app/agent/compiler`), delivered as the `<agent>-agent-spec` ConfigMap.
- **Capability** — versioned, digest-pinned unit of agent behavior (OCI wheelhouse artifact mirror); admission machine-checks config schema, `requires` tuple, and dependency conflicts before anything deploys. See [`../docs/capability.md`](../docs/capability.md).
- **AgentTool** — declarative MCP endpoint (`url`/`serviceRef`, transport, header secrets, `allowedTools`). The `openapi` type is retired.
- **Model** / **ModelProvider** — shareable model config + provider connection (OpenAI, Anthropic, Google, Bedrock).
- **Instruction** — system prompt → ConfigMap.
- **AgentWorkflow** — **frozen** template-only A2A composition → Argo `WorkflowTemplate`s (no new features; see `../docs/roadmap/`).
- **AgentTrigger** — event-driven invocation via Argo Events → Sensor. See [`../docs/agenttrigger.md`](../docs/agenttrigger.md).

## Key commands

Run from `operator/`. `make help` lists everything.

```bash
# Codegen (run after editing api/v1alpha1/*_types.go)
make manifests generate            # CRDs, RBAC, webhooks + DeepCopy
make generate-python-models        # Pydantic models for the SDK (needs yq)
make buf-generate                  # gRPC code from proto

# Build & test
make build                         # manager + server binaries
make test                          # unit (envtest)
make test-integration              # Docker-free: manager + real runner over A2A
make test-e2e                      # Kind cluster

# Lint & invariant gates
make lint                          # golangci-lint (incl. depguard layer boundaries)
make verify-codegen                # fail if generated artifacts are stale
make verify-endpoint-refs          # docs/samples must use the published endpoint

# Local one-shot boot (minikube) — also `make up`/`down`/`urls` from the repo root
make local-up                      # build images into minikube, deploy-full, port-forward UIs
make local-down                    # stop forwards + undeploy-full

# Deploy
make deploy-full                   # operator + Argo Workflows + executor plugins
```

`make local-up` (script: `hack/local-up.sh`) builds images directly into minikube's docker
daemon and reads `OPENAI_API_KEY` from the repo-root `.env`. The runner image tag must match
`spec.DefaultRunnerVersion` in `internal/spec/spec.go` — bump both together.

## Working with the codebase

- **Add a field to a CRD**: edit `api/v1alpha1/*_types.go` (+ kubebuilder markers) → `make manifests generate` → update `internal/controller/` → `make generate-python-models` if it affects SDK types → tests → `make test`.
- **Add a CRD**: new `api/v1alpha1/<name>_types.go` → register in `groupversion_info.go` → `make manifests generate` → controller in `internal/controller/` (with RBAC markers) → register in `cmd/main.go` → sample in `config/samples/` → tests.
- **Never hand-edit generated code** (`zz_generated.*.go`, CRD YAML, `flokoa_types/*.py`); regenerate. CI runs `make verify-codegen`.

Conventions, the controller pattern, testing tiers, provider implementations, and common issues:
[operator-conventions](../docs/reference/operator-conventions.md).

## Docker images

| Image | Make target | Registry |
|-------|-------------|----------|
| Operator | `docker-build` / `docker-push` | `ghcr.io/danielnyari/flokoa-operator` |
| Server | `docker-build` / `docker-push` | `ghcr.io/danielnyari/flokoa-server` |
| A2A Plugin | `docker-build-plugins` / `docker-push-plugins` | `ghcr.io/danielnyari/flokoa-a2a-plugin` |
| Generic runner | `make docker-build-runner` (in `sdk/python/`) | `ghcr.io/danielnyari/flokoa-runner` |

Image versions are driven by the release process (a `v*` tag) — don't hand-maintain them.
