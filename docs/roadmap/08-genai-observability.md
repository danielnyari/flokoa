# 08 — GenAI Observability by Default

**Phase:** 1 · **Size:** M · **Depends on:** — · **Enables:** 12 (cost controls need token metrics), 10 (dashboard)

## Goal

Every managed-runtime invocation emits traces **and metrics** with GenAI semantics (model, token usage, tool calls) without the user configuring anything beyond a collector endpoint — and the operator injects that endpoint. "Every harness invocation automatically generates traces, logs, and metrics" is the AgentCore bar; Flokoa is one wiring pass away from it.

## Current state

- **Tracing is built but opt-in and unwired in-cluster**:
  - Python: `flokoa/src/flokoa/telemetry.py` — `init_telemetry(service_name, restore_context_from_env=)` (TracerProvider + OTLP gRPC exporter iff `OTEL_EXPORTER_OTLP_ENDPOINT` set), `instrument_pydantic_ai()` (→ `Agent.instrument_all()`, which emits **GenAI semconv spans including token usage attributes** — the hard part is already free), `instrument_fastapi(app)`. The managed agent calls all three at boot (`flokoa_managed_agent/__main__.py`). All no-op gracefully when otel extras are missing (`tracing` extra in `flokoa/pyproject.toml`).
  - Go: `internal/telemetry/telemetry.go` — `Init(ctx, serviceName)` (OTLP iff env set), `Tracer(name)`, `NewTraceparent()`; A2A plugin wraps outbound HTTP in `otelhttp` and propagates traceparent; `FLOKOA_TRACEPARENT` restores context in one-shot tasks.
- **Gaps**:
  1. The operator never injects `OTEL_EXPORTER_OTLP_ENDPOINT` (or service name/resource attrs) into agent Deployments — in-cluster agents trace into the void unless users hand-set `TemplatedRuntimeSpec.Env`.
  2. **Zero metrics**: no MeterProvider anywhere in Python; Go side exposes only controller-runtime system metrics; nothing counts invocations, latency, or tokens.
  3. The gRPC server has logging/recovery interceptors but no otelgrpc tracing.
  4. `service.name` is the constant `"flokoa-managed-agent"` for every agent — useless for per-agent dashboards.

## Target design

### Operator: inject telemetry env per agent

- Helm value `telemetry.otlpEndpoint` (e.g. the collector's cluster DNS) → operator env `FLOKOA_AGENT_OTLP_ENDPOINT` → app layer adds to every template-runtime Deployment:
  - `OTEL_EXPORTER_OTLP_ENDPOINT=<endpoint>`
  - `OTEL_SERVICE_NAME=<agent name>` (per-agent identity)
  - `OTEL_RESOURCE_ATTRIBUTES=k8s.namespace.name=<ns>,flokoa.agent.name=<name>,flokoa.framework=<spec.framework>`
- Same `runtime_env.go` projection point as units 04/05; user-set `TemplatedRuntimeSpec.Env` entries win on conflict (explicit beats injected). No CRD change — this is operator policy, not agent declaration; a per-Agent override field is a recorded non-goal until someone needs it.

### Python: metrics + token accounting

- Extend `init_telemetry` to also build a `MeterProvider` (OTLP metric exporter, same endpoint gate) and expose `flokoa.telemetry.get_meter()`. Keep the ImportError-guard style.
- New `flokoa/src/flokoa/metrics.py` instrument set, recorded in `PydanticAIAgentExecutor.execute()` (and inherited by the templated executor):

| Instrument | Type | Attributes |
|---|---|---|
| `flokoa.agent.requests` | counter | `outcome` (ok/error), `flokoa.session` (bool) |
| `flokoa.agent.request.duration` | histogram (s) | `outcome` |
| `gen_ai.client.token.usage` | histogram | `gen_ai.token.type` (input/output), `gen_ai.request.model` |

  Token source: `result.usage()` on the pydantic-ai `AgentRunResult` (input/output token counts) — record per run. Use the official GenAI semconv instrument name for tokens so off-the-shelf dashboards work; keep `flokoa.*` namespace for harness-level RED metrics.
- Wrap `execute()` bodies' run section in a span `flokoa.agent.invoke` carrying `flokoa.agent.name` (from `OTEL_SERVICE_NAME`/resource), contextId (session id), task id — the parent for pydantic-ai's model/tool spans, making one invocation one trace subtree.

### Go: server + workflow spans

- Add `otelgrpc.NewServerHandler()` stats handler to the gRPC server options and `otelhttp` to the gateway/SSE mux — server requests join traces.
- (Already present: plugin outbound spans; workflow traceparent generation via `NewTraceparent()` — verify end-to-end continuity in the e2e trace test below.)

### Collector & dashboard plumbing

- Helm optional subchart/values for an `otel-collector` deployment (contrib image, OTLP in → Prometheus exporter + optional Tempo/OTLP out), gated `telemetry.collector.enabled=false` by default. Keep it minimal — users with existing collectors just set `telemetry.otlpEndpoint`.
- Grafana dashboard JSON committed under `operator/charts/flokoa/dashboards/agent-harness.json` (requests, latency, tokens by model, trace links) — content finalized in 10.

## Implementation plan

1. Python: MeterProvider in `init_telemetry`, `metrics.py`, executor instrumentation (span + counters + token histogram); extend the `tracing` extra with the OTLP metrics package if it's a separate dist; bump managed-agent deps.
2. Operator: env projection + Helm `telemetry.*` values + precedence rule (+ unit tests via fakes asserting env on built Deployment).
3. Go server: otelgrpc/otelhttp wiring + `telemetry.Init` call audit in the server entrypoint.
4. Collector values + example manifest; smoke-verify locally with `docker run otel/opentelemetry-collector-contrib` + `flokoa run`.

## Testing

- Python unit: with `InMemorySpanExporter`/`InMemoryMetricReader` (otel-sdk testing utils) assert: invoke span exists with attrs; token histogram recorded with input+output points after a TestModel run; counters increment on error paths.
- Go: builder env-injection table tests incl. user-override precedence.
- E2E (Kind, the money test): install with `telemetry.collector.enabled=true`; run a 2-task AgentWorkflow; query the collector (debug/logging exporter or Prometheus endpoint) and assert (a) one trace contains plugin→agent→model spans (continuity), (b) `gen_ai.client.token.usage` series exists labeled per agent. This test is the proof behind the "observable" claim.

## Acceptance criteria

- Fresh install with one Helm value set (`telemetry.otlpEndpoint`) → every template-runtime agent exports traces *and* metrics with per-agent `service.name`, with zero per-Agent configuration.
- Token usage per model is queryable in Prometheus; an invocation renders as a single coherent trace across plugin → agent → LLM spans.
- All telemetry remains optional: no endpoint → no exporter, no crashes, no warnings spam.

## Out of scope

- Log shipping (structured logs already go to stdout; collection is cluster policy). Cost *enforcement* (12). Dashboard polish + docs walkthrough (10). Standard-runtime instrumentation (document the env contract; their image, their telemetry).
