# 09 — Capability Artifacts & Delivery

**Phase:** P0b · **Size:** L · **Depends on:** 03, 08 · **Enables:** 10; runner consumption (05) gets its real input

## Goal

Define the OCI wheelhouse artifact format and deliver it into runner pods: **initContainer copy as the default, ImageVolume as the detected optimization** (brief §4, decision 4), with cosign verification and digest pinning throughout.

## Artifact format

An OCI **image** (not a raw OCI artifact — initContainers must run it, and image-ness keeps every registry/cache/scanner happy):

```
FROM busybox:stable-musl          # tiny static shell+cp; needed for the initContainer copy path
COPY wheelhouse/ /wheelhouse/     # capability wheel + pinned closure of non-baseline deps
COPY manifest.json /wheelhouse/manifest.json
```

- `manifest.json`: `{name, version, contractVersion, entrypoint, requires{python, pydantic-ai, flokoa-runner}, dependencies[name==version], wheels[{file, sha256}], schemaDigest}` — the source the Capability CR mirrors (08).
- Multi-arch via manifest lists (amd64/arm64); wheels per-arch where non-pure (built by 10's `capability build` inside the runner image per-arch).
- **Boundary, documented loudly**: wheels only. System deps (apt packages, binaries) → custom agent images (brief §4).

## Delivery

### Default path: initContainer

Per attached Capability, the builder (extending 04's `DeploymentParams`) adds:

```yaml
initContainers:
- name: cap-<name>
  image: <artifact@sha256:…>            # the digest from the Capability CR
  command: ["/bin/sh","-c","cp -r /wheelhouse/* /opt/flokoa/capabilities/<name>/"]
  volumeMounts: [{name: flokoa-capabilities, mountPath: /opt/flokoa/capabilities}]
  resources: {…minimal…}
  securityContext: {readOnlyRootFilesystem: true, runAsNonRoot: true, …}
```

into a shared `emptyDir` mounted read-only into the runner at `/opt/flokoa/capabilities/` (03's layout). Kubelet image caching makes repeat starts cheap; works on every cluster.

### Fast path: ImageVolume

When enabled (Helm value `capabilities.delivery: imageVolume|initContainer|auto`; `auto` probes — see below), each artifact mounts as a read-only `image:` volume at the same path; no initContainers, no copy, kubelet-cached layers shared across pods. Same digests, same verification, same runner install code (`pip install --no-index --find-links`).

- **Feature detection for `auto`**: attempt is the only reliable probe — the operator creates a tiny probe pod with an `image:` volume at startup (or first use) and records the result in an operator-level condition + metric; failure falls back to initContainer silently. Re-probed on operator restart (cluster upgrades change the answer). Per-cluster, not per-agent.
- Requirements documented honestly: feature gate (beta), containerd 2.x, patchy managed-cloud support in mid-2026.

### Verification

- **Admission already pinned the digest** (08); the kubelet pulls by digest — content addressing does the integrity work.
- **Cosign (optional, Helm-gated)**: `capabilities.verification.cosign.enabled` + key/keyless config → the Capability *controller* verifies the signature against the digest at CR reconcile (sets the `Verified` condition; 08); a cluster policy flag makes `Verified=False` block Agent admission. Verification at reconcile-time (once, surfaced in status) — not in the pod path — keeps pods fast and failures legible. Clusters running sigstore policy-controller get the same enforcement at a different layer; document both.

### Runner consumption (already specced in 05, restated as contract)

For each `/opt/flokoa/capabilities/<name>/`: read `manifest.json`, defense-in-depth `requires` re-check, `pip install --no-index --find-links <dir> <wheel pins>` into the pod-local env, resolve `entrypoint`. Wheel sha256s verified before install (the manifest carries them — cheap, and it covers the emptyDir copy path).

## Implementation plan

1. Artifact format doc (section in the runtime contract, 03) + a reference artifact fixture used by all tests (built in CI from a trivial capability).
2. Builder: initContainer + volume emission from compiled attachments (pure function, fakes-tested); naming/collision rules (`<name>` = Capability CR name, DNS-safe).
3. ImageVolume path + probe + Helm value + operator condition.
4. Capability controller verification (cosign via sigstore-go, optional dep) + `Verified` condition.
5. Runner: sha256 verification + install hardening (already 05's `install_capabilities`; this unit supplies the real fixtures and the negative cases).
6. E2E: Kind (initContainer path) — Agent + 2 Capabilities → pods start, tools present; ImageVolume path behind a Kind feature-gate config in a separate CI job (advisory until GA).

## Acceptance criteria

- An Agent referencing two published Capability artifacts starts on a vanilla Kind cluster (initContainer path) with both capabilities importable; flipping the Helm value to ImageVolume on a supporting cluster changes delivery with zero manifest changes for users.
- A tampered wheelhouse (sha mismatch) fails bootstrap with a structured error; an unsigned artifact under cosign-required policy never deploys.

## Out of scope

- Building/pushing artifacts (10). Registry hosting/index (10). Capability *updates* semantics beyond digest-pin replacement (a new digest is a new spec → normal 04 rollout).
