/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ModelSpec defines the desired state of Model: a named, shareable model
// configuration that compiles to AgentSpec `model` + `model_settings`.
// Rotating one Model CR recompiles every Agent that references it.
type ModelSpec struct {
	// Model is the model identifier (e.g., "gpt-5-mini",
	// "claude-sonnet-4-5"). The pydantic-ai provider prefix is derived from
	// the referenced ModelProvider unless the identifier already contains
	// one (e.g. "openai:gpt-5-mini").
	// +kubebuilder:validation:MinLength=1
	Model string `json:"model"`

	// ProviderRef references the ModelProvider resource that supplies
	// connection configuration (API key secret, base URL).
	ProviderRef ProviderRef `json:"providerRef"`

	// Settings contains the model settings applied to every request.
	// +optional
	Settings *ModelSettings `json:"settings,omitempty"`
}

// ProviderRef references a ModelProvider resource
type ProviderRef struct {
	// Name of the ModelProvider resource
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace of the ModelProvider (defaults to Model's namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ModelSettings mirrors pydantic-ai's common ModelSettings fields with typed
// schemas. Provider-specific knobs (extra_body, thinking levels, service
// tiers, …) flow through Extra and merge into the compiled `model_settings`
// object as-is. Decimal fields use strings to avoid floating-point drift in
// the Kubernetes API; the compiler emits them as JSON numbers.
type ModelSettings struct {
	// MaxTokens is the maximum number of tokens to generate.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxTokens *int32 `json:"maxTokens,omitempty"`

	// Temperature controls randomness in the output (0.0 to 2.0), e.g. "0.7".
	// +kubebuilder:validation:Pattern=`^(0(\.\d+)?|1(\.\d+)?|2(\.0+)?)$`
	// +optional
	Temperature string `json:"temperature,omitempty"`

	// TopP controls nucleus sampling (0.0 to 1.0), e.g. "0.95".
	// +kubebuilder:validation:Pattern=`^(0(\.\d+)?|1(\.0+)?)$`
	// +optional
	TopP string `json:"topP,omitempty"`

	// TopK limits the number of tokens to consider for each step.
	// +kubebuilder:validation:Minimum=1
	// +optional
	TopK *int32 `json:"topK,omitempty"`

	// TimeoutSeconds bounds each model request.
	// +kubebuilder:validation:Minimum=1
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	// ParallelToolCalls enables parallel tool calling where supported.
	// +optional
	ParallelToolCalls *bool `json:"parallelToolCalls,omitempty"`

	// Seed for deterministic generation (where supported).
	// +optional
	Seed *int64 `json:"seed,omitempty"`

	// PresencePenalty penalizes tokens already present in the context
	// (-2.0 to 2.0), e.g. "0.5".
	// +kubebuilder:validation:Pattern=`^-?[0-2](\.\d+)?$`
	// +optional
	PresencePenalty string `json:"presencePenalty,omitempty"`

	// FrequencyPenalty penalizes frequent tokens (-2.0 to 2.0), e.g. "0.5".
	// +kubebuilder:validation:Pattern=`^-?[0-2](\.\d+)?$`
	// +optional
	FrequencyPenalty string `json:"frequencyPenalty,omitempty"`

	// LogitBias modifies the likelihood of specified tokens appearing.
	// +optional
	LogitBias map[string]int32 `json:"logitBias,omitempty"`

	// StopSequences are sequences where the model will stop generating.
	// +optional
	StopSequences []string `json:"stopSequences,omitempty"`

	// ExtraHeaders to include in all requests.
	// +optional
	ExtraHeaders map[string]string `json:"extraHeaders,omitempty"`

	// Extra merges additional provider-specific settings into the compiled
	// model_settings object (e.g. extra_body, thinking, service_tier).
	// Typed fields win on key conflicts.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Extra *apiextensionsv1.JSON `json:"extra,omitempty"`
}

// =============================================================================
// Model Status
// =============================================================================

// ModelStatus defines the observed state of Model
type ModelStatus struct {
	// Standard conditions
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Observed generation
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ResolvedProvider contains the resolved provider information
	// +optional
	ResolvedProvider *ResolvedProviderInfo `json:"resolvedProvider,omitempty"`

	// Ready indicates if the model is ready to be used
	// +optional
	Ready bool `json:"ready,omitempty"`
}

// ResolvedProviderInfo contains information about the resolved ModelProvider
type ResolvedProviderInfo struct {
	// Provider is the resolved provider type
	Provider ProviderType `json:"provider,omitempty"`

	// Namespace of the referenced ModelProvider
	Namespace string `json:"namespace,omitempty"`

	// Name of the referenced ModelProvider
	Name string `json:"name,omitempty"`
}

// =============================================================================
// Model CRD
// =============================================================================

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Model",type="string",JSONPath=".spec.model"
// +kubebuilder:printcolumn:name="Provider",type="string",JSONPath=".status.resolvedProvider.provider"
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Model is the Schema for the models API.
// It defines a specific LLM model with its settings, referencing a ModelProvider for connection configuration.
type Model struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelSpec   `json:"spec,omitempty"`
	Status ModelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ModelList contains a list of Model
type ModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Model `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Model{}, &ModelList{})
}
