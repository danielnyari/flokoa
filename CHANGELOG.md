# Changelog

All notable changes to Flokoa are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Capability CRD + admission** (roadmap 08): `Capability` is a versioned,
  digest-pinned, schema-published unit of agent behavior. Agent admission now
  machine-checks the compatibility matrix before anything deploys —
  attachment config against the capability's published JSON Schema
  (`${secret:NAME}` placeholders validate as shapes), the `requires` tuple
  against the Agent's runner baseline, and dependency-conflict detection
  across attachments plus the runner lockfile. The compiler re-runs the same
  checks on every recompile and emits attachment entries + artifact delivery
  inputs; `schemaPolicy: permissive` is loudly surfaced in status, warnings,
  and printcolumns. Artifact delivery (09) and the capability CLI (10) are
  next.

## [0.2.0] - 2026-06-11

First post-pivot release, executing Phase 0 of the
[Pivot v2.1 roadmap](docs/roadmap/README.md): flokoa is the open-source agent
harness for Kubernetes, targeting **pydantic-ai exclusively**.

### Added

- **Admission webhooks in the Helm chart**: webhook Service,
  `ValidatingWebhookConfiguration` for all seven CRDs, cert-manager
  Issuer/Certificate (with a manual cert/CA path for clusters without
  cert-manager), and controller wiring — enabled by default via
  `webhooks.enabled`. Chart installs now have admission validation active.
- Root `README.md` with positioning and quickstart, and
  `docs/agenttrigger.md` documenting the shipped Argo Events-based
  AgentTrigger design (replacing the stale Knative-era RFC).

### Changed

- **AgentWorkflow is frozen** as a template-only resource (static A2A
  composition between deployed Agents). The `agentTask` task type is
  unsupported: the admission webhook rejects new usage and the compiler
  refuses to compile it; the field remains only for API compatibility.
- The Python builder/toolset registries are single-framework: builders are
  keyed by agent type, toolset builders by tool type, and
  `LlmAgentConfig.framework` / `IntegrationType` were removed.

### Removed

- **google-adk integration**: the executor, its tests, the `google-adk`
  extra, and the ADK-specific agent-card machinery.
- **The integrations registry** (`flokoa.integrations` dispatch);
  `PydanticAIAgentExecutor` is the only executor and is imported directly.
- **`flokoa-managed-task`** (the Marvin task runtime): the package, its
  Dockerfile and image targets, its release build-matrix entry, the
  `taskconfig` generated types, and the `agentTask` samples.
- The `flokoa run --framework` CLI flag (pydantic-ai is the framework).
- The removed frameworks' values from the Agent CRD `framework` enum and the
  gRPC `Framework` enum.

## [0.1.0] - 2026-06-10

First public alpha release of Flokoa — an open-source platform for managing AI
agents in Kubernetes.

### Added

- **Kubernetes Operator** managing six CRDs under `agent.flokoa.ai/v1alpha1`:
  `Agent`, `AgentTool`, `AgentWorkflow`, `Model`, `ModelProvider`, and
  `Instruction`, each with controllers, admission webhooks, and structured
  error classification.
- **Agent runtime modes**: `standard` (user-supplied image) and `template`
  (operator-managed pydantic-ai runtime).
- **AgentWorkflow compiler** translating workflows into Argo
  `WorkflowTemplate`s, with an A2A executor plugin (sidecar) for calling
  agents from Argo Workflows.
- **gRPC/REST server** with five services, grpc-gateway REST, SSE watch
  endpoints, an AG-UI playground, and optional OIDC authentication; serves an
  embedded Nuxt 4 web UI.
- **Python SDK** (uv workspace): the public `flokoa` CLI/library with
  pydantic-ai and google-adk integrations, OpenAPI tooling, and A2A serving;
  `flokoa-types` generated Pydantic models; `flokoa-managed-agent` and
  `flokoa-managed-task` operator-deployed runtimes.
- **Helm chart** for deploying the controller, server, and A2A plugin.
- **Release machinery**: tag-triggered `release.yml` workflow that builds and
  pushes all images (operator, server, a2a-plugin, flokoa-cli, managed-task)
  with semver tags, packages and pushes the Helm chart to the GHCR OCI
  registry, generates a single-file `install.yaml` bundle, and creates a
  GitHub Release (with opt-in PyPI publishing).

### Changed

- Aligned all component versions to `0.1.0` (operator Makefile, Helm chart
  `version`/`appVersion`, kustomize image tags, and Python packages).
- Pinned the default managed runtime images to the release tag: the template
  runtime image is now `ghcr.io/danielnyari/flokoa-cli:0.1.0` and the managed
  Marvin task image is `ghcr.io/danielnyari/flokoa-managed-task:0.1.0`
  (renamed from the previous inconsistent `flokoa/managed-task` path).

[Unreleased]: https://github.com/danielnyari/flokoa/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/danielnyari/flokoa/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/danielnyari/flokoa/releases/tag/v0.1.0
