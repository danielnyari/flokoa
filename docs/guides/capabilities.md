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

## Using built-in capabilities

Before you build anything, check whether a **built-in** already covers your
need. Built-in capabilities are first-party capabilities baked into the runner
image (source tier [`builtin`](../capability.md#source-tiers)): there is nothing
to build, publish, or download — the chart installs their `Capability` CRs and
attaching one is a one-liner.

The first-party built-in today is **`flokoa-openapi`** (front any OpenAPI spec
as agent tools). Built-in CRs ship with the Helm chart
(`capabilities.builtin.install`, default `true`) into the operator's release
namespace; they appear in `flokoa capability search` with `TIER` = `builtin`
and `kubectl get capabilities` shows them with `SOURCE` = `builtin`,
`VERIFIED` = `True` (reason `BuiltIn`).

Attach one by name — the same shape as any Capability, but with no artifact to
publish first:

```yaml
spec:
  capabilities:
    - ref:
        name: flokoa-openapi
      config:
        spec: https://api.example.com/openapi.json
        base_url: https://api.example.com
```

The attaching Agent must be in the same namespace as the built-in CR
(cross-namespace capability refs are unsupported). An Agent with **only**
built-ins gets a plain pod: no initContainer, no `emptyDir`, no download. See
the [built-in attach example](../examples/README.md#capability-examples).

> **`flokoa-codemode-mcp` is not yet built-in.** The Code-Mode MCP package is
> an MCP *server*, not a capability class, so it ships as a separately-deployed
> server fronted by an [`AgentTool`](../agenttool.md), not as a `builtin`
> Capability. Only `flokoa-openapi` is baked in today.

## Prerequisites

The CLI orchestrates a container build and registry pushes; it shells out to
the tools you already use rather than vendoring binaries into the wheel:

| Tool | Needed by | Notes |
|---|---|---|
| `docker` or `podman` | `build`, `import` | The build runs **inside the Flokoa base image** (the runner baseline + build front-end). `CONTAINER_TOOL` overrides detection (docker preferred). `podman` builds one platform per image; multi-arch needs `docker buildx`. |
| `crane` | `push`, `import` | Pushes the OCI-layout tarball and records the digest. `FLOKOA_CRANE` overrides the binary. Install: `brew install crane`. |
| `cosign` | `push --sign`, `import --sign` | Signs the pushed digest. `FLOKOA_COSIGN` overrides. Install: `brew install cosign`. Only needed when you sign. |
| `kubectl` | `push --apply`, `search --cluster` | Applies the pinned CR and merges in-cluster Capability CRs. Skipped gracefully by `search` when absent. |
| The base image | `build`, `import` | Resolved as `ghcr.io/danielnyari/flokoa-capability-base:<version>` by default ([The base image](#the-base-image); `--base-image`/`--base-version`/`--runner-image`/`--runner-version`/`FLOKOA_CAPABILITY_BASE_IMAGE` override). The build wheels against this exact environment, so the artifact's compatibility is satisfied by construction. |
| `git`, `gh`, or an SSH agent | `build --from-git` | Provide the ambient credentials for a private repo clone ([Building from a private git repo](#building-from-a-private-git-repo)). Optional — public repos and the `GITHUB_TOKEN`/`GH_TOKEN` fallback need none of them. |

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
generated `Capability` CR — all from one disposable container of the
[base image](#the-base-image) (the pinned runner baseline plus the build
front-end), so the compatibility matrix is satisfied by construction.

```text
flokoa capability build [OPTIONS] [PATH]

  Build a capability artifact from PATH, --from-pypi, or --from-git.

  Exactly one source is required:
    PATH          a local Python project (source: image)
    --from-pypi   a PyPI package (EXTREMELY DANGEROUS; needs --allow-pypi)
    --from-git    a git repo, uv-style git+https://… / git+ssh://… (source: git)

  Produces in --output:
    <name>-artifact.oci.tar   OCI-layout artifact image (busybox + wheelhouse)
    manifest.json             artifact self-description (v1)
    <name>.capability.yaml    Capability CR (digest placeholder; push rewrites)
    config-schema.json        the config schema (strict builds)

Options:
  --from-pypi TEXT       Build from PyPI: PKG or PKG==VERSION (requires
                         --allow-pypi; excludes PATH/--from-git).
  --allow-pypi           Acknowledge the EXTREMELY DANGEROUS PyPI tier
                         (required with --from-pypi).
  --from-git TEXT        Build from git: git+https://… or git+ssh://… [@ref]
                         [#subdirectory=…] (excludes PATH/--from-pypi).
  --tag TEXT             Artifact image ref (default <name>:<version>);
                         required for push.
  --entrypoint TEXT      Capability class as module:attr (default: heuristic).
  --schema FILE          Use this config JSON Schema instead of deriving one.
  --permissive           Accept an underivable schema (loud warning;
                         permissive CR).
  --name TEXT            Capability CR name (default: normalized dist name).
  --base-image TEXT      Full build image override (default: flokoa-
                         capability-base:<ver>).
  --base-version TEXT    Build base image version (composed with the base
                         repository).
  --runner-version TEXT  (retained) build environment version; the default now
                         resolves to the base image.
  --runner-image TEXT    (retained) full build image override (back-compat
                         alias for --base-image).
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

1. **Freeze the baseline** — `pip list --format=freeze` inside the base image
   (whose baseline is byte-identical to the runner's) *is* the baseline. The
   non-baseline closure is whatever the build resolves minus these pins.
2. **Build the wheelhouse** — `pip wheel` the target (the local `PATH`,
   `pkg==version` for `--from-pypi`, or the in-container git checkout for
   `--from-git`, which is cloned first and then treated exactly like a `PATH`)
   with the baseline freeze as constraints and `--only-binary :all:`, then drop
   wheels already in the baseline. Any dependency that ships no wheel
   (sdist-only) is **refused** with an error naming the custom-agent-image
   escape hatch — wheels only is the artifact boundary.
3. **Smoke test** — install the wheelhouse the same way a runner pod will
   (`pip install --no-index --find-links`), import the entrypoint, and
   instantiate it where possible. *A capability that can't import never gets an
   artifact.* `--skip-smoke-test` exists but warns loudly.
4. **Derive the schema** — resolve the entrypoint (`--entrypoint`, else the
   heuristic — see [Selecting the entrypoint](#selecting-the-entrypoint)) and
   derive the config schema.

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

A local `PATH` build records source tier
[`image`](../capability.md#source-tiers) — the author-built-their-own case. You
never hand-write the artifact, the `manifest.json`, or the `Capability` CR — all
three are generated; `push` then pins the digest.

### The capability name (`--name`)

The `Capability` CR's `metadata.name` must be an **RFC 1123 DNS label**:
lowercase alphanumerics and `-`, **no dots**, at most **63 characters**. The pod
container and volume names the operator emits derive from `cap-<name>` (itself a
DNS label), so admission requires it and the CLI enforces it up front.

`--name` defaults to the PEP 503-normalized distribution name (`_`/`.`→`-`,
lowercased) — already DNS-safe for ordinary package names. Pass `--name` when
the normalized dist name is not a valid label (e.g. it exceeds 63 chars) or when
you want a shorter handle; the CLI rejects an invalid name with the exact rule:

```console
$ flokoa capability build . --name my.cap
Error: capability name 'my.cap' is not an RFC 1123 DNS label (lowercase
alphanumerics and '-', no dots, max 63 chars) — pod container and volume names
derive from cap-<name>, so admission requires a DNS label — pass --name
```

(The `serializationName` — the *spec-entry* name capabilities are referenced by
in a compiled spec — is a separate thing and *may* carry a first-party dotted
namespace for built-ins; see the [CR reference](../capability.md#spec-fields).)

### The base image

`build` (and `import`) run their pipeline inside the Flokoa **base image**, not
on your host interpreter. The base image is the pinned runner baseline plus the
build front-end (`pip`/`wheel`/`setuptools`) — literally
`FROM ghcr.io/danielnyari/flokoa-runner:<version>` plus a build-tooling layer —
so the wheelhouse is resolved against the *exact* environment a runner pod has
and the compatibility matrix holds by construction.

It is resolved as `ghcr.io/danielnyari/flokoa-capability-base:<version>`, tagged
to match the runner version (the base image, runner, and operator all move
together with each release). Override it, highest precedence first:

| Override | Effect |
|---|---|
| `--base-image TEXT` | Full image reference (repo + tag). |
| `--base-version TEXT` | Version only, composed with the default base repository. |
| `FLOKOA_CAPABILITY_BASE_IMAGE` env | Full image reference. |
| `--runner-image` / `--runner-version` | Retained back-compat aliases for `--base-image` / `--base-version`. Pointing `--runner-image` at a *bare* runner image still works — the CLI falls back to seeding `pip` per build — but the happy path no longer needs it. |

!!! note "Reproducible builds: the base image pins its build front-end"
    The base image pins `pip`, `wheel`, and `setuptools` to exact versions per
    runner release (baked once at image-build time, not re-fetched per CLI run).
    Rebuilding `flokoa-capability-base:<runnerVersion>` therefore yields the same
    build toolchain instead of whatever happened to be latest on PyPI that day —
    so two builds of the same source against the same base tag resolve the same
    wheelhouse. These pins are the *build* front-end, distinct from the runtime
    baseline (`runner.lock` / `runner-manifest.json`); pip is intentionally
    absent from the locked baseline, exactly as in the runner image. Override
    them per build with `--build-arg PIP_VERSION=… WHEEL_VERSION=…
    SETUPTOOLS_VERSION=…` when rebuilding the base image itself.

## Building from a private git repo

`flokoa capability build --from-git` builds a normal signed artifact from a
(typically private) git repo, recording source tier
[`git`](../capability.md#source-tiers). The clone happens **at build time,
inside the disposable build container**; the output is the same self-contained
wheelhouse as any other build, and **nothing is fetched from git at deploy or
run time**.

```bash
flokoa capability build \
  --from-git git+https://github.com/org/private-cap@v1.2.0#subdirectory=pkg/cap \
  --tag ghcr.io/org/capabilities/private-cap:1.2.0
```

The URL grammar is uv/pip-style and validated on the host before anything
reaches the container — a bare `https://…` without `git+`, extras, environment
markers, smuggled pip options, and embedded `user:pass@` credentials are all
rejected:

- **scheme** — `git+https://…` (the named, tested path) or `git+ssh://…`
  (works against `github.com` via the SSH agent).
- **host** — `HOST` or `HOST:PORT`. A **non-standard `:port`** (anything other
  than 80/443) is carried through to the clone *and* into the credential lookup,
  so a self-hosted instance on a custom port resolves its per-`host:port`
  credentials correctly.
- **ssh user** — `git+ssh://git@HOST/…` may carry an ssh user (`git@`). A
  **`git+https://…` URL must NOT carry a `user@` prefix**: https credentials are
  resolved separately, so a username there has no effect and is rejected as
  misleading — use `git+ssh://git@HOST/…` if you need an ssh user. (Embedded
  passwords, `user:pass@`, are forbidden for both schemes.)
- **`@ref`** (optional) — a branch, tag, or commit SHA. Recorded as
  `provenance.git.ref`. With a ref, the clone is a full clone followed by
  `git checkout <ref>`; **with no ref, the clone is shallow** (`--depth 1`) since
  only the default-branch tip is needed for the wheelhouse build. A `..` path
  component in `@ref` or `#subdirectory=` is rejected on the host.
- **`#subdirectory=…`** (optional) — the path within the repo to the Python
  project. Recorded as `provenance.git.subdirectory`.

### Authentication: ambient first, token fallback

Credentials are resolved **on the host** (where your ambient creds live) and
only the minimum is injected into the build container. The precedence is:

1. **Ambient (preferred), nothing injected as a long-lived secret:**
    - `git+https://…` — your **git credential helper** (`git credential fill`),
      else your **GitHub CLI login** (`gh auth token`). Whatever your normal
      `git`/`gh` already uses just works.
    - `git+ssh://…` — your **SSH agent**: the agent *socket* (`$SSH_AUTH_SOCK`)
      is forwarded into the build container with strict host-key checking — no
      private-key material is copied. `github.com`'s host keys are pre-seeded in
      the base image, so `git+ssh` against GitHub works out of the box.
2. **Token fallback** — `GITHUB_TOKEN`, then `GH_TOKEN`, from the environment.
3. **Public repos need no credential** — auth only matters on a 401/403.

If a private clone needs auth and none resolves, `build` fails up front naming
the precedence you can fix (e.g. *"tried the ambient git credential helper and
gh auth token, then `$GITHUB_TOKEN` / `$GH_TOKEN`; set one to build from a
private repo"*).

### The token never leaks

When an https token is used it is passed to the clone step **only** as an
ephemeral env var on the container `exec` (consumed via `GIT_ASKPASS`) — never
in the argv, a mount file, or the image. The recorded git remote URL is clean
(no `user:token@`); the artifact is built from the checked-out source (no
credentials in it); and the generated CR's `provenance.git` records only the
**clean URL + resolved commit** (plus `ref`/`subdirectory` if given). Captured
git stderr is credential-redacted before it can reach any error or log line.
The resolved commit is the durable provenance — it pins the exact code built
even if the branch or tag later moves. Non-GitHub `git+https`/`git+ssh` hosts
should work via ambient credentials; GitHub is the named, tested path.

### Troubleshooting git builds

| Symptom | Fix |
|---|---|
| **Private `git+https` repo: clone fails with "requires credentials and none resolved"** | The host has no credential for the host. Set one of, in precedence order: a **git credential helper** (`git config --global credential.helper …` so `git credential fill` returns a token for the host), a **`gh` login** (`gh auth login`), or export **`GITHUB_TOKEN`** / **`GH_TOKEN`**. `build` resolves auth host-side and fails up front naming this exact ladder. |
| **`git+ssh` repo: clone fails / "needs a running SSH agent ($SSH_AUTH_SOCK)"** | Start an agent and add your key: `eval "$(ssh-agent -s)" && ssh-add ~/.ssh/id_ed25519`. The agent *socket* is forwarded into the build container (no private key is copied). Or switch to `git+https://…` with a credential helper / `GITHUB_TOKEN`. |
| **`git+ssh` against a non-GitHub host: "Host key verification failed"** | The base image strict-checks host keys and pre-seeds **only `github.com`**'s keys. For another host, add its key to `flokoa-capability-base`'s `ssh_known_hosts` and rebuild the base image (point `--base-image` at it), or clone over `git+https` instead. `github.com` works out of the box. |
| **"--from-git https URLs must not include a 'user@' prefix"** | https credentials are resolved separately, so a username is dropped — drop it. If you genuinely need an ssh user, use `git+ssh://git@HOST/…`. |
| **"a bare https:// without git+ … are not accepted"** | Use the `git+https://` (or `git+ssh://`) scheme, not a plain `https://` URL — the grammar is uv/pip-style on purpose. |
| **Token appears to have leaked into a log** | It cannot: the token rides an ephemeral `exec` env consumed via `GIT_ASKPASS`, the recorded remote URL is reset to the clean URL, and captured git stderr is credential-redacted before printing. If you see `<redacted>@` in output, that is the redactor working as intended. |

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

## Building or importing from PyPI

!!! danger "PyPI is the EXTREMELY DANGEROUS tier"
    Building from PyPI (`build --from-pypi`, and `import`) pulls code published
    by **arbitrary maintainers with no first-party vetting** — a supply-chain
    risk. The artifact is digest-pinned once built (integrity), but that says
    **nothing** about whether the source package is malicious. Prefer
    `--from-git` or building your own ([image tier](#building)), and restrict
    the cluster with [`capabilities.policy.allowedSources`](../capability.md#source-policy-allowedsources).

`build --from-pypi` records source tier
[`pypi`](../capability.md#source-tiers) and **requires an explicit
`--allow-pypi`** acknowledgment. Without it the command fails:

```console
$ flokoa capability build --from-pypi pydantic-ai-foo==1.2.0 --tag ...
Error: --from-pypi builds from an unvetted PyPI package (arbitrary maintainers,
no first-party vetting) — the EXTREMELY DANGEROUS tier. Re-run with --allow-pypi
to acknowledge, and prefer --from-git or building your own (image tier).
```

With `--allow-pypi` the build proceeds, printing a loud red
EXTREMELY-DANGEROUS banner first and stamping the CR `source: pypi` (with
`provenance.pypi.requirement`). An operator can refuse the `pypi` tier
cluster-wide via `allowedSources` — a `pypi` Capability is then denied at both
admission and compile.

`flokoa capability import` is the one-command promise: any
`pydantic-ai-<name>` package on PyPI is one command from being an attachable
Flokoa capability. It composes `build --from-pypi` → an interactive schema
review → `push`. **`import` implies the `--allow-pypi` acknowledgment** (it *is*
a PyPI build) but still prints the danger banner and stamps `source: pypi`, in
addition to its own interactive schema-review gate.

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
flokoa capability list --no-cluster                 # index only (skip kubectl)
```

The output table has `NAME · VERSION · TIER · RUNNER · POLICY · SIGNED · SOURCE`
columns:

- **`TIER`** is the [source tier](../capability.md#source-tiers) (`builtin` ·
  `image` · `git` · `pypi`). A `pypi` row is flagged `pypi (!!)` with a red
  footnote — it is the EXTREMELY DANGEROUS tier and the cluster may refuse it
  via [`allowedSources`](../capability.md#source-policy-allowedsources). A row
  with no recorded tier renders `-`.
- **`SOURCE`** is `index` or `cluster` — which feed the row came from (distinct
  from `TIER`, which is where the *code* came from).
- **`POLICY`** is `strict` or, for permissive capabilities, `permissive (!)`
  with a footnote — unvalidated per-agent config is a property the operator
  picking a capability must see.
- **`SIGNED`** is `yes`/`no`. In-cluster rows derive it from the CR's `Verified`
  condition, so a `builtin` row shows `yes` (its `Verified=True / BuiltIn`
  short-circuit); index rows use the published `signed` flag.

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
- [ADR-003](../design-docs/adr-003-capability-source-tiers.md) — the design
  decisions behind the source tiers, built-ins, and the git/pypi build sources.
