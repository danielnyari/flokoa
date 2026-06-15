# Python SDK & CLI

The **`flokoa` Python package** is the client side of Flokoa: a `flokoa` CLI for
running agents and authoring capabilities locally, plus a small library for
serving pydantic-ai agents over [A2A](https://a2a-protocol.org/). It is the same
machinery the in-cluster [generic runner](reference/runtime-contract.md) uses, so
what you run locally mirrors what the operator runs in production.

The operator (Go) and the SDK (Python) are complementary: the operator *compiles
and deploys* agents from CRDs; the SDK *builds, runs, and packages* the Python
that those agents execute.

## Install

```bash
pip install flokoa                 # CLI + A2A serving library
pip install "flokoa[pydantic-ai]"  # adds pydantic-ai — required to run agents
pip install "flokoa[tracing]"      # adds OpenTelemetry SDK + OTLP exporter
```

- **Python ≥ 3.13** is required.
- The `pydantic-ai` extra pins a version compatible with the runner baseline
  (`pydantic-ai >= 1.107.0, < 2`). Anything that hydrates an `AgentSpec`
  (`flokoa run -f`, capability builds) needs it.
- The `tracing` extra wires the same telemetry the platform injects in-cluster.

## `flokoa run` — serve an agent locally

`flokoa run` starts an A2A server for one agent. It has two modes:

```bash
# 1. Serve a hand-built pydantic-ai Agent (uvicorn-style module:attr)
flokoa run -m my_package.agents:support_agent

# 2. Serve a compiled AgentSpec file — the local mirror of the cluster runner
flokoa run -f agent-spec.yaml
```

| Flag | Description |
|---|---|
| `-m, --module` | Import path `module:attr` to a constructed pydantic-ai `Agent`. |
| `-f, --file` | Path to an `AgentSpec` (YAML/JSON); hydrated via `Agent.from_spec`. Requires the `pydantic-ai` extra. |
| `--host` | Bind host (default `localhost`). |
| `--port` | Bind port (default `10001`). |

Exactly one of `-m` / `-f` is required. The server builds the agent's A2A card,
mounts the A2A FastAPI app, initializes telemetry, and serves with uvicorn — the
same `build_app` / executor path the runner uses, so a spec that serves locally
serves in-cluster.

## `flokoa capability` — author and publish capabilities

The `flokoa capability` command group builds, signs, publishes, and discovers
[Capability](capability.md) artifacts (OCI wheelhouses):

| Command | Purpose |
|---|---|
| `flokoa capability build` | Build an artifact (wheelhouse + manifest + generated CR + config schema) from a project or `--from-pypi`. |
| `flokoa capability push` | Push the built OCI artifact, pin the CR to the published digest, optionally sign (`--sign`) and apply (`--apply`). |
| `flokoa capability import` | One-shot `build --from-pypi` → schema review → `push` for any PyPI package. |
| `flokoa capability search` | Search the published index and in-cluster Capability CRs. |
| `flokoa capability list` | List everything `search` would match. |

The build runs inside the pinned runner image so the wheelhouse and derived
config schema match the runtime exactly. See the
**[capabilities guide](guides/capabilities.md)** for the full workflow,
prerequisites (docker, crane, cosign, kubectl), and option reference.

## Serving as a library

For embedding an agent in your own FastAPI/ASGI app, the serving primitives are
public:

```python
from pydantic_ai import Agent
from flokoa.serving import build_app
from flokoa.utils.agent_card_builder import AgentCardBuilder

agent = Agent("openai:gpt-5-mini", instructions="You are helpful.")
card = AgentCardBuilder(name="demo", description="Demo", version="0.1.0").build()
app = build_app(agent, card)   # an A2A-compatible FastAPI app
```

- `flokoa.serving` — `build_app(agent, card) -> FastAPI` and `SpecAgentExecutor`
  (the A2A `AgentExecutor` wrapping a constructed pydantic-ai agent).
- `flokoa.context` — accessors for capability authors:
  `agent_name()`, `agent_namespace()`, `public_url()`, `context_id()`,
  `task_id()`, and `bind_request()`.
- `flokoa.telemetry` — `init_telemetry`, `instrument_pydantic_ai`,
  `instrument_fastapi`.

## Workspace packages

The SDK is a uv workspace; only `flokoa` is published to PyPI. The rest are
internal building blocks:

| Package | Role |
|---|---|
| **`flokoa`** | Public SDK — the `flokoa` CLI and the A2A serving/context/telemetry library. |
| **`flokoa-types`** | Auto-generated Pydantic v2 models from the CRD schemas (`make generate-python-models`). Do not edit by hand. |
| **`flokoa-runner`** | The generic runner baked into the operator's runner image; hydrates a compiled spec via `Agent.from_spec` and serves it. Owns the `pydantic-ai==1.107.0` pin. |
| **`flokoa-codemode-mcp`** | Code-mode MCP server (LLM-generated code executed against OpenAPI specs in a sandbox). |
| **`flokoa-openapi`** | Turns an OpenAPI document into typed pydantic-ai tools; ships as a Capability. |
| **`flokoa-common`** | Shared internals (OpenAPI parsing, auth, URL/SSRF validation). Not a public API. |

## Relationship to the runtime contract

`flokoa run -f` and the in-cluster runner both call `Agent.from_spec` on a
compiled `AgentSpec` — the local command is the development mirror of the
operator's delivery path. The normative operator↔runner interface (mount paths,
env vars, secret projection, capability delivery) is the
[runtime contract](reference/runtime-contract.md).
