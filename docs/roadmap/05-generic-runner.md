# 05 — The Generic Runner

**Phase:** P0a · **Size:** L · **Depends on:** 03 · **Co-delivered with:** 04 · **Enables:** 07, 09

## Goal

Turn `flokoa-managed-agent` into the **generic runner** of brief §3: hydrate the compiled AgentSpec, resolve secret placeholders, install capability wheelhouses, construct the agent from the spec, serve A2A. Most users never build a container. Narrow the Python SDK to its post-pivot scope (brief §7).

## Current state

- `flokoa-managed-agent/__main__.py`: loads unified `agent-config.json` (`LlmAgentConfig`) or legacy multi-file config, builds a pydantic-ai Agent via `TemplatedAgentBuilder`/`build_agent_from_config`, serves via a2a-sdk (`A2AFastAPIApplication` + `DefaultRequestHandler` + `InMemoryTaskStore`), executor `TemplatedPydanticAIAgentExecutor` ← `PydanticAIAgentExecutor` ← `FlokoaAgentExecutor` (ConfigCache-driven tool/model/instruction reload).
- `flokoa` SDK: CLI `run`, integrations registry (deleted in 02), OpenAPI toolset machinery, telemetry helpers, agent-card builder, `ConfigCache`.
- Server-side machinery that the runner must keep compatible: playground bridge, trigger invoke + push-notification delivery (`push_gateway.go`), e2e A2A tests (`sdk/python/tests/e2e/`).

## Target design

### Package: `flokoa-runner` (rename of `flokoa-managed-agent`)

Bootstrap pipeline (each step a function; the pipeline is the package's public story):

```
load_manifest()            # /etc/flokoa/runner-manifest.json — identity, pins (03)
load_compiled_spec()       # /etc/flokoa/agent-spec.yaml
resolve_secrets(spec)      # ${secret:NAME} → FLOKOA_SECRET_<NORM> env; fail fast,
                           #   listing ALL missing refs at once; values never logged
install_capabilities()     # for each /opt/flokoa/capabilities/<name>/:
                           #   verify manifest requires-tuple against runner manifest (defense in
                           #   depth — admission already checked), then
                           #   pip install --no-index --find-links <dir> <pinned wheel>
build_agent(spec)          # pydantic-ai AgentSpec → Agent (Agent.from_spec / constructor with
                           #   spec= on the pinned version — verify exact API at implementation;
                           #   capability entries resolve entrypoints module:attr for Capability-CR
                           #   capabilities, native names for baseline ones)
serve(agent)               # A2A app + health + telemetry (07)
```

- **Fail-fast with structured errors**: every bootstrap failure exits non-zero with a single-line JSON error (`{"stage": "resolve_secrets", "missing": [...]}`) so `kubectl logs` and the controller's pod-failure surfacing are actionable. Bad *spec* shouldn't reach here (04 validates) — bootstrap failures are environment problems and must say so.
- **A2A serving: keep the a2a-sdk stack** (executor + `A2AFastAPIApplication`) for this unit — the playground, push gateway, and e2e tests speak it today, and the executor remains our seam for the injected-capability wiring (07) and router handshake (12). **Tracked decision** `docs/decisions/a2a-stack.md`: migrate to FastA2A/`to_a2a()` iff it can replace the executor seam without losing task events/push compatibility — evaluate after 07 lands, not during this unit.
- **Hot reload narrows by design**: the compiled spec is immutable per pod generation (spec changes roll the Deployment via 04's hash annotation). Keep ConfigCache only if the card stays file-mounted; delete tool/model/instruction reload paths with the legacy contract. Simpler runner, deterministic pods.
- The executor simplifies: model/tools/instructions all come from the constructed Agent (spec-defined); the legacy per-request `run_kwargs` overrides (model/instructions/toolsets from mounted files) are deleted with the legacy contract.

### SDK narrowing (`flokoa` package)

Keeps: `flokoa run -m module:agent` (local dev, A2A serving for BYO agents — now also `flokoa run -f agentspec.yaml` to serve a spec file locally, the local mirror of the runner path), telemetry helpers, context helpers (`flokoa.context`: agent name/namespace/session id accessors for capability authors), platform-capability implementations (07). Deletes: OpenAPI toolset machinery (`tools/openapi/`, with `flokoa-common`'s parser absorbed or deleted per its only remaining consumer) — **retired in favor of MCP** per 04's AgentTool repositioning; migration note in docs. `flokoa-types` regenerates from the new CRDs; hand-maintained wrappers (`ToolDefinition` etc.) shrink accordingly.

### Image

`flokoa-runner` image (replaces the `flokoa-cli` image's runtime role; release matrix updated): runner package + baseline lockfile, `pip`/`uv` available for wheelhouse installs, non-root, manifest at `/etc/flokoa/runner-manifest.json`, labeled with `runnerVersion`/`contractVersion`. The `DefaultTemplateRuntimeImage` constant in `internal/infra/builder/deployment.go` repoints here (ldflags-versioned per existing release machinery).

## Implementation plan

1. Rename/restructure package; bootstrap pipeline with per-stage unit tests (tmp-dir fixtures for spec/manifest/wheelhouse layouts).
2. Secret grammar implementation shared-tested against 03's normalization rule (same table as the Go compiler's env emission — golden pairs).
3. `build_agent` against the pinned pydantic-ai (verify `from_spec` API; capability entrypoint loading with actionable ImportError naming the Capability CR and artifact).
4. Serve + health + executor simplification; keep e2e green by cutting over with 04 in one release train.
5. SDK deletions + `flokoa run -f` + context helpers; docs migration notes (OpenAPI→MCP).
6. Image + release matrix + builder repoint.

## Testing

- Stage unit tests incl. all-missing-secrets aggregation and wheelhouse `requires` mismatch.
- **Contract test (binds 03/04/05)**: 04's golden compiled specs hydrate to a working Agent inside the real runner image (container test in CI), answer a TestModel run, serve a card.
- E2E (Kind): Agent CR (composition from 04) → runner pod → A2A invocation → playground and trigger-invoke paths still pass.

## Acceptance criteria

- A composed Agent runs on the generic runner with zero user-built images; `kubectl logs` on any misconfiguration names the failing stage and refs.
- `flokoa run -f spec.yaml` serves the same spec locally that the cluster runs.
- No OpenAPI tool code remains; e2e suite green on the new contract only.

## Out of scope

- Capability artifact *delivery* (09 — this unit consumes `/opt/flokoa/capabilities/` as given, using a test fixture layout). Platform capability implementations (07). FastA2A migration (tracked decision).
