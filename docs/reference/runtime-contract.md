# The Runtime Contract

**Status: normative.** `contractVersion: 1`

This document defines the versioned contract between the flokoa control plane
(operator + spec compiler) and the generic runner (`flokoa-runner`). Every
other part of the platform builds against it: the compiler emits what this
document says, the runner consumes exactly that, capability artifacts declare
compatibility against it. Changes are **additive within a runner major
version**, and any change to this contract is a PR-blocking review item.

## 1. The pinned environment ("the published lockfile is the platform")

Each flokoa release pins **one runner version**. A runner version fixes:

- the Python minor version,
- the **pydantic-ai core** version (which includes the stable Capabilities API
  and the native capabilities: WebSearch, WebFetch, MCP, Thinking, ToolSearch, …),
- the baseline libraries (httpx, starlette, pydantic, opentelemetry-sdk).

The pin lives in `sdk/python/flokoa-runner/pyproject.toml`; the exported
lockfile `sdk/python/flokoa-runner/runner.lock` is published as a release
artifact and **is the platform**: capability artifacts may rely on exactly
these versions being present and on nothing else.

`pydantic-ai-harness` is deliberately **not** part of the baseline. Harness
and third-party capability implementations ship exclusively as versioned,
digest-pinned Capability artifacts (product brief §4); CI fails if a harness
package ever appears in `runner.lock`.

One global runner version per flokoa release, with `spec.runtime.runnerVersion`
on the Agent CR as the per-agent escape hatch. The support window is ~2
concurrent runner versions; the operator embeds one AgentSpec schema per
supported version.

### The runner manifest

Every runner image carries its identity at `/etc/flokoa/runner-manifest.json`:

```json
{
  "contractVersion": 1,
  "runnerVersion": "0.2.0",
  "python": "3.13",
  "pydantic-ai": "1.107.0",
  "baseline": {"httpx": "…", "starlette": "…", "pydantic": "…", "opentelemetry-sdk": "…"},
  "platformCapabilities": {"flokoa.platform/telemetry": "0.2.0", "…": "…"},
  "agentSpecSchemaDigest": "sha256:…"
}
```

The manifest is generated from the lockfile (`make runner-contract` in
`sdk/python/`); CI verifies lockfile ↔ manifest ↔ schema agreement. The image
is labeled with `ai.flokoa.runner-version` and `ai.flokoa.contract-version`.

### The AgentSpec JSON Schema

The runner release ships the JSON Schema of pydantic-ai's `AgentSpec` for its
pinned version, generated **inside the runner environment**
(`hack/gen_agentspec_schema.py`) with flokoa's platform capabilities included,
and committed to `operator/internal/spec/schemas/agentspec-<runnerVersion>.json`.
The operator embeds these schemas (`go:embed`) and validates every compiled
spec against the schema for the agent's runner version — no Python in the
control plane. The schema's sha256 digest is recorded in the runner manifest.

**Version-skew behavior:** if an Agent pins a `runnerVersion` with no embedded
schema, compilation fails with a `SpecValid=False` condition (no Deployment
update happens; the last good generation keeps running). At bootstrap the
runner compares its manifest against the expectations the operator delivered
(`FLOKOA_EXPECTED_RUNNER_VERSION`, `FLOKOA_EXPECTED_SCHEMA_DIGEST`) and exits
with a structured error on mismatch — skew is a loud condition on both sides,
never a runtime surprise.

## 2. The interface (normative)

| Channel | Path / variable | Direction |
|---|---|---|
| Compiled spec | `/etc/flokoa/agent-spec.yaml` (ConfigMap-mounted; secret placeholders intact) | operator → runner |
| Agent card | `/etc/flokoa/agent-card.json` (same ConfigMap, second key) | operator → runner |
| Runner identity | `/etc/flokoa/runner-manifest.json` (baked into the image) | image → operator/CLI introspection |
| Capability wheelhouses | `/opt/flokoa/capabilities/<name>/` (wheels + `manifest.json`) | delivery → runner |
| Secret indirection | `${secret:NAME}` placeholders in the spec ↔ `FLOKOA_SECRET_<NORMALIZED_NAME>` env (`valueFrom.secretKeyRef`) | operator → runner, resolved at hydration |
| Serving | `FLOKOA_HOST` (default `0.0.0.0`), `FLOKOA_PORT` (default `8080`), `FLOKOA_PUBLIC_URL` (the published endpoint, = `status.url`) | operator → runner |
| Skew detection | `FLOKOA_EXPECTED_RUNNER_VERSION`, `FLOKOA_EXPECTED_SCHEMA_DIGEST` | operator → runner |
| Telemetry | `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES` | operator → runner |

The spec file is a YAML document conforming to the pinned pydantic-ai
`AgentSpec` schema. The runner hydrates it with
`Agent.from_spec(spec, custom_capability_types=…)`, where the custom types are
the platform capabilities plus any entrypoints from installed capability
wheelhouses.

### Bootstrap pipeline and failure semantics

The runner bootstraps in fixed stages: `load_manifest → load_compiled_spec →
resolve_secrets → install_capabilities → build_agent → serve`. Every bootstrap
failure exits non-zero with a single-line JSON error on stderr naming the
stage, e.g.:

```json
{"stage": "resolve_secrets", "error": "missing secret refs", "missing": ["api-token", "db-dsn"]}
```

A schema-invalid spec should never reach the runner (the compiler validates
first); bootstrap failures are environment problems and must read as such.
Secret values are never logged.

## 3. Secret-placeholder grammar

A placeholder is exactly `${secret:NAME}` where `NAME` matches
`[A-Za-z0-9._-]+` and names either a key in the Agent CR's
`spec.secretRefs` map or a compiler-derived reference (e.g. AgentTool header
secrets, named `tool-<tool>-<header>`). Placeholders may appear in any string
value of the compiled spec. They are resolved **in the runner at hydration
time** from environment variables projected by the operator via
`valueFrom.secretKeyRef` — secret values never appear in any ConfigMap, CR,
or compiled artifact.

**Normalization rule** (shared by the Go compiler's env emission and the
runner's resolution; golden-pair tested on both sides): uppercase the name and
replace every character outside `[A-Z0-9]` with `_`, then prefix
`FLOKOA_SECRET_`. Examples:

| Placeholder | Environment variable |
|---|---|
| `${secret:api-token}` | `FLOKOA_SECRET_API_TOKEN` |
| `${secret:db.dsn}` | `FLOKOA_SECRET_DB_DSN` |
| `${secret:KbToken}` | `FLOKOA_SECRET_KBTOKEN` |

Resolution is all-or-nothing: the runner collects **all** missing references
and fails the `resolve_secrets` stage listing every one of them at once.

## 4. Capability wheelhouse layout

Each Capability artifact is delivered (initContainer copy or ImageVolume
mount) to `/opt/flokoa/capabilities/<name>/` containing:

- the capability wheel plus the pinned closure of dependencies **not** in the
  baseline,
- `manifest.json` with at minimum:

```json
{
  "name": "…", "version": "…",
  "entrypoint": "module:attr",
  "requires": {"python": "3.13", "pydantic-ai": ">=1.107,<2", "flokoa-runner": ">=0.2"},
  "contractVersion": 1
}
```

The runner verifies the `requires` tuple against its own manifest (defense in
depth — admission already checked it), then installs with
`pip install --no-index --find-links <dir>` and registers the entrypoint class
in `custom_capability_types`. System-level dependencies (binaries, apt
packages) are out of scope for artifacts — that's what custom images are for.

## 5. `requires` / compatibility semantics

A capability's `requires` tuple is checked against the runner manifest:
`python` (exact minor), `pydantic-ai` (PEP 440 specifier), `flokoa-runner`
(PEP 440 specifier). The webhook refuses incompatible attachments at
admission; the runner refuses them at install. The compatibility matrix is
one-dimensional by construction: one runner version per release.

## 6. Platform-injected capabilities

The operator appends flokoa-owned capability entries to every compiled spec,
**after** all user entries (deterministic position — users cannot shadow or
reorder them). Their implementations live in the runner baseline
(`flokoa_runner.platform_capabilities`), version with the runner, and are
listed in the runner manifest. Injected entries are visible in the resolved
spec and surfaced in Agent status (`status.injectedCapabilities`) —
transparency without editability. Opt-out is cluster policy only (Helm
values), never per-Agent.

Reserved names under contract v1:

| Name | Status | Config (operator-populated) |
|---|---|---|
| `flokoa.platform/telemetry` | shipped | `{}` (endpoint/identity flow via `OTEL_*` env) |
| `flokoa.platform/session-persistence` | reserved (P1, roadmap 13) | `{dsn: "${secret:…}", ttlSeconds: int, maxTurns: int}` |
| `flokoa.platform/budget-guardrail` | reserved (P1, roadmap 14) | `{maxTokens: int, maxRuns: int, action: "block"}` |

## 7. The legacy channels (removed)

The pre-contract channels — `template-config.json`, `agent-config.json`,
`instruction.txt`, `model.json`, `tools/*/spec.json` as separate mounts, and
the multi-ConfigMap layout — are **removed** as of this contract. The operator
version determines which contract it speaks; runner images support exactly
one. There is no dual-stack period: operator and runner cut over atomically in
one release train.

## 8. Change policy

- The contract version appears in the runner manifest, the runner image
  labels, and every capability artifact manifest.
- Within a runner major version, changes must be additive (new env vars, new
  optional manifest fields, new reserved capability names).
- Any change to this document is a PR-blocking review item and requires
  regenerating the contract artifacts (`make runner-contract`) in the same PR.
- Runner release procedure: bump the pin in `flokoa-runner/pyproject.toml` →
  `make runner-contract` → commit lockfile + manifest + schema → CI gates on
  drift (`make verify-runner-contract`) and on the no-harness rule.
