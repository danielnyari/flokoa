# ADR-003: Capability sourcing tiers and provenance

**Status:** Accepted — 2026-06-15

## Context

[ADR-002](adr-002-capability-artifacts-and-cli.md) shipped the capability
artifact format, its delivery into runner pods, optional cosign verification,
and the `flokoa capability build | push | import | search` CLI. At that point a
capability's code could come from exactly two places: a **local Python project**
(`build PATH`) or a **PyPI package** (`build --from-pypi`). PyPI is a
supply-chain risk — arbitrary maintainers, no first-party vetting — and there
was no way for the platform to know, or refuse, where a capability's code came
from. Two further gaps followed from ADR-002: registry **seeding** (publishing a
first-party capability set such as `flokoa-openapi`) was deferred, and authors
had no frictionless way to build from a private repo.

This unit adds a `spec.source` tier to the [`Capability`](../capability.md) CRD
and the machinery around it: a four-tier trust ladder, a **built-in** tier baked
into the runner image, an **image** tier built against a published base image, a
**git** tier built from a private repo with ambient→token auth, and the existing
**pypi** tier marked dangerous and made cluster-refusable. The runtime delivery
path is unchanged — artifacts are still self-contained, digest-pinned OCI
wheelhouse images; git/PyPI fetching is **build-time only**, nothing is fetched
from git or PyPI at deploy or run time. The change is additive
(`contractVersion: 1` unchanged).

## Decision

| Decision | Rationale |
|---|---|
| **A four-tier trust ladder recorded in `spec.source` (`builtin` > `image` > `git` > `pypi`)** | The platform must be able to reason about — and refuse — where capability code came from. A single enum, defaulting to `image`, makes every existing ADR-002 capability valid unchanged (they were the author-built-their-own case all along) while giving operators a lens for policy. The ordering is documentation/policy-level reasoning, not a numeric runtime comparison: `allowedSources` is an explicit set, because "allow image and git but not pypi" is a legitimate non-prefix set. |
| **`builtin` is baked into the runner image; no artifact, integrity via the image digest** | The strongest supply-chain story: Flokoa's own capabilities install into the runner image at build time, so attaching one downloads nothing and a vanilla cluster starts the pod with the capability importable. There is no separate artifact to cosign-verify — integrity rides on the runner image digest the cluster already pulls. This **subsumes the deferred registry seeding** (a built-in attaches with no artifact to publish) and is the cleanest integration: the compiler emits the spec entry but skips delivery, so a built-in contributes zero mounts and the builder is untouched. |
| **A `source: builtin` CR is matched against embedded built-in metadata, not trusted** | Admission must validate a builtin CR offline with no artifact to read. The operator embeds a per-runner-version `builtinCapabilities` map (alongside the existing baseline) and the webhook/compiler match the CR's name + `entrypoint` + `schemaDigest` against it. A forged `source: builtin` CR for an unknown capability is rejected — there is no "trust the CR's claim" path. This closes the only spoofing hole the no-artifact tier introduces. |
| **`image` tier = the base image as the build environment; the CLI stays the builder** | Authors build their own capability exactly as before, but `build` now runs inside a published `flokoa-capability-base` image (the pinned runner baseline + `pip`/`wheel`/`setuptools`) rather than the bare runner. This bakes `pip` once at image-build time, retiring the per-build `ensurepip` hack on the happy path, while keeping the compatibility-by-construction guarantee (the base image is `FROM` the runner, so its baseline is byte-identical). `--base-image`/`--base-version` override; `--runner-image`/`--runner-version` are retained back-compat aliases. `manifest.json` stays fully auto-generated — the author never writes it. |
| **`git` tier builds at build time only, with ambient→token auth and no-leak token handling** | `--from-git git+https://…` / `git+ssh://…` clones inside the disposable build container, then proceeds exactly like a `PATH` build — the output is a normal, self-contained, signable artifact, and nothing is fetched from git at deploy/run time. Auth is resolved **host-side** (it has the user's ambient creds): for https the git credential helper / `gh auth token`, then a `GITHUB_TOKEN`/`GH_TOKEN` fallback; for ssh the forwarded agent socket (not keys). An https token reaches the container **only** as an ephemeral env var on the `exec` (consumed via `GIT_ASKPASS`) — never argv, mount, image, the recorded URL, the CR, or a log line. Only the clean repo URL + resolved commit are recorded as provenance. The URL grammar is uv/pip-style and host-validated: a `git+https://` URL **must not** carry a `user@` prefix (https creds are resolved separately, so a username has no effect and is rejected as misleading — use `git+ssh://git@HOST/…` for an ssh user); a non-standard `:port` is carried into the per-`host:port` credential lookup so self-hosted instances on a custom port resolve creds correctly; a `..` path component in `@ref`/`#subdirectory=` is rejected; and the clone is shallow (`--depth 1`) when no `@ref` is given, full + `checkout` with one. |
| **`pypi` is kept but gated (`--allow-pypi`) and cluster-refusable** | The PyPI path stays for compatibility but is the EXTREMELY DANGEROUS tier: `--from-pypi` now requires an explicit `--allow-pypi` acknowledgment and prints a loud red banner; the CR is stamped `source: pypi`. An operator can refuse the tier cluster-wide via `allowedSources`. `import` (which is a PyPI build) implies the acknowledgment but still prints the banner and stamps the tier, in addition to its own schema-review gate. |
| **`source` is asserted provenance metadata, NOT a cryptographic claim** | `source` records the origin the build asserted; it is not a proof. The robust control for "only my org's builds may attach" is **cosign keyless identity policy + `requireVerified`** (provenance *of the digest*); `allowedSources` is a coarser, complementary filter on the recorded tier. The docs and this ADR state this plainly so operators don't over-trust the field. A forged `source: image` still must supply a digest-pinned artifact, and under `requireVerified` + cosign the lie does not help. |
| **The `allowedSources` policy is a dual-gate, mirroring `requireVerified`** | The cluster policy `capabilities.policy.allowedSources` (empty = allow all four, the default) is enforced at **both** admission (Agent create/update) and compile (a Capability's `source` edited after admission, surfacing `SpecValid=False`). Both the Helm chart and the operator binary validate the source set at config time, so a typo'd policy fails fast rather than silently allowing everything — the same belt-and-braces pattern as `requireVerified`. |

### `Verified=True / BuiltIn` for the built-in tier

A `builtin` capability has no separate artifact whose signature could attest
provenance. Rather than leave its `Verified` condition `Unknown` (which would
brick built-ins under `requireVerified` — perverse, since they are the *most*
trusted tier), the controller short-circuits the condition to `True` with reason
`BuiltIn` and the message *"built into the runner image; integrity bound by the
runner image digest"*. This keeps `requireVerified` clusters working with
built-ins while being honest that the proof is image-level (the runner image is
cosign-verifiable as an image at the pod-admission layer), not per-artifact
cosign. The webhook/compiler `requireVerified` checks already key off
`Verified=True`, so no change was needed there.

### Built-ins become part of the baseline (a desired consequence)

Once `flokoa-openapi` (and any future built-in) is a runner-runtime dependency,
the runner baseline closure includes it and its transitive deps. The existing
dependency-conflict detector then treats those as baseline packages: a
third-party capability pinning a *different* version of one of them is a
**correct** admission conflict — one pod, one Python environment. This is the
intended behavior, called out so it is not mistaken for a regression.

## Consequences

- **A new published image, `flokoa-capability-base`** (the runner + `pip`/`wheel`/
  `setuptools` + an SSH client and GitHub's seeded host keys for `git+ssh`),
  tagged to the runner version and built in `release.yml` after the runner it is
  `FROM`. It is now the default build environment; the per-build `ensurepip` hack
  stays only as a fallback for someone overriding `--runner-image` to a bare
  runner.
- **The runner image and operator baseline gained a `builtinCapabilities` map**
  (additive optional manifest field — [runtime contract §1](../reference/runtime-contract.md#1-the-pinned-environment-the-published-lockfile-is-the-platform)).
  It is generated by `gen_builtin_capabilities.py`, merged into both the image
  manifest and the embedded operator baseline, and cross-checked by the existing
  `verify-runner-contract` gate.
- **`manifest.json` and the Capability CR gained `source` + `provenance`**
  (additive optional fields — [runtime contract §4](../reference/runtime-contract.md#4-capability-artifacts-and-the-wheelhouse-layout)).
  `spec.artifact` became optional (widened to empty-or-digest-pinned), with the
  required/forbidden-per-source rule enforced in the webhook and re-checked in
  the compiler.
- **`flokoa-codemode-mcp` is not shipped built-in.** It is an MCP *server*, not
  an `AbstractCapability` subclass, so it stays a separately-deployed server
  fronted by an `AgentTool`. Only `flokoa-openapi` is baked in today; the
  built-in set is intentionally small and may grow.
- **Built-in serialization names allow a dot** (`flokoa.OpenAPI`) for the
  first-party namespace; user tiers (`image`/`git`/`pypi`) keep the strict
  bare-class-name rule. Schema-digest equality between the Python build path and
  the Go admission path is **structural**, not string-compare, to survive
  Python/Go float rendering (`30.0` vs `30`).
- **The harness capability set is explicitly future, NOT baked in** — harness
  packages remain contractually banned from the runner baseline (the no-harness
  gate) and ship as digest-pinned artifacts, unchanged.
- **Version skew is the existing surface, not a new one.** Built-in metadata is
  keyed by runner version like the baseline and the AgentSpec schema; an Agent
  pinning a `runnerVersion` whose embedded baseline doesn't list a built-in fails
  compile with `SpecValid=False`, never a broken pod.
- **The CLI rejects a non-DNS-label `--name` up front.** The generated CR's
  `metadata.name` must be an RFC 1123 DNS *label* (lowercase alphanumerics + `-`,
  no dots, max 63) because pod container/volume names derive from `cap-<name>`.
  `--name` defaults to the PEP 503-normalized distribution name; the CLI
  validates it through the same constraint the Capability webhook enforces, so
  the rule cannot drift from a hand-rolled regex.

## See also

- Roadmap [09](../roadmap/09-capability-artifacts-delivery.md) ·
  [10](../roadmap/10-capability-cli-and-registry.md) — the artifact, delivery,
  and CLI foundation this unit extends.
- [ADR-002](adr-002-capability-artifacts-and-cli.md) — the prior decisions on
  artifacts, delivery, verification, and the CLI.
- [Capability CR reference](../capability.md) — `spec.source`, the
  artifact-required/forbidden rule, the `Source` printcolumn, and the
  `allowedSources` policy.
- [Authoring & publishing guide](../guides/capabilities.md) — using built-ins,
  building from git, the base image, and the PyPI danger flow.
- [Runtime contract §1](../reference/runtime-contract.md#1-the-pinned-environment-the-published-lockfile-is-the-platform)
  (`builtinCapabilities`) and
  [§4](../reference/runtime-contract.md#4-capability-artifacts-and-the-wheelhouse-layout)
  (manifest `source`/`provenance`) — the normative additive contract changes.
