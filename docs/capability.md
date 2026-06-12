# Capability

A `Capability` is a **versioned, digest-pinned, schema-published unit of agent
behavior** (product brief §4): a pydantic-ai capability implementation packaged
as an OCI wheelhouse artifact, mirrored into a CR so admission can
machine-check the compatibility matrix — config-schema validation,
`requires`-tuple checks, and dependency-conflict detection — **before**
anything deploys, offline and air-gap-friendly.

Native capabilities (WebSearch, MCP, Thinking, …) don't need a Capability CR —
they live in the runner baseline and go directly in an Agent's inline spec
fragment. Capability CRs are for **harness and third-party** implementations,
whose 0.x churn is absorbed by digest pinning: a breaking change is a new
artifact version with a new config schema; running agents keep their pinned
digest and never break uninvited.

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Capability
metadata:
  name: kb-search
spec:
  artifact: ghcr.io/danielnyari/capabilities/kb-search@sha256:4f5d…  # digest-pinned, always
  version: 0.1.0
  entrypoint: flokoa_kb_search.capability:KBSearch
  schemaPolicy: strict
  configSchema:
    type: object
    required: [endpoint]
    properties:
      endpoint: {type: string, pattern: "^https://"}
      apiKey: {type: string}
      maxResults: {type: integer, default: 5}
    additionalProperties: false
  requires:
    python: "3.13"
    pydanticAI: ">=1.107,<2"
    flokoaRunner: ">=0.2"
  dependencies:
    - kb-search-client==1.4.2
```

Agents attach it with per-agent config:

```yaml
spec:
  capabilities:
    - ref: {name: kb-search}
      config:
        endpoint: https://kb.example.com
        apiKey: ${secret:kb-api-key}   # needs a matching spec.secretRefs entry
```

## Spec fields

| Field | Description |
|---|---|
| `artifact` | OCI reference of the wheelhouse artifact image. **Must be digest-pinned** (`…@sha256:<64 hex>`); admission rejects tags. |
| `version` | The capability's own semantic version (matches the artifact manifest). |
| `entrypoint` | Python `module:attr` resolving to the capability class (a pydantic-ai `AbstractCapability` subclass). |
| `serializationName` | The capability's spec-entry name when the class overrides pydantic-ai's default (the class name). Defaults to the `attr` part of `entrypoint`. |
| `configSchema` | JSON Schema for per-agent config. Required under `schemaPolicy: strict`. |
| `schemaPolicy` | `strict` (default) or `permissive` — the **loud opt-out**: config skips validation, and the CR is flagged in status, admission warnings, and printcolumns. |
| `requires` | Compatibility tuple mirrored from the artifact manifest: `python` (exact minor), `pydanticAI` and `flokoaRunner` (PEP 440 specifier sets). |
| `dependencies` | The artifact's pinned dependency closure (`name==version`), mirrored for offline conflict detection. |
| `provenance` | Signature/attestation metadata (verification mechanics land with artifact delivery, roadmap 09). |

The spec mirrors the artifact manifest **by value** so admission never fetches
from a registry. `flokoa capability push` (roadmap 10) generates the CR from
the manifest, so the mirror never drifts in practice.

## What admission checks

Misconfigured capability config, an incompatible runner, or conflicting
dependencies **cannot reach a pod** — each fails Agent admission with a
message naming the offending refs and versions:

1. **Config-schema validation** — each attachment's `config` validates against
   the capability's `configSchema`. `${secret:NAME}` placeholders satisfy
   string-level constraints (shape is validated at admission; values resolve
   in the runner) but never non-string types. Permissive capabilities skip
   this with an admission **warning**.
2. **Requires check** — the `requires` tuple is evaluated against the Agent's
   resolved runner version (the operator embeds each supported runner's
   baseline). Incompatible attachments are denied naming both tuples, e.g.
   `capability "kb-search" requires pydantic-ai ">=2,<3" but runner 0.2.0
   provides pydantic-ai "1.107.0"`.
3. **Dependency-conflict detection** — `dependencies` are unioned across all
   attached capabilities plus the runner baseline lockfile. Two capabilities
   pinning different `pydantic-ai-harness` versions is the canonical rejected
   case — *correct behavior, not a limitation*: one pod, one Python
   environment.

A referenced Capability that doesn't exist yet only warns at admission (the
same ordering-tolerant pattern as Model/Instruction/AgentTool references); the
compiler re-runs all three checks when the CR appears, so the guarantee holds
regardless of creation order — a bad composition surfaces as
`SpecValid=False`, never as a broken pod.

## How it compiles

Each attachment becomes a capability entry in the compiled spec, named by
`serializationName` (default: the entrypoint's class name), placed after the
fragment's and AgentTools' entries and before the platform-injected block:

```yaml
capabilities:
  - KBSearch:
      endpoint: https://kb.example.com
      apiKey: ${secret:kb-api-key}
  - flokoa.platform/telemetry
```

The runner resolves the entry against the classes installed from the
capability's wheelhouse (runtime contract §4).

## Status

`kubectl get capabilities` (short name: `cap`) shows version, runner range,
and schema policy. Conditions:

- `Permissive` — `True` with a loud message when `schemaPolicy: permissive`.
- `Verified` — artifact digest/signature verification; stays `Unknown` until
  controller-side verification ships with delivery (roadmap 09). Admission
  already enforces the digest pin itself.

## Current limits (roadmap 09/10)

Artifact **delivery is not wired yet**: attaching a Capability compiles and
validates, but the runner pod does not yet receive the wheelhouse
(initContainer/ImageVolume delivery is roadmap 09), so the entry cannot
hydrate at bootstrap. The `flokoa capability build/push/import/search` CLI and
registry seeding are roadmap 10. Until those land, Capability CRs are the
admission-checked control surface their consumers build on.
