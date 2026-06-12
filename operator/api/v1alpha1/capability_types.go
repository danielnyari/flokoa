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

// Condition types surfaced on Capability status.
const (
	// CapabilityConditionVerified reports artifact digest/signature
	// verification (controller-side checks land with roadmap 09).
	CapabilityConditionVerified = "Verified"
	// CapabilityConditionPermissive loudly surfaces schemaPolicy: permissive.
	CapabilityConditionPermissive = "Permissive"
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
// with roadmap 09.
type CapabilityProvenance struct {
	// SignatureRef optionally records where the artifact's cosign signature
	// lives when it is not the default sidecar tag in the artifact repository.
	// +optional
	SignatureRef string `json:"signatureRef,omitempty"`
}

// CapabilitySpec defines the desired state of a Capability: a versioned,
// digest-pinned, schema-published unit of agent behavior (product brief §4).
// The spec mirrors the artifact manifest by value (schema, requires,
// dependencies) so admission stays offline and air-gap-friendly; the
// controller's job is verifying mirror ↔ artifact agreement, not fetching at
// admission time. `flokoa capability push` (roadmap 10) generates this CR
// from the manifest, so the mirror never drifts in practice.
type CapabilitySpec struct {
	// Artifact is the OCI reference of the wheelhouse artifact image
	// (runtime contract §4). MUST be digest-pinned.
	// +kubebuilder:validation:Pattern=`@sha256:[a-f0-9]{64}$`
	Artifact string `json:"artifact"`

	// Version is the capability's own semantic version (matches the artifact
	// manifest).
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`

	// Entrypoint is the Python `module:attr` resolving to the capability
	// class (a pydantic-ai AbstractCapability subclass).
	// +kubebuilder:validation:Pattern=`^[\w.]+:[\w.]+$`
	Entrypoint string `json:"entrypoint"`

	// SerializationName is the capability's spec-entry name when the class
	// overrides pydantic-ai's default (the class name). Compiled specs
	// reference the capability by this name; defaults to the attr part of
	// entrypoint.
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
// +kubebuilder:printcolumn:name="Runner",type="string",JSONPath=".spec.requires.flokoaRunner"
// +kubebuilder:printcolumn:name="Policy",type="string",JSONPath=".spec.schemaPolicy"
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
