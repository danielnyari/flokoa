# Changelog

All notable changes to Flokoa are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/danielnyari/flokoa/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/danielnyari/flokoa/releases/tag/v0.1.0
