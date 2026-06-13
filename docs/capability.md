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
| `entrypoint` | Python `module:attr` resolving to the capability class (a pydantic-ai `AbstractCapability` subclass). `attr` must be the class itself (a single identifier), bound under its own `__name__` — no factories or re-export aliases. |
| `serializationName` | The capability's spec-entry name when the class overrides pydantic-ai's default. Defaults to the entrypoint class name (`attr`), which is pydantic-ai's own default serialization name. Must be a bare identifier; the `flokoa.platform/` prefix is reserved. |
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
`serializationName` (default: the entrypoint class name `attr`), placed after
the fragment's and AgentTools' entries and before the platform-injected block.
Two attachments that resolve to the same entry name — or one that collides with
a native or injected entry — are rejected at admission (set `serializationName`
to disambiguate). Capability references must be in the agent's own namespace
(cross-namespace allow-listing is later policy work).

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

## Artifact delivery

Attaching a Capability digest-pins an OCI wheelhouse artifact; the operator's
job is to land that wheelhouse at `/opt/flokoa/capabilities/<name>/` inside the
runner pod so `Agent.from_spec` can hydrate the entry at bootstrap. Two
delivery mechanisms produce the *identical* on-pod layout — the runner never
knows which one carried the wheels:

- **initContainer (default, works everywhere).** One init container per
  attached capability runs the digest-pinned artifact image and copies
  `/wheelhouse/.` into a shared `emptyDir`, each into its own `subPath`
  subdirectory so siblings can't touch one another. The runner mounts that
  volume read-only. This needs nothing from the cluster beyond the ability to
  pull an image, so it is the only *required* path.
- **ImageVolume (fast path).** Each capability's artifact is mounted directly
  as a Kubernetes `image:` volume — no copy, no init container, no `emptyDir`.
  It is faster but depends on cluster features (below).

**Delivery mode is per-cluster, not per-Agent.** There is no knob on the Agent
or Capability CR. The operator resolves one mode at startup and every Agent it
reconciles uses it. Set it with the Helm value `capabilities.delivery.mode`:

| `capabilities.delivery.mode` | Behavior |
|---|---|
| `initContainer` (default) | Always initContainer copy. No probe. |
| `imageVolume` | Always ImageVolume. **Trusted, not probed** — if the cluster doesn't support it, agent pods fail to start (this surfaces as pod-level `FailedMount`/`CreateContainerError`, not an operator error). |
| `auto` | The operator runs a one-shot **startup probe** (below). Probe succeeds → `imageVolume`; any failure → **silent fallback** to `initContainer`. |

The probe (`auto` mode only) creates a short-lived pod named
`flokoa-imagevolume-probe` in the operator's namespace that mounts the runner
image as an `image:` volume with the *exact* `subPath` shape the builder emits
and checks the file is readable. It is deleted in all paths. The probe pod
image defaults to the operator's pinned runner image (already pullable on the
cluster) and the timeout is `capabilities.delivery.probe.timeoutSeconds`
(default 60s) — in `auto` mode on a non-supporting cluster, startup is delayed
by up to that timeout before falling back. The result is settled for the
operator process lifetime; **each operator restart re-probes**, because a
cluster upgrade can change the answer.

**Requirements for ImageVolume:** the `ImageVolume` feature gate must be on at
both the API server and the kubelet, and the node runtime must be
**containerd 2.x** (the `subPath`-on-image-volume surface in particular is the
least-baked part). On clusters that lack these, leave the mode at
`initContainer` or use `auto` so the operator picks the safe path for you.

### Observing the effective mode

`auto`'s fallback is silent toward Agents, but never invisible to operators.
Three surfaces report the resolution:

- **Metrics** (on the operator's secured metrics endpoint):
  `flokoa_capability_delivery_mode{mode="initContainer|imageVolume"} 1`
  reports the active mode, and `flokoa_capability_imagevolume_supported`
  (`1`/`0`) reports the probe verdict — the latter is exported *only* when a
  probe actually ran (i.e. in `auto` mode).
- **State ConfigMap** `flokoa-capability-delivery` in the operator's install
  namespace (`flokoa-system` by convention), with the fields `configuredMode`,
  `effectiveMode`, `imageVolumeSupported`, `probedAt`, and `message`:

  ```console
  $ kubectl get configmap flokoa-capability-delivery -n flokoa-system -o yaml
  ```
- A structured startup log line on the operator (`"capability delivery mode
  resolved"`).

Attached Agents also carry a `flokoa.ai/capability-delivery: <mode>` pod
annotation for at-a-glance inspection of an individual pod.

!!! note "Operator note: cold-start scaling"
    In initContainer mode, init containers run **sequentially** per pod, so on
    a cold node the wheelhouse image pulls add up before the agent container
    starts. This is fine for a handful of small, pure-Python capabilities. For
    a heavy dependency closure, the recommended escape hatch is to **bake the
    wheels into a custom runner image** rather than delivering them as
    capabilities. ImageVolume mode removes the sequential-pull cost entirely
    where it is available.

## Signature verification

Digest pinning (enforced at admission) already gives **integrity**: content
addressing means a registry can at worst deny service, never alter delivered
bytes. Cosign verification is the optional layer that adds **provenance** — *who*
published this digest — and drives the `Verified` condition. It runs at
Capability reconcile, never in the pod path, and is sound to cache because
digests are immutable.

Enable it with the Helm value `capabilities.verification.cosign.enabled: true`,
then configure **exactly one** mode:

- **Key-based** — `capabilities.verification.cosign.keySecretRef`: the name of
  a Secret in the operator namespace holding the cosign public key under the
  `cosign.pub` key. The chart mounts it read-only into the controller.
- **Keyless (Fulcio/Rekor)** — both `capabilities.verification.cosign.keyless.issuer`
  (the exact OIDC issuer, e.g. `https://token.actions.githubusercontent.com`)
  and `capabilities.verification.cosign.keyless.identityRegexp` (a regexp the
  certificate SAN must match) are **required**. Identity-free or wildcard-only
  keyless config is rejected at startup — a keyless signature with no identity
  policy proves only that *someone* signed the digest.

For private artifact registries, `capabilities.verification.registrySecretRef`
names a `dockerconfigjson` Secret in the operator namespace used during
verification.

### The `Verified` condition state machine

| Status / Reason | Meaning |
|---|---|
| `Unknown` / `VerificationDisabled` | Cosign is not enabled on this cluster (the default). |
| `True` / `SignatureVerified` | A signature verified against the configured policy; the verified digest is in the condition message. |
| `False` / `SignatureMissing` | No signature exists at the expected location. |
| `False` / `SignatureInvalid` | A signature exists but does not verify (bad signature, wrong digest claim, or an identity outside the policy). |
| `Unknown` / `VerifyError` | A **transient** failure (registry or sigstore trust root unavailable). Re-queued with backoff — never flips to `False` on a registry blip. |

`kubectl get capabilities` shows the `Verified` column directly.

### The `requireVerified` cluster policy

`capabilities.verification.requireVerified: true` turns
`Verified != True` into a deployment-blocking policy, enforced at **both** gates
(the same belt-and-braces pattern as the compatibility checks above):

1. **Admission** — attaching a Capability whose `Verified` condition is not
   `True` is denied. Under this policy a *missing* Capability CR is also a
   denial (an unverifiable capability must never deploy), and a transient
   `Unknown/VerifyError` denies with a retryable "verification in flight"
   message rather than a hard failure.
2. **Compile** — the compiler re-checks the condition when a Capability is
   edited after Agent admission, surfacing as `SpecValid=False` on the next
   reconcile. Already-running pods keep running (last-good-generation
   semantics).

The chart **refuses `requireVerified: true` without `cosign.enabled: true`** —
without verification the `Verified` condition stays `Unknown` forever and the
policy would brick every attachment. The operator binary enforces the same
guard and refuses to start on the equivalent flag combination.

Clusters already running the sigstore **policy-controller** get an equivalent
provenance gate at the pod-admission layer; the two are complementary, and
running both is redundant rather than conflicting.

!!! note "Operator note: accepted residual risk"
    In **initContainer** mode the artifact's `manifest.json` is not
    *independently* integrity-bound during the brief `emptyDir` copy window
    (the digest pin covers the image bytes, but the manifest is read from the
    copied directory). This window is closed in practice by two controls: the
    delivery override policy forbids user sidecars and init containers in agent
    pods (so only operator-emitted containers ever write the volume), and the
    runner verifies every wheel's sha256 against the manifest and rejects
    unlisted or non-wheel files at bootstrap. The residual exposure is at the
    "rogue namespace admin" tier and is accepted, not silently ignored. Full
    closure would require an additional CRD field or injected digest env.

## Current limits (roadmap 10)

The `flokoa capability build/push/import/search` CLI (roadmap 10) automates
authoring and publishing artifacts. Registry **seeding** (publishing a
first-party capability set, e.g. `flokoa-openapi`, to a public registry) is
deferred. Until seeding lands you build and push capability artifacts
yourself; the admission, delivery, and verification machinery described above
is fully wired.
