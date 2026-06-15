// api/v1alpha1/capability_types.go

package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SchemaPolicy selects how a Capability's per-agent config is validated.
// +kubebuilder:validation:Enum=strict;permissive
type SchemaPolicy string

const (
	// SchemaPolicyStrict requires a published ConfigSchema; agent config is
	// validated against it at admission.
	SchemaPolicyStrict SchemaPolicy = "strict"
	// SchemaPolicyPermissive is the loud opt-out: config is accepted without
	// schema validation, and the capability is visibly flagged in status and
	// CLI output (product brief §4).
	SchemaPolicyPermissive SchemaPolicy = "permissive"
)

// CapabilitySource records where the capability's code came from, safest →
// riskiest. It drives the artifact-required/forbidden rule, the allowedSources
// cluster policy, and the Source printcolumn. It is provenance metadata: it
// records the origin the build asserted, not a cryptographic proof of it
// (cosign/Verified does provenance-of-the-digest).
// +kubebuilder:validation:Enum=builtin;image;git;pypi
type CapabilitySource string

const (
	// CapabilitySourceBuiltin: a first-party capability baked into the runner
	// image. No artifact, nothing downloaded at deploy/run time.
	CapabilitySourceBuiltin CapabilitySource = "builtin"
	// CapabilitySourceImage: the author built their own capability with the
	// flokoa-capability-base image as the build environment.
	CapabilitySourceImage CapabilitySource = "image"
	// CapabilitySourceGit: built from a (typically private) git repo at build
	// time; the artifact is normal, self-contained, and signable.
	CapabilitySourceGit CapabilitySource = "git"
	// CapabilitySourcePypi: built from a PyPI package — the EXTREMELY DANGEROUS
	// tier (arbitrary maintainers, no first-party vetting). Opt-in to build,
	// cluster-refusable to deploy.
	CapabilitySourcePypi CapabilitySource = "pypi"
)

// Condition types surfaced on Capability status.
const (
	// CapabilityConditionVerified reports artifact signature verification
	// (cosign at controller reconcile; roadmap 09).
	CapabilityConditionVerified = "Verified"
	// CapabilityConditionPermissive loudly surfaces schemaPolicy: permissive.
	CapabilityConditionPermissive = "Permissive"
)

// Reasons on the Verified condition. Shared constants because the Agent
// webhook and the spec compiler read the condition to enforce the
// requireVerified cluster policy and must phrase denials by reason
// (an in-flight Unknown reads as retryable, never as "invalid").
const (
	// CapabilityVerifiedReasonDisabled: cosign verification is not enabled
	// on this cluster (Verified=Unknown).
	CapabilityVerifiedReasonDisabled = "VerificationDisabled"
	// CapabilityVerifiedReasonVerified: the artifact signature verified
	// (Verified=True); the condition message records the verified digest.
	CapabilityVerifiedReasonVerified = "SignatureVerified"
	// CapabilityVerifiedReasonMissing: no signature exists for the artifact
	// (Verified=False, definitive for this digest).
	CapabilityVerifiedReasonMissing = "SignatureMissing"
	// CapabilityVerifiedReasonInvalid: a signature exists but did not verify
	// (Verified=False, definitive for this digest).
	CapabilityVerifiedReasonInvalid = "SignatureInvalid"
	// CapabilityVerifiedReasonError: verification could not complete
	// (registry blip, trust-root fetch failure); Verified=Unknown and the
	// controller retries with backoff.
	CapabilityVerifiedReasonError = "VerifyError"
	// CapabilityVerifiedReasonBuiltIn: the capability is built into the runner
	// image (source: builtin) — it has no separate artifact to cosign-verify;
	// integrity rides on the runner image digest. Verified=True so requireVerified
	// clusters keep working with the most-trusted tier (§8.3).
	CapabilityVerifiedReasonBuiltIn = "BuiltIn"
)

// CapabilityRequires is the compatibility tuple mirrored from the artifact
// manifest (runtime contract §5): the webhook refuses incompatible
// attachments at admission; the runner re-checks at install.
type CapabilityRequires struct {
	// Python is the required Python minor (exact match), e.g. "3.13".
	// +kubebuilder:validation:Pattern=`^\d+\.\d+$`
	// +optional
	Python string `json:"python,omitempty"`

	// PydanticAI is a PEP 440 specifier set for the runner's pydantic-ai
	// core pin, e.g. ">=1.107,<2".
	// +optional
	PydanticAI string `json:"pydanticAI,omitempty"`

	// FlokoaRunner is a PEP 440 specifier set for the runner release,
	// e.g. ">=0.2".
	// +optional
	FlokoaRunner string `json:"flokoaRunner,omitempty"`
}

// CapabilityProvenance carries signature/attestation metadata mirrored from
// the artifact. Verification mechanics (cosign at controller reconcile) land
// with roadmap 09. Source-origin provenance (git/pypi) records where the code
// came from at build time; it carries no credential material.
type CapabilityProvenance struct {
	// SignatureRef optionally records where the artifact's cosign signature
	// lives when it is not the default sidecar tag in the artifact repository.
	// +optional
	SignatureRef string `json:"signatureRef,omitempty"`

	// Git records the git origin a `source: git` capability was built from
	// (repo URL + resolved commit). The resolved commit is the durable record:
	// even if the branch/tag moves, this captures the exact code built.
	// +optional
	Git *CapabilityGitProvenance `json:"git,omitempty"`

	// Pypi records the PyPI requirement a `source: pypi` capability was built
	// from.
	// +optional
	Pypi *CapabilityPypiProvenance `json:"pypi,omitempty"`
}

// CapabilityGitProvenance records the git origin of a source: git capability.
// It never carries credential material — only the clean repo URL and the
// resolved commit (runtime contract §4.3).
type CapabilityGitProvenance struct {
	// URL is the clean repository URL (no embedded credentials).
	URL string `json:"url"`
	// Ref is the requested branch, tag, or commit (informational).
	// +optional
	Ref string `json:"ref,omitempty"`
	// Commit is the resolved git commit the artifact was built from.
	// +kubebuilder:validation:Pattern=`^[a-f0-9]{7,40}$`
	Commit string `json:"commit"`
	// Subdirectory is the path within the repo to the Python project, if any.
	// +optional
	Subdirectory string `json:"subdirectory,omitempty"`
}

// CapabilityPypiProvenance records the PyPI requirement a source: pypi
// capability was built from.
type CapabilityPypiProvenance struct {
	// Requirement is the resolved PyPI requirement, e.g. "some-pkg==1.2.0".
	Requirement string `json:"requirement"`
}

// CapabilitySpec defines the desired state of a Capability: a versioned,
// digest-pinned, schema-published unit of agent behavior (product brief §4).
// The spec mirrors the artifact manifest by value (schema, requires,
// dependencies) so admission stays offline and air-gap-friendly; the
// controller's job is verifying mirror ↔ artifact agreement, not fetching at
// admission time. `flokoa capability push` (roadmap 10) generates this CR
// from the manifest, so the mirror never drifts in practice.
type CapabilitySpec struct {
	// Source records where the capability's code came from (builtin|image|
	// git|pypi). Defaults to image (the author-built-their-own case). The
	// artifact field is REQUIRED for image/git/pypi and FORBIDDEN for builtin
	// (built-in capabilities are baked into the runner image, never delivered).
	// +kubebuilder:default=image
	// +optional
	Source CapabilitySource `json:"source,omitempty"`

	// Artifact is the OCI reference of the wheelhouse artifact image
	// (runtime contract §4). MUST be digest-pinned when present. Required for
	// source image/git/pypi; forbidden for source builtin (no artifact is
	// delivered — the code is in the runner image).
	// +kubebuilder:validation:Pattern=`^$|@sha256:[a-f0-9]{64}$`
	// +optional
	Artifact string `json:"artifact,omitempty"`

	// Version is the capability's own semantic version (matches the artifact
	// manifest).
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`

	// Entrypoint is the Python `module:attr` resolving to the capability
	// class (a pydantic-ai AbstractCapability subclass). The attr must be the
	// class itself, bound in the module under its own __name__ — no factories
	// or re-export aliases — so the compiled spec entry name (the class name,
	// pydantic-ai's default) resolves at hydration. If the class overrides
	// get_serialization_name(), set serializationName to match.
	// +kubebuilder:validation:Pattern=`^[\w.]+:[A-Za-z_]\w*$`
	Entrypoint string `json:"entrypoint"`

	// SerializationName is the capability's spec-entry name when the class
	// overrides pydantic-ai's default (the class name). Compiled specs
	// reference the capability by this name; defaults to the attr part of
	// entrypoint. It may carry a first-party dotted namespace (e.g.
	// flokoa.OpenAPI for built-ins) but no '/' or ':' path punctuation; the
	// operator-injected flokoa.platform/ prefix is reserved. The webhook
	// enforces the bare-class-name rule for user (image/git/pypi) capabilities
	// and the dot-tolerant rule for source: builtin.
	// +kubebuilder:validation:Pattern=`^[A-Za-z_][\w.]*$`
	// +optional
	SerializationName string `json:"serializationName,omitempty"`

	// ConfigSchema is the JSON Schema for per-agent config, validated
	// offline at admission. Required unless schemaPolicy is permissive.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	ConfigSchema *apiextensionsv1.JSON `json:"configSchema,omitempty"`

	// SchemaPolicy: strict (default) requires ConfigSchema; permissive is
	// the loud opt-out.
	// +kubebuilder:default=strict
	// +optional
	SchemaPolicy SchemaPolicy `json:"schemaPolicy,omitempty"`

	// Requires is the compatibility tuple, mirrored from the artifact
	// manifest.
	Requires CapabilityRequires `json:"requires"`

	// Dependencies mirrors the artifact's pinned dependency closure
	// (name==version) for admission-time conflict detection without registry
	// access.
	// +kubebuilder:validation:items:Pattern=`^[A-Za-z0-9][A-Za-z0-9._-]*==[A-Za-z0-9._+!-]+$`
	// +optional
	Dependencies []string `json:"dependencies,omitempty"`

	// Provenance carries signature/attestation metadata (cosign verification
	// config lands with roadmap 09).
	// +optional
	Provenance *CapabilityProvenance `json:"provenance,omitempty"`
}

// CapabilityStatus defines the observed state of a Capability.
type CapabilityStatus struct {
	// Conditions represent the latest available observations of the
	// capability's state (Verified, Permissive).
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the
	// controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=cap
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.version"
// +kubebuilder:printcolumn:name="Source",type="string",JSONPath=".spec.source"
// +kubebuilder:printcolumn:name="Runner",type="string",JSONPath=".spec.requires.flokoaRunner"
// +kubebuilder:printcolumn:name="Policy",type="string",JSONPath=".spec.schemaPolicy"
// +kubebuilder:printcolumn:name="Verified",type="string",JSONPath=".status.conditions[?(@.type=='Verified')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Capability is the Schema for the capabilities API: a versioned,
// digest-pinned unit of agent behavior attachable to Agents via
// spec.capabilities, with machine-checked compatibility at admission.
type Capability struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CapabilitySpec   `json:"spec,omitempty"`
	Status CapabilityStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CapabilityList contains a list of Capability
type CapabilityList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Capability `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Capability{}, &CapabilityList{})
}
