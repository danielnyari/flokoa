# Authoring, building, and publishing capabilities

This guide walks through turning a pydantic-ai capability into a published,
attachable Flokoa [`Capability`](../capability.md) — authoring the project,
building the artifact with `flokoa capability build`, publishing it with
`flokoa capability push`, importing one straight from PyPI with `flokoa
capability import`, and finding what is already published with `flokoa
capability search`.

For the `Capability` CR field reference and what admission checks, see
[the Capability reference](../capability.md). For the on-disk artifact format
and how the runner consumes a wheelhouse, see the
[runtime contract §4](../reference/runtime-contract.md#4-capability-artifacts-and-the-wheelhouse-layout).

## What a capability artifact is

A capability artifact is a pydantic-ai capability implementation packaged as an
**OCI wheelhouse image**: a tiny `busybox` base carrying a `/wheelhouse/`
directory of the capability's own wheel plus the pinned closure of any
non-baseline dependencies, with a self-describing `manifest.json` alongside.
The operator delivers it into runner pods (initContainer copy or ImageVolume
mount), the runner installs it offline (`pip install --no-index --find-links`)
and registers the entrypoint class. Each artifact is mirrored into a
digest-pinned `Capability` CR so admission can machine-check the compatibility
matrix — config schema, `requires` tuple, dependency conflicts — before
anything reaches a pod. The artifact format is normative in
[runtime contract §4](../reference/runtime-contract.md#4-capability-artifacts-and-the-wheelhouse-layout).

## Prerequisites

The CLI orchestrates a container build and registry pushes; it shells out to
the tools you already use rather than vendoring binaries into the wheel:

| Tool | Needed by | Notes |
|---|---|---|
| `docker` or `podman` | `build`, `import` | The build runs **inside the pinned runner image**. `CONTAINER_TOOL` overrides detection (docker preferred). `podman` builds one platform per image; multi-arch needs `docker buildx`. |
| `crane` | `push`, `import` | Pushes the OCI-layout tarball and records the digest. `FLOKOA_CRANE` overrides the binary. Install: `brew install crane`. |
| `cosign` | `push --sign`, `import --sign` | Signs the pushed digest. `FLOKOA_COSIGN` overrides. Install: `brew install cosign`. Only needed when you sign. |
| `kubectl` | `push --apply`, `search --cluster` | Applies the pinned CR and merges in-cluster Capability CRs. Skipped gracefully by `search` when absent. |
| The runner image | `build`, `import` | Resolved as `ghcr.io/danielnyari/flokoa-runner:<version>` (`--runner-version`/`--runner-image`/`FLOKOA_RUNNER_IMAGE` override). The build wheels against this exact environment, so the artifact's compatibility is satisfied by construction. |

Each command preflights only the binaries its requested options actually need
and fails up front with an install one-liner if one is missing — `build`
without `--apply` never asks for `kubectl`.

> The build container runs the package's own code during the smoke test, so it
> always runs inside a disposable container, never on your host interpreter.

## Authoring a capability

A capability is a Python project whose distribution exports exactly one
concrete pydantic-ai
[`AbstractCapability`](../reference/runtime-contract.md#4-capability-artifacts-and-the-wheelhouse-layout)
subclass. The worked example throughout this guide is the `echo` fixture at
`operator/test/e2e/fixtures/capabilities/echo/` — the smallest real capability:
one config field, one tool, zero non-baseline dependencies (so the wheelhouse
holds exactly one wheel).

### Project shape

```
echo/
├── pyproject.toml
└── src/
    └── flokoa_cap_echo/
        └── __init__.py
```

`pyproject.toml` is an ordinary build-backend project; `pydantic-ai` is in the
runner baseline, so you depend on it but it is **not** shipped in the
wheelhouse:

```toml
[project]
name = "flokoa-cap-echo"
version = "0.1.0"
description = "Reference fixture: the smallest real capability (zero non-baseline deps)"
requires-python = ">=3.13"
# pydantic-ai is in the runner baseline — the wheelhouse for this project
# therefore holds exactly one wheel: the capability itself.
dependencies = ["pydantic-ai>=1.107,<2"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[tool.hatch.build.targets.wheel]
packages = ["src/flokoa_cap_echo"]
```

### The AbstractCapability subclass

The capability class is a pydantic-ai `AbstractCapability` subclass that
returns a toolset. Type its config so the build can derive a JSON Schema from
it (see below). The echo fixture uses a dataclass field:

```python
from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from pydantic_ai.capabilities.abstract import AbstractCapability
from pydantic_ai.toolsets import FunctionToolset


@dataclass
class EchoCapability(AbstractCapability[Any]):
    """Echo messages back, prefixed."""

    prefix: str = "echo"

    def get_toolset(self) -> FunctionToolset[Any]:
        toolset: FunctionToolset[Any] = FunctionToolset()

        @toolset.tool_plain
        def echo(message: str) -> str:
            """Echo the message back, prefixed with the configured prefix."""
            return f"{self.prefix}: {message}"

        return toolset
```

Spec entries hydrate through pydantic-ai's default `from_spec` →
`cls(**config)` path, so the **per-agent config schema is the class's own typed
shape** — here, the single `prefix: str = "echo"` field.

### Typed config → derived schema

The build derives the `configSchema` by introspecting the entrypoint class
inside the runner image, using pydantic's `TypeAdapter`/`create_model`
machinery:

| Class shape | How the schema is derived |
|---|---|
| dataclass | `TypeAdapter(cls).json_schema()` |
| `pydantic.BaseModel` subclass | `cls.model_json_schema()` |
| typed `__init__` (or a typed `from_spec` override) | a model synthesized from the typed parameters |

The framework base fields every `AbstractCapability` carries (`id`,
`description`, `defer_loading`) are stripped from the derived schema — they are
spec-entry plumbing, not per-agent config. A class with `*args`/`**kwargs` or
untyped constructor parameters is **underivable**: the schema is classified,
never guessed (see [Building](#building) for the escape hatches). The derived
schema is what attaching agents validate their `config` against at admission,
so typing the config is the difference between catching a config typo at
admission versus at pod bootstrap.

## Building

`flokoa capability build` produces an artifact image, its manifest, and a
generated `Capability` CR — all from one disposable container of the pinned
runner image, so the compatibility matrix is satisfied by construction.

```text
flokoa capability build [OPTIONS] [PATH]

  Build a capability artifact from PATH (a Python project) or --from-pypi.

  Produces in --output:
    <name>-artifact.oci.tar   OCI-layout artifact image (busybox + wheelhouse)
    manifest.json             artifact self-description (v1)
    <name>.capability.yaml    Capability CR (digest placeholder; push rewrites)
    config-schema.json        the config schema (strict builds)

Options:
  --from-pypi TEXT       Build from PyPI: PKG or PKG==VERSION (excludes PATH).
  --tag TEXT             Artifact image ref (default <name>:<version>);
                         required for push.
  --entrypoint TEXT      Capability class as module:attr (default: heuristic).
  --schema FILE          Use this config JSON Schema instead of deriving one.
  --permissive           Accept an underivable schema (loud warning;
                         permissive CR).
  --name TEXT            Capability CR name (default: normalized dist name).
  --runner-version TEXT  Runner release to build against (default: SDK-pinned).
  --runner-image TEXT    Full runner image override.
  --platforms TEXT       OCI platforms, e.g. linux/amd64,linux/arm64 (default:
                         host arch).
  --output DIRECTORY     Output directory for the artifact tar, manifest, and
                         CR.  [default: dist]
  --skip-smoke-test      Skip the install/import smoke test (discouraged).
```

Building the echo fixture:

```bash
cd operator/test/e2e/fixtures/capabilities/echo
flokoa capability build . --tag ghcr.io/danielnyari/capabilities/flokoa-cap-echo:0.1.0
```

### What the pipeline does

Inside one container session (the venv state carries across steps, exactly like
a runner pod's single venv), the build runs four steps:

1. **Freeze the baseline** — `pip list --format=freeze` inside the runner image
   *is* the baseline. The non-baseline closure is whatever the build resolves
   minus these pins.
2. **Build the wheelhouse** — `pip wheel` the target (the local `PATH` or
   `pkg==version` for `--from-pypi`) with the baseline freeze as constraints
   and `--only-binary :all:`, then drop wheels already in the baseline. Any
   dependency that ships no wheel (sdist-only) is **refused** with an error
   naming the custom-agent-image escape hatch — wheels only is the artifact
   boundary.
3. **Smoke test** — install the wheelhouse the same way a runner pod will
   (`pip install --no-index --find-links`), import the entrypoint, and
   instantiate it where possible. *A capability that can't import never gets an
   artifact.* `--skip-smoke-test` exists but warns loudly.
4. **Derive the schema** — resolve the entrypoint (`--entrypoint`, else the
   heuristic — see [Importing from PyPI](#importing-from-pypi)) and derive the
   config schema.

The host side then computes wheel sha256s, writes the doubly-validated
`manifest.json` (against both the pydantic model and the published v1 JSON
Schema), builds the `busybox` artifact image as an OCI-layout tarball
(`docker buildx --output type=oci`), and generates the `Capability` CR.

### What the outputs contain

In `--output` (default `./dist`):

- **`<name>-artifact.oci.tar`** — the OCI-layout image: `busybox:stable-musl` +
  `COPY --chmod=0644 wheelhouse/ /wheelhouse/`, labelled with
  `ai.flokoa.capability-name/-version/-contract-version`.
- **`manifest.json`** — the v1 artifact self-description: `name`, `version`,
  `contractVersion`, `entrypoint`, `requires` (the compatibility tuple anchored
  at the build image's pinned versions), `dependencies` (the pinned non-baseline
  closure), `wheels` (`{file, sha256}` per wheel), and — for strict builds —
  the inline `configSchema` plus its `schemaDigest`.
- **`<name>.capability.yaml`** — the generated `Capability` CR, with
  `spec.artifact` carrying a deliberately-invalid `@sha256:DIGEST-PENDING`
  placeholder so an un-pushed CR fails admission loudly. `push` rewrites it with
  the real digest. The spec is validated through the generated
  `flokoa_types.capability` model, so the mirror cannot drift from the CRD shape.
- **`config-schema.json`** — the derived (or supplied) config schema, on strict
  builds.

The `requires` tuple in the manifest is derived from the build image's runner
manifest: the same Python minor, `pydantic-ai >=<built minor>,<<next major>`,
and `flokoa-runner >=<built minor>`. Because the build happened inside the
pinned runner image, the produced artifact is compatible with that runner by
construction — that is the whole point of building in-image.

### Schema derivation outcomes

| Outcome | What `build` does |
|---|---|
| **derived** (typed config) | writes the schema into the CR and manifest; CR is `schemaPolicy: strict`. |
| **multiple capability classes** | refuses, listing each as `--entrypoint module:Class` so you can pick one. |
| **no capability class found** | refuses — pass `--entrypoint module:Class`. |
| **underivable** (untyped config) | refuses by default. Supply `--schema file.json` (makes the CR strict), or opt into `--permissive`. |

`--schema` and `--permissive` are mutually exclusive (`--schema` makes the CR
strict). `--permissive` prints a loud warning and writes a
`schemaPolicy: permissive` CR with no `configSchema` — per-agent config then
skips admission validation and surfaces only inside the runner pod. Permissive
capabilities are flagged in `kubectl get capabilities` and in `search` output;
prefer a typed schema or `--schema`.

## Publishing

`flokoa capability push` publishes a build's artifact and pins the CR to the
pushed digest.

```text
flokoa capability push [OPTIONS] REF

  Push the built artifact to REF and pin the Capability CR to its digest.

Options:
  --from DIRECTORY    Build output directory holding the artifact tar, CR, and
                      manifest.  [default: dist]
  --sign / --no-sign  cosign sign the pushed digest.  [default: no-sign]
  --cosign-key FILE   Key-based signing; omitted with --sign means keyless
                      (ambient OIDC).
  --apply             kubectl apply the digest-pinned CR.
  --namespace TEXT    Namespace for --apply.
  --index PATH        Local checkout of the index file/repo to append or
                      update (you commit it).
```

```bash
flokoa capability push ghcr.io/danielnyari/capabilities/flokoa-cap-echo:0.1.0
```

What it does:

- **`crane push`** the OCI-layout tarball to `REF` (always with `--index`) and
  records the returned `sha256:` digest. `REF` must be a **tag** reference — the
  digest is recorded from the push itself, never supplied.
- **Pins the CR** — rewrites `spec.artifact` from the `@sha256:DIGEST-PENDING`
  placeholder to `REF@sha256:<digest>`, and re-validates the pinned spec through
  the generated CRD model before writing it back. (Push refuses a CR that no
  longer carries the placeholder — that signals an already-pushed CR; re-run
  `build` for a fresh one.)
- **`--sign`** — `cosign sign` the pushed digest. Key-based with `--cosign-key`,
  otherwise keyless via ambient OIDC (workload identity in CI, browser flow
  interactively). The operator verifies signatures with `sigstore-go` at CR
  reconcile and surfaces the result in the `Verified` condition; see
  [the Capability reference](../capability.md) for verification and the
  `requireVerified` cluster policy.
- **`--apply`** — `kubectl apply` the pinned CR (optionally `--namespace`).
- **`--index PATH`** — append or replace the `(name, version)`-keyed entry in a
  local checkout of the index file (see [Finding capabilities](#finding-capabilities)).
  The CLI edits the JSON; **you commit and push it** — there is no git
  automation in v1.

A maintainer can publish a typed, signed, indexed capability in well under five
minutes with `build` + `push --sign --index ...`.

## Importing from PyPI

`flokoa capability import` is the one-command promise: any
`pydantic-ai-<name>` package on PyPI is one command from being an attachable
Flokoa capability. It composes `build --from-pypi` → an interactive schema
review → `push`.

```text
flokoa capability import [OPTIONS] PACKAGE

  Import PACKAGE (PKG or PKG==VERSION) from PyPI as a capability.

  Equivalent to:
    flokoa capability build --from-pypi PACKAGE --tag REF ...
    (interactive review of the derived config schema)
    flokoa capability push REF ...

Options:
  --tag TEXT             Artifact image ref to push, e.g.
                         ghcr.io/org/capabilities/name:1.2.0.  [required]
  --entrypoint TEXT      Capability class as module:attr (default: heuristic).
  --schema FILE          Use this config JSON Schema instead of deriving one.
  --permissive           Accept an underivable schema (loud warning;
                         permissive CR).
  --name TEXT            Capability CR name (default: normalized dist name).
  --runner-version TEXT  Runner release to build against (default: SDK-pinned).
  --runner-image TEXT    Full runner image override.
  --platforms TEXT       OCI platforms, e.g. linux/amd64,linux/arm64 (default:
                         host arch).
  --output DIRECTORY     Build output directory (what push reads). [default: dist]
  --sign / --no-sign     cosign sign the pushed digest.  [default: no-sign]
  --cosign-key FILE      Key-based signing; omitted with --sign means keyless.
  --apply                kubectl apply the digest-pinned CR.
  --namespace TEXT       Namespace for --apply.
  --index PATH           Local checkout of the index file/repo to append.
  --yes                  Non-interactive: accept the derived schema (CI).
```

```bash
flokoa capability import pydantic-ai-something==1.2.0 \
  --tag ghcr.io/myorg/capabilities/something:1.2.0
```

**Note:** `PACKAGE` must be a plain PyPI name with an optional `==version` pin.
VCS/URL requirements (`git+https://…`), extras, environment markers, and
smuggled pip options are rejected before the value reaches pip — this is
stricter than a bare `pip install` argument on purpose.

### Interactive schema review

Because the config schema was *derived*, not authored, `import` shows it and
asks a human to confirm before publishing:

- A **derived (strict) schema** is pretty-printed and confirmed (defaults to
  yes): *"Publish this schema as the capability's strict config contract?"*
- A **permissive** import (no schema) prompts with the consequence spelled out
  and defaults to **no**: *"… per-agent config will not be validated at
  admission. Publish anyway?"*

Decline and `import` aborts with guidance: refine with `--schema`, pick another
class with `--entrypoint`, or (last resort) opt into `--permissive`. `--yes`
skips the prompt for CI.

### Selecting the entrypoint

When a distribution exports several capability classes, the heuristic can't pick
one — it lists them and asks for `--entrypoint module:Class`. The heuristic
enumerates the concrete `AbstractCapability` subclasses the *distribution itself
defines* (re-exported pydantic-ai builtins and other libraries' capabilities are
ignored); exactly one candidate is picked automatically.

## Finding capabilities

`flokoa capability search` (and its no-argument alias `list`) merges two
sources into one table: the published v1 index, fetched and grepped
client-side, and in-cluster `Capability` CRs via `kubectl`.

```text
flokoa capability search [OPTIONS] [QUERY]

  Search the capability index (and the cluster) for QUERY.

  QUERY is a case-insensitive substring matched against name, description, and
  keywords; omit it to list everything.

Options:
  --index TEXT              Index URL or local path (default: the published
                           index URL, env FLOKOA_CAPABILITY_INDEX).
  --cluster / --no-cluster  Also list in-cluster Capability CRs (skipped
                           gracefully without kubectl/cluster).  [default: cluster]
```

```bash
flokoa capability search openapi
flokoa capability list
flokoa capability list --index ./capability-index   # a local checkout
```

The output table has `NAME · VERSION · RUNNER · POLICY · SIGNED · SOURCE`
columns; `SOURCE` is `index` or `cluster`. Permissive entries are flagged
`permissive (!)` with a footnote — unvalidated per-agent config is a property
the operator picking a capability must see. In-cluster rows derive `SIGNED` from
the CR's `Verified` condition.

> **The default index URL 404s today.** The published index ships with registry
> seeding (roadmap 10); until then the default
> `https://raw.githubusercontent.com/danielnyari/flokoa/main/capability-index/index.json`
> returns HTTP 404 and `search`/`list` say so (and still list in-cluster
> capabilities). Point `--index` (or `FLOKOA_CAPABILITY_INDEX`) at a published
> index URL or a local file — `push --index <checkout>` is what populates one.

## See also

- [Capability CR reference](../capability.md) — fields, what admission checks,
  how attachments compile, the `Verified`/`Permissive` conditions.
- [Runtime contract §4](../reference/runtime-contract.md#4-capability-artifacts-and-the-wheelhouse-layout)
  — the normative artifact format and runner consumption.
- [Capability examples](../examples/README.md#capability-examples) — a published
  CR and an Agent attaching it.
- [ADR-002](../design-docs/adr-002-capability-artifacts-and-cli.md) — the design
  decisions behind artifact delivery and the CLI.
