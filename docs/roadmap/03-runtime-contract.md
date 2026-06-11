# 03 — The Runtime Contract (Keystone)

**Phase:** P0a, first · **Size:** M · **Depends on:** 02 · **Enables:** 04, 05, 08, 09 — every other unit builds against this

## Goal

Define and ship the versioned contract between the control plane and the runner: the pinned runtime environment ("the published lockfile is the platform"), the AgentSpec JSON Schema for that pin, and the file/env interface. Brief §4 calls this the keystone — it is written and merged **before** the compiler (04) and runner (05) are built against it.

## Deliverables

### 1. Contract document: `docs/reference/runtime-contract.md`

Normative, versioned (`contractVersion: 1`). Sections: pinned environment, interface table, secret-placeholder grammar, capability mount layout, compatibility/`requires` semantics, change policy (additive within a runner major; any change is a PR-blocking review item; the contract version appears in the runner image label and capability manifests).

### 2. Pinned runner environment (the platform)

- Definition lives in `sdk/python/flokoa-runner/` (the renamed/refactored `flokoa-managed-agent` — rename happens in 05; this unit creates the pinning machinery): `pyproject.toml` + **exported lockfile** `runner.lock` (uv export) published as a release artifact and baked into the image at `/etc/flokoa/runner-manifest.json`:
  ```json
  {
    "contractVersion": 1,
    "runnerVersion": "0.2.0",
    "python": "3.13",
    "pydantic-ai": "1.x.y",
    "baseline": {"httpx": "…", "starlette": "…", "pydantic": "…", "opentelemetry-sdk": "…"},
    "agentSpecSchemaDigest": "sha256:…"
  }
  ```
- Baseline = Python minor + pydantic-ai core (Capabilities API + native capabilities: WebSearch, WebFetch, MCP, Thinking, ToolSearch, …) + httpx/starlette/pydantic/opentelemetry. **pydantic-ai-harness is explicitly NOT baseline** (brief §4 taxonomy) — assert this with a CI check that the lockfile contains no harness packages.
- One runner version per flokoa release; support window ~2 concurrent versions. `make runner-manifest` generates the manifest from the lockfile; CI verifies image ↔ manifest ↔ lockfile agreement.

### 3. AgentSpec JSON Schema extraction

- A small generator (`hack/gen-agentspec-schema.py`, runs **inside the runner image** so the schema matches the pin exactly): emits the AgentSpec JSON Schema via pydantic-ai's own spec-schema support (AgentSpec ships schema generation for editor autocompletion — use that API; verify the exact call on the pinned version, e.g. the schema companion emitted by `AgentSpec.to_file`).
- Output committed to `operator/internal/spec/schemas/agentspec-<runnerVersion>.json` and embedded in the operator binary (`go:embed`) — the Go compiler (04) validates compiled specs against it with no Python in the control plane.
- The schema digest is recorded in the runner manifest; the operator refuses to pair a runner image whose manifest digest doesn't match an embedded schema (clear `SpecValid=False` reason), which is how runner-version skew becomes a loud condition instead of a runtime surprise.

### 4. Interface table (normative)

| Channel | Path / variable | Direction |
|---|---|---|
| Compiled spec | `/etc/flokoa/agent-spec.yaml` (ConfigMap; placeholders intact) | operator → runner |
| Capability wheelhouses | `/opt/flokoa/capabilities/<name>/` (wheels + `manifest.json`) | delivery (09) → runner |
| Secret indirection | `${secret:<ref>}` in spec ↔ `FLOKOA_SECRET_<NORMALIZED_REF>` env via `valueFrom.secretKeyRef` | operator → runner, resolved at hydration |
| Serving | `FLOKOA_HOST`, `FLOKOA_PORT`, `FLOKOA_PUBLIC_URL` (existing) | operator → runner |
| Telemetry | `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES` | operator → runner |
| Runner identity | `/etc/flokoa/runner-manifest.json` | image → operator/CLI introspection |

Placeholder grammar: `${secret:NAME}` where `NAME` matches a `secretRefs[NAME] → SecretKeySelector` entry on the Agent CR (04). Normalization rule (uppercase, non-alphanumeric → `_`) is specified here once and shared by compiler and runner.

### 5. Legacy-path deprecation schedule

The existing channels (`template-config.json`, `agent-config.json`, `instruction.txt`, `model.json`, `tools/*/spec.json`, `agent-card.json`) are documented as **deprecated, removed when 04+05 ship**. Until then the current managed agent keeps working unchanged — 04/05 cut over atomically, no dual-stack period in user clusters (the operator version determines which contract it speaks; runner images support exactly one).

## Implementation plan

1. Write the contract doc with the tables above; circulate as the PR description's core.
2. Pin + lockfile machinery in the runner package; `runner-manifest.json` bake + CI consistency check + no-harness-in-baseline check.
3. Schema generator + embedded schemas + Go loader (`operator/internal/spec/`: `LoadSchema(runnerVersion)`, `Validate(doc) error` using `santhosh-tekuri/jsonschema/v6`); table-driven tests with valid/invalid AgentSpec documents captured from the real pydantic-ai version (golden files generated inside the runner image, so Go tests don't need Python).
4. Wire `make` targets: `runner-lock`, `runner-manifest`, `gen-agentspec-schema`; document the runner-release procedure (bump pin → regenerate lock/manifest/schema → CI gate).

## Acceptance criteria

- A single command regenerates lockfile + manifest + schema coherently; CI fails on any drift between the three.
- The operator binary can validate a known-good AgentSpec document and reject a malformed one with a precise error path, for the pinned runner version.
- The contract doc answers, without reading code: "what Python/pydantic-ai do I get, what files appear where, how do secrets resolve, what happens on version skew."

## Out of scope

- The compiler itself (04), runner bootstrap (05), capability manifest *contents* beyond the `requires`/contract fields (08/09).
