# flokoa

The Python SDK and CLI for [Flokoa](https://github.com/danielnyari/flokoa) — the
open-source agent harness for Kubernetes. Build, run, and package
[pydantic-ai](https://ai.pydantic.dev) agents locally with the same machinery the
Flokoa operator runs in your cluster.

## Install

```bash
pip install flokoa                 # CLI + A2A serving library
pip install "flokoa[pydantic-ai]"  # adds pydantic-ai — required to run agents
pip install "flokoa[tracing]"      # adds OpenTelemetry SDK + OTLP exporter
```

Requires **Python ≥ 3.13**.

## Run an agent

```bash
# Serve a hand-built pydantic-ai Agent (module:attr)
flokoa run -m my_package.agents:support_agent

# Serve a compiled AgentSpec file (the local mirror of the cluster runner)
flokoa run -f agent-spec.yaml --port 10001
```

`flokoa run` serves a single agent over the [A2A](https://a2a-protocol.org/)
protocol (default `localhost:10001`). `-f` hydrates an `AgentSpec` via
`Agent.from_spec`, exactly as the in-cluster generic runner does.

## Author a capability

```bash
flokoa capability build ./my-capability --tag ghcr.io/me/caps/my-cap:0.1.0
flokoa capability push  ghcr.io/me/caps/my-cap:0.1.0 --sign --apply
```

Build, sign, publish, and discover Capability artifacts (OCI wheelhouses).
Subcommands: `build`, `push`, `import`, `search`, `list`.

## Serve as a library

```python
from pydantic_ai import Agent
from flokoa.serving import build_app
from flokoa.utils.agent_card_builder import AgentCardBuilder

agent = Agent("openai:gpt-5-mini", instructions="You are helpful.")
card = AgentCardBuilder(name="demo", description="Demo", version="0.1.0").build()
app = build_app(agent, card)  # an A2A-compatible FastAPI app
```

## Documentation

- [Python SDK & CLI guide](https://github.com/danielnyari/flokoa/blob/main/docs/sdk.md)
- [Capabilities guide](https://github.com/danielnyari/flokoa/blob/main/docs/guides/capabilities.md)
- [Flokoa documentation](https://github.com/danielnyari/flokoa/tree/main/docs)

## License

[Apache 2.0](https://github.com/danielnyari/flokoa/blob/main/LICENSE)
