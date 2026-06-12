# flokoa

**flokoa is the open-source agent harness for Kubernetes**

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

## Local development (one command)

Boot the entire stack on a local [minikube](https://minikube.sigs.k8s.io/) — cluster,
operator, gRPC server, Argo Workflows + the A2A executor plugin, a sample agent, and
background port-forwards for every UI:

```bash
echo "OPENAI_API_KEY=sk-..." > .env   # read automatically by `make up`
make up
```

`make up` is idempotent and reuses the existing Makefile targets. It will:

1. start (or refresh) minikube and enable the `yakd` dashboard addon;
2. build all images **straight into minikube's docker daemon** — no registry push,
   no `image load`, and no `:latest` pull surprises;
3. install the CRDs and deploy the operator, server, Argo Workflows and executor plugins;
4. create the `openai-api-key` secret and deploy the petstore sample agent + demo workflow;
5. port-forward the three UIs in the background and print their URLs:

   | UI | URL |
   |----|-----|
   | Flokoa UI | http://localhost:8080 |
   | Argo Workflows | https://localhost:2746 (self-signed) |
   | yakd (Kubernetes dashboard) | http://localhost:8081 |

Tear it down with:

```bash
make down                      # stop port-forwards + undeploy workloads
make down ARGS=--forwards-only # only stop the port-forwards
make down ARGS=--stop-minikube # also stop the minikube VM
```

**Knobs** (env vars): `WITH_TESTDATA=false` skips the sample agent, `CONTAINER_TOOL=podman`
swaps the builder, and `FLOKOA_UI_PORT` / `ARGO_UI_PORT` / `YAKD_UI_PORT` remap the local
ports. The underlying targets live in [`operator/Makefile`](operator/Makefile)
(`make local-up` / `local-down` / `local-urls`).

## Documentation

- [Getting started](docs/getting-started.md)
- [Architecture](docs/architecture.md)
- [Agent CRD](docs/agent.md) · [AgentTool](docs/agenttool.md) · [Model](docs/model.md) · [ModelProvider](docs/modelprovider.md) · [AgentTrigger](docs/agenttrigger.md)
- [Quick reference](docs/quick-reference.md)
- [Roadmap](docs/roadmap/README.md) — the Pivot v2.1 plan this project is executing

## Repository layout

- `operator/` — Kubernetes operator (Go): CRDs, controllers, admission webhooks, gRPC API, Helm chart
- `sdk/python/` — Python SDK (uv workspace): `flokoa` CLI/library, generated CRD types, the generic runner (`flokoa-runner`)

## Status

Early development, executing the Pivot v2.1 roadmap. Phase 0 and P0a — runtime contract, spec compiler, generic runner, virtual endpoint identity, and injected telemetry — are done; the Capability CRD (P0b) is next, and isolated sessions, the A2A session-routing gateway, and swarms (SwarmRun) are future work. See the [roadmap](docs/roadmap/README.md) for what is frozen, kept, and coming.

## License

[Apache 2.0](LICENSE)
