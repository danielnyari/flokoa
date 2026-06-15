# ADR-002: Capability artifacts, delivery, and the authoring CLI

**Status:** Accepted — 2026-06-13

## Context

Roadmap unit 08 shipped the [`Capability`](../capability.md) CRD: a fully
validated but **inert** mirror of an artifact — admission machine-checks the
config schema, `requires` tuple, and dependency conflicts, but nothing
delivered a wheelhouse to a pod and nothing built one. Units
[09](../roadmap/09-capability-artifacts-delivery.md) (artifacts & delivery) and
[10](../roadmap/10-capability-cli-and-registry.md) (CLI & registry) close the
loop: an on-disk artifact format, its delivery into runner pods with optional
signature verification, and `flokoa capability build | push | import | search`
— the tooling whose job is to make publishing a capability boring. The decisions
below were taken so a maintainer can publish a typed, signed, indexed capability
in under five minutes and so any harness-ecosystem PyPI package is one command
from being attachable.

## Decision

| Decision | Rationale |
|---|---|
| **The artifact is an OCI _image_ (`busybox:stable-musl` base), not a raw OCI artifact** | initContainers must be able to `run` it to `cp` the wheelhouse out; image-ness also keeps every registry, cache, and scanner happy. `busybox` supplies a static shell + `cp` and nothing else. |
| **initContainer copy is the default delivery path; ImageVolume is the detected optimization** | The initContainer path (copy into a shared `emptyDir`, runner mounts it read-only) works on every cluster and kubelet image-caching makes repeat starts cheap. ImageVolume (a read-only `image:` volume, no copy) is faster but beta, default-off, needs containerd 2.x, and has patchy managed-cloud support in mid-2026. `auto` mode probes the exact mount shape once at operator startup and falls back silently-but-observably; the mode is per-cluster, never per-agent. |
| **Verification happens at CR reconcile, never in the pod path** | Digests are immutable, so verify-once-per-digest has no TOCTOU window on content. Digest pinning (admission, unit 08) already does integrity; cosign adds _provenance_ (who published this digest). Verifying in the controller and recording the result in the `Verified` condition keeps pods fast and failures legible, instead of a per-pod-start network call. The `requireVerified` cluster policy is enforced at both admission and compile to cover policy flips. |
| **The CLI builds _inside_ the pinned runner image** | `pip wheel`-ing the capability and its non-baseline closure against the runner's own installed packages (the baseline freeze, by construction) means the compatibility matrix is satisfied by construction — the produced artifact is compatible with that exact runner because it was built against it. The smoke test runs in the same disposable container; a capability that can't import never gets an artifact. |
| **crane + a JSON index for v1, not a registry client or hosted service** | `push` shells out to `crane` (OCI-layout tarball push, digest captured deterministically via `--image-refs`); the index is a single `index.json` in a git repo, fetched and grepped client-side, `(name, version)`-keyed. Both are deliberately boring — a hosted index is a later investment, not P0b. No binaries are vendored into the `flokoa` wheel; `crane`/`cosign` are discovered on PATH and only the command that needs one preflights it. |
| **`manifest.json` carries the inline `configSchema`** (plus `schemaDigest`) | The roadmap's minimal field list is extended with the inline schema (an additive optional field, allowed by the runtime-contract change policy) so the artifact is fully self-describing — `push` can regenerate the `Capability` CR from the artifact alone, and the CR spec mirrors the manifest by value bound to the pushed digest. |
| **Wheels only, no sdists, ever** | Installing a wheel executes no setup code; an sdist would. The boundary is enforced at build (`--only-binary :all:` refuses the sdist fallback) and again at bootstrap (only manifest-listed `.whl` files may exist). System-level dependencies (binaries, apt packages) are out of scope for capability artifacts — that is what custom agent images are for. The build error names that escape hatch. |

### Schema derivation, not authoring (CLI)

The CLI derives the `configSchema` by introspecting the entrypoint class inside
the runner image (pydantic's `TypeAdapter`/`create_model` over a dataclass,
`BaseModel`, or typed constructor), stripping the framework base fields. An
underivable shape (`*args`/`**kwargs`/untyped) is **classified and refused**, not
guessed — `--schema file.json` and an explicit `--permissive` are the escape
hatches, and `import` adds an interactive human review of the derived schema
before publishing. The refusal-by-default posture is deliberate: a schema-less
capability accepts attacker-shaped config invisibly at admission.

## Consequences

- **The runner image now seeds `pip`** — the uv-managed venv shipped without it,
  which would have failed `_pip_install` at every pod bootstrap (found and fixed
  while wiring delivery). The CLI's in-runner scripts also `ensurepip` once.
- **The operator gained `sigstore-go` + `go-containerregistry`** (pinned to
  v1.1.0 / v0.20.6 to stay on Go 1.24.10), isolated behind the
  `verify.ArtifactVerifier` interface and constructed only when cosign
  verification is enabled. Controller and webhook tests run against fakes.
- **A digest bump is a normal rollout** — it changes the pod template
  (initContainer image / ImageVolume reference), so Deployments roll naturally;
  a Capability edit recompiles dependent Agents via the existing watch.
- **The CLI shells out** to `docker`/`podman`, `crane`, `cosign`, and `kubectl`
  with explicit argv arrays (no shell interpolation); a missing binary fails only
  the command that needs it, with an install one-liner.
- **The published index does not exist yet.** `search`/`list` default to a raw
  GitHub URL that 404s helpfully until registry seeding publishes
  `capability-index/index.json`; `--index`/`FLOKOA_CAPABILITY_INDEX` point at any
  index, and in-cluster capabilities still list. Seeding the harness +
  endorsed-community capabilities is deferred per the session scope decision.
- **Two manifest-field deviations from the roadmap** are carried deliberately:
  the inline `configSchema` (above), and the fixture closure being declared
  statically pre-CLI (reconciled by a CLI-vs-fixture manifest-parity test).

## See also

- Roadmap [09](../roadmap/09-capability-artifacts-delivery.md) ·
  [10](../roadmap/10-capability-cli-and-registry.md)
- [Capability CR reference](../capability.md) ·
  [Authoring & publishing guide](../guides/capabilities.md)
- [Runtime contract §4](../reference/runtime-contract.md#4-capability-artifacts-and-the-wheelhouse-layout)
  — the normative artifact format
