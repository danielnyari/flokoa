# Flokoa Operator

The Kubernetes operator behind [Flokoa](https://github.com/danielnyari/flokoa) —
the open-source agent harness for Kubernetes. It manages AI agents declaratively
through CRDs under the `agent.flokoa.ai` API group: it **compiles** each Agent
(an inline pydantic-ai AgentSpec fragment plus Model/Instruction/AgentTool/
Capability references) into one resolved, schema-validated spec and runs it on
the generic runner — no per-agent image builds.

It also ships a gRPC/REST API server, admission webhooks, an Argo Workflows A2A
executor plugin, and a Helm chart.

- **Module guide (start here):** [`CLAUDE.md`](CLAUDE.md) — layout, commands, conventions
- **User documentation:** [`../docs/`](../docs/) — [getting started](../docs/getting-started.md), [architecture](../docs/architecture.md), CRD reference
- **Normative interface:** [runtime contract](../docs/reference/runtime-contract.md)

## Prerequisites

- Go 1.24+
- Docker (or Podman) and `kubectl`
- A Kubernetes 1.25+ cluster (Helm 3.8+ for OCI chart installs)

## Install

```bash
# Helm (chart published to GHCR on each release)
helm install flokoa oci://ghcr.io/danielnyari/charts/flokoa

# …or the manifest bundle attached to each GitHub release
kubectl apply -f https://github.com/danielnyari/flokoa/releases/latest/download/install.yaml
```

> **Pre-release:** no `v*` tag is published yet, so neither the chart nor the
> `install.yaml` bundle is on GHCR/Releases until `v0.1.0` ships. To run locally
> in the meantime, use `make up` from the repository root (minikube one-shot).

## Local development

```bash
# From the repo root: full local stack on minikube (operator + server + Argo + sample agent)
make up        # build images into minikube, deploy, port-forward the UIs
make down      # tear it down

# From operator/: equivalent targets and a la carte deploys
make local-up        # build images into minikube + deploy-full + port-forward
make deploy-full     # operator + Argo Workflows + executor plugins
```

`make local-up` (script: [`hack/local-up.sh`](hack/local-up.sh)) builds images
directly into minikube's docker daemon and reads `OPENAI_API_KEY` from a repo-root
`.env`.

## Build, test, codegen

Run from `operator/`; `make help` lists everything.

```bash
make build                 # manager + server binaries
make test                  # unit tests (envtest)
make test-integration      # Docker-free: manager + real runner over A2A
make test-e2e              # Kind cluster

make manifests generate    # CRDs, RBAC, webhooks, DeepCopy (after editing api/v1alpha1/*_types.go)
make generate-python-models # regenerate the Python SDK types (needs yq)
make buf-generate          # gRPC code from proto

make lint                  # golangci-lint (incl. layer-boundary depguard)
make verify-codegen        # fail if generated artifacts are stale
```

## Images

Image versions are driven by the release process (a `v*` tag); don't hand-maintain them.

| Image | Make target | Registry |
|-------|-------------|----------|
| Operator | `docker-build` / `docker-push` | `ghcr.io/danielnyari/flokoa-operator` |
| Server | `docker-build` / `docker-push` | `ghcr.io/danielnyari/flokoa-server` |
| A2A plugin | `docker-build-plugins` / `docker-push-plugins` | `ghcr.io/danielnyari/flokoa-a2a-plugin` |
| Generic runner | `make docker-build-runner` (in `sdk/python/`) | `ghcr.io/danielnyari/flokoa-runner` |

## License

Apache 2.0 — see [LICENSE](../LICENSE).
