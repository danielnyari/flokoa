# 10 — Capability CLI & Registry Seeding

**Phase:** P0b · **Size:** L · **Depends on:** 08, 09 · **Enables:** the marketplace; P0b exit

## Goal

`flokoa capability build | push | import | search` — the tooling that makes publishing a capability boring and importing the ecosystem one command. Then use it: **seed the registry with the harness capabilities + endorsed community packages** (brief §4: seeding is in-scope P0b work; publish friction is the pillar's success metric).

## CLI design (extends the Click CLI in `flokoa/src/flokoa/`)

### `flokoa capability build [PATH] [--tag REF]`

- Input: a Python project (pyproject with the capability) or `--from-pypi <package>==<version>` (used by `import`).
- **Builds inside the pinned runner image** (docker run the runner image, `pip wheel` the package + non-baseline dep closure against the baseline lockfile as constraints) — the compatibility matrix is satisfied *by construction* (brief §4). Non-baseline closure = full resolution minus packages already pinned in `runner.lock`.
- **Smoke test**: in the same container, install from the wheelhouse and import the entrypoint; instantiate with `{}` or schema defaults where possible. A capability that can't import never gets an artifact.
- **Schema derivation**: introspect the entrypoint — pydantic models / dataclasses / typed `__init__` or `from_spec` signatures → JSON Schema (pydantic's own `TypeAdapter(...).json_schema()` machinery, run inside the runner image where the package imports). Untypeable → require `--schema file.json` or explicit `--permissive` (refused without the flag; the flag prints the loud warning).
- Output: artifact image (09 layout, multi-arch via buildx), `manifest.json`, and a **generated Capability CR YAML** (the 08 mirror — name, version, digest placeholder filled on push, schema, requires, deps).

### `flokoa capability push REF [--sign]`

Pushes the artifact (digest recorded), optionally cosign-signs, rewrites the generated CR with the final digest, optionally `kubectl apply`s it (`--apply`). Also appends to the index (below).

### `flokoa capability import <pypi-package>[==version]`

`build --from-pypi` + interactive schema review (show derived schema; confirm/permissive) + `push`. The promise: *any `pydantic-ai-<name>` package on PyPI is one command from being a flokoa capability.* Where a distribution exposes multiple capability classes, `--entrypoint` selects (default: heuristic over exported `AbstractCapability` subclasses, listed for choice).

### `flokoa capability search [QUERY]` / `list`

v1 index is deliberately boring: a JSON index file in a git repo / OCI artifact (`flokoa-capability-index`), fetched and grepped client-side; `search` also lists in-cluster Capability CRs. A hosted index is a later investment, not P0b.

## Seeding (deliverable, not an aspiration)

- Package and publish under `ghcr.io/danielnyari/capabilities/` (namespace TBD with Dani): **all pydantic-ai-harness capabilities** (file system, shell, CodeMode, context management, memory/persistence/checkpointing, sub-agents, skills, planning, guardrails — one artifact per capability or per coherent group; grouping decided by harness's own packaging extras), plus endorsed community packages (vstorm-co shields/deepagents/subagents/summarization/todo; DougTrajano skills).
- Each seed gets: derived-or-authored schema (no permissive seeds — we own them, we type them), smoke test, signed artifact, index entry, and a docs example attaching it to an Agent.
- Seeding doubles as the tooling's acceptance test: every friction found while seeding ~15 capabilities is a bug in `build`/`import`.

## Implementation plan

1. CLI scaffolding (`capability` Click group; container execution helper — docker/podman detection, runner image resolution from the contract).
2. `build`: wheelhouse resolution with baseline constraints + smoke test + schema derivation (the introspection module runs *inside* the runner; CLI orchestrates).
3. `push` (ORAS/registry client or shell-out to crane — pick `crane` for v1 simplicity, vendored binary check) + cosign integration + CR generation.
4. `import` flow + entrypoint heuristics.
5. Index format + `search`.
6. Seed batch + examples + docs (`docs/guides/capabilities.md`: authoring, importing, publishing).

## Testing

- Unit: schema derivation table (pydantic model / dataclass / typed init / untyped → require flag); constraint-resolution against a fixture baseline; CR generation goldens.
- Integration (CI with docker): `build` a fixture capability end-to-end → artifact + CR; `import` a real small pydantic-ai community package; smoke-test failure path (unimportable package refused).
- The seed batch itself running through CI is the system test.

## Acceptance criteria

- A maintainer publishes a new capability — typed schema, signed, indexed — in under five minutes with `build` + `push`.
- `import` turns a real PyPI harness-ecosystem package into an attachable Capability CR without editing JSON by hand.
- ≥ harness-complete seed set published and each attachable on a Kind cluster.

## Out of scope

- Hosted registry/index service. Web UI. Capability *runtime* behavior (08/09). Monetization/namespacing policy for third-party publishers (later governance doc).
