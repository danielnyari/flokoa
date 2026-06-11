# 07 — Platform-Injected Capabilities

**Phase:** P0a (mechanism + telemetry) · **Size:** M · **Depends on:** 04, 05 · **Enables:** 13 (session persistence), P1 budget guardrail, 15 (loop budgets)

## Goal

Build the mechanism by which the operator auto-injects flokoa-owned capabilities into every compiled spec (brief §3), and ship the first one: **telemetry**. Session-persistence and budget-guardrail capabilities reuse this mechanism in P1 (13/14) — this unit makes them pure capability work, no new plumbing.

## Design

### Mechanism (compiler side, extends 04 step 3)

- Injected entries are ordinary capability entries in the compiled AgentSpec, namespaced `flokoa.platform/<name>@<version>`, appended **after** user capabilities (deterministic position; users cannot shadow or reorder them).
- Implementations live **in the runner baseline** (`flokoa_runner.platform_capabilities`), not in OCI artifacts — they version with the runner, are part of the contract (03 manifest lists them), and cannot be absent. They are still real `AbstractCapability` subclasses using only the stable core API (tools/instructions/model-settings contributions + lifecycle hooks).
- Configuration comes from the operator: a `platform` block in the compiled spec's injected entries, populated from Helm values (cluster policy) + Agent fields where the brief defines them (budgets later). Users see them in the *resolved* spec (status surfaces the injected list) but not in their CR — transparency without editability.
- Opt-out is **cluster policy only** (Helm value per capability, e.g. air-gapped clusters disabling telemetry export) — never per-Agent, or budget enforcement (P1) becomes advisory again.

### First capability: `flokoa.platform/telemetry`

Subsumes the old observability spec (08-old) at the right layer:

- On construction: ensures OTel tracer/meter providers (reusing `flokoa/telemetry.py` init, OTLP iff `OTEL_EXPORTER_OTLP_ENDPOINT`), enables pydantic-ai instrumentation (GenAI semconv spans incl. token attributes).
- Via hooks: wraps each run in a `flokoa.agent.invoke` span (agent name, contextId, task id); records metrics — `flokoa.agent.requests` (outcome), `flokoa.agent.request.duration`, `gen_ai.client.token.usage` (input/output, model) from run usage; `on_*_error` hooks record failure outcomes.
- Operator side (small, with 04's builder work): inject `OTEL_SERVICE_NAME=<agent>`, `OTEL_RESOURCE_ATTRIBUTES=k8s.namespace.name=…,flokoa.agent.name=…`, endpoint from Helm `telemetry.otlpEndpoint`. User env wins on conflict.

### Stubs registered now (implemented in P1)

`flokoa.platform/session-persistence` and `flokoa.platform/budget-guardrail` get reserved names + config schemas in the contract doc, so 13/14 add implementations without mechanism changes.

## Implementation plan

1. Compiler injection step + status surfacing of injected entries + Helm policy values.
2. `platform_capabilities` package in the runner with the capability base wiring + telemetry implementation (hook names verified against the pinned pydantic-ai capability API).
3. Contract doc section: injected-capability list, naming, config source, opt-out policy.
4. Wire 04's golden specs to include injected entries (they're part of the resolved spec hash — cluster policy changes legitimately roll agents).

## Testing

- Python: telemetry capability against `InMemorySpanExporter`/`InMemoryMetricReader` — invoke span, token histogram after a TestModel run, error-path outcomes; capability inactive (no exporter) when no endpoint.
- Go: injection ordering property (always after user entries), Helm-value plumbing, opt-out.
- E2E: one invocation renders as a coherent trace (server → runner → model spans) with per-agent service name; token series queryable.

## Acceptance criteria

- Every agent on the generic runner emits traces + GenAI token metrics with zero user configuration beyond a cluster-level collector endpoint; injected entries are visible in resolved-spec status; per-Agent opt-out is impossible.

## Out of scope

- Session-persistence and budget-guardrail implementations (13/P1). Collector deployment (17). Dashboards (17).
