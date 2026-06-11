# 08 — Capability CRD + Admission

**Phase:** P0b · **Size:** L · **Depends on:** 03 (requires-tuple semantics), 02 (webhooks installable) · **Enables:** 09, 10; Agent attachment via 04's `capabilities[]`

## Goal

The Capability CRD of brief §4: a versioned, digest-pinned, schema-published unit of agent behavior — with admission that makes the compatibility matrix machine-checked: config-schema validation, `requires`-tuple checks, and dependency-conflict detection, all **before** anything deploys.

## Target CRD

```go
type CapabilitySpec struct {
    // Artifact is the OCI reference of the wheelhouse artifact. MUST be digest-pinned.
    // +kubebuilder:validation:Pattern=`@sha256:[a-f0-9]{64}$`
    Artifact string `json:"artifact"`

    // Version is the capability's own semantic version (matches the artifact manifest).
    Version string `json:"version"`

    // Entrypoint is the Python `module:attr` resolving to the capability class/factory.
    // +kubebuilder:validation:Pattern=`^[\w.]+:[\w.]+$`
    Entrypoint string `json:"entrypoint"`

    // ConfigSchema is the JSON Schema for per-agent config (offline admission validation).
    // +kubebuilder:pruning:PreserveUnknownFields
    // +optional
    ConfigSchema *apiextensionsv1.JSON `json:"configSchema,omitempty"`

    // SchemaPolicy: strict (default) requires ConfigSchema; permissive is the loud opt-out.
    // +kubebuilder:default=strict
    // +kubebuilder:validation:Enum=strict;permissive
    SchemaPolicy string `json:"schemaPolicy,omitempty"`

    // Requires is the compatibility tuple, mirrored from the artifact manifest.
    Requires CapabilityRequires `json:"requires"` // {python, pydanticAI, flokoaRunner — PEP 440 specifiers}

    // Dependencies mirrors the artifact's pinned dep closure (name==version) for
    // admission-time conflict detection without registry access.
    // +optional
    Dependencies []string `json:"dependencies,omitempty"`

    // Provenance: signature/attestation metadata (cosign verification config in 09).
    // +optional
    Provenance *CapabilityProvenance `json:"provenance,omitempty"`
}
```

Status: `Verified` condition (artifact digest/signature checks from 09's controller-side verification), `Permissive` condition (loud surfacing of `schemaPolicy: permissive` — brief §4 requires it visible in status/CLI/UI), printcolumns for version/runner-range/policy.

Design notes: spec mirrors the artifact manifest **by value** (schema, requires, deps) precisely so admission is offline and air-gap-friendly; the controller's job is verifying mirror ↔ artifact agreement (digest of manifest recorded in status), not fetching at admission time. `flokoa capability push` (10) generates the CR from the manifest, so the mirror never drifts in practice.

## Admission (three checks, all with precise errors)

Extends the existing CustomValidator pattern; the Agent webhook gains capability-aware validation (the webhook needs Capability CR reads — use a webhook with a client, as `internal/webhook/v1alpha1/agentworkflow_webhook.go` already demonstrates for cross-resource validation):

1. **Config-schema validation** (on Agent create/update): each `capabilities[].config` validates against the referenced Capability's `ConfigSchema` (santhosh-tekuri/jsonschema, shared with 03's validator) — with `${secret:NAME}` placeholders treated as satisfying `type: string` (admission validates *shape*, values resolve in the runner; brief §3). `permissive` capabilities skip with an admission **warning**.
2. **Requires check**: capability `requires` tuples ∩ the Agent's resolved runner version's manifest (03). Incompatible → denial naming both tuples.
3. **Dep-conflict detection**: union `Dependencies` across all attached Capabilities + the runner baseline lockfile; conflicting pins (same package, different versions; or pin colliding with baseline) → denial naming the two capabilities and versions. Two Capabilities pinning different `pydantic-ai-harness` versions is the canonical rejected case — *correct behavior, not a limitation* (brief §4).

## Implementation plan

1. CRD types + registration + `make manifests generate` + chart CRD sync + `make generate-python-models`.
2. `internal/domain/capability/`: requires-tuple evaluation (PEP 440 specifier matching in Go — use `aquasecurity/go-pep440-version` or implement the subset; table-tested), dep-union conflict detection, schema-validation helper.
3. Webhooks: Capability validator (digest-pinned artifact, entrypoint format, schema-policy coherence) + Agent validator extension (the three checks).
4. Compiler integration (04): attached capabilities compile into spec entries `{entrypoint, config, source: capability-cr}`; builder emits the delivery inputs for 09 (artifact ref per attachment).
5. Status/conditions + printcolumns; `Permissive` surfacing.

## Testing

- Domain tables: specifier matching, conflict matrices (incl. baseline collisions), placeholder-tolerant schema validation.
- Webhook envtest: each denial path with exact message assertions (these messages are product surface); permissive warning path.
- Integration with 04's compiler goldens: an Agent with two compatible capabilities compiles; the harness-version-conflict case denies at admission.

## Acceptance criteria

- Misconfigured capability config, incompatible runner, or conflicting deps **cannot reach a pod** — each fails admission with a message naming the offending refs/versions.
- `kubectl get capabilities` shows version, runner range, schema policy; permissive capabilities are visibly flagged.

## Out of scope

- Artifact format/delivery/verification mechanics (09). CLI/schema derivation (10). Per-namespace capability allow-listing (post-P1 policy work; note in 17).
