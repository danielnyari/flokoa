# flokoa

**flokoa is the open-source agent harness for Kubernetes — what AgentCore is for AWS.**

A Kubernetes-native runtime for single agents and swarms of [pydantic-ai](https://ai.pydantic.dev) agents: declarative definitions, packaged capabilities, isolated sessions, event triggers, and an A2A gateway, on your own cluster.

- **Single framework, deep integration** — built on pydantic-ai primitives, not wrappers around them
- **Declarative core** — define agents, models, instructions, and tools as Kubernetes resources; edit YAML, behavior changes, no image rebuild
- **Boring infrastructure** — OCI registries, Argo Events, OpenTelemetry, Helm, lean controllers

## Quickstart

Install the operator with Helm (chart published to GHCR on every release):

```bash
helm install flokoa oci://ghcr.io/danielnyari/charts/flokoa
```

Then follow the **[getting started guide](docs/getting-started.md)**.

## Documentation

- [Getting started](docs/getting-started.md)
- [Architecture](docs/architecture.md)
- [Agent CRD](docs/agent.md) · [AgentTool](docs/agenttool.md) · [Model](docs/model.md) · [ModelProvider](docs/modelprovider.md)
- [Quick reference](docs/quick-reference.md)
- [Roadmap](docs/roadmap/README.md) — the Pivot v2.1 plan this project is executing

## Repository layout

- `operator/` — Kubernetes operator (Go): CRDs, controllers, admission webhooks, gRPC API, Helm chart
- `sdk/python/` — Python SDK (uv workspace): `flokoa` CLI/library, generated CRD types, managed agent runtime

## Status

Early development. The CRD surface and runtime contract are evolving; see the [roadmap](docs/roadmap/README.md) for what is frozen, kept, and coming.

## License

[Apache 2.0](LICENSE)
