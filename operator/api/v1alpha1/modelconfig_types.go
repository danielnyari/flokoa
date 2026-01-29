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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ReasoningEffort represents the reasoning effort level for OpenAI models
// +kubebuilder:validation:Enum=low;medium;high
type ReasoningEffort string

const (
	ReasoningEffortLow    ReasoningEffort = "low"
	ReasoningEffortMedium ReasoningEffort = "medium"
	ReasoningEffortHigh   ReasoningEffort = "high"
)

// ThinkingType represents the type of thinking configuration for Anthropic models
// +kubebuilder:validation:Enum=enabled;disabled
type ThinkingType string

const (
	ThinkingTypeEnabled  ThinkingType = "enabled"
	ThinkingTypeDisabled ThinkingType = "disabled"
)

// ModelConfigSpec defines the desired state of ModelConfig
type ModelConfigSpec struct {
	// ModelRef references the Model resource to use
	ModelRef ModelRef `json:"modelRef"`

	// Common model parameters applicable to most providers
	// +optional
	Parameters *ModelParameters `json:"parameters,omitempty"`

	// OpenAI-specific parameters
	// +optional
	OpenAI *OpenAIParameters `json:"openai,omitempty"`

	// Anthropic-specific parameters
	// +optional
	Anthropic *AnthropicParameters `json:"anthropic,omitempty"`

	// Gemini-specific parameters
	// +optional
	Gemini *GeminiParameters `json:"gemini,omitempty"`
}

// ModelRef references a Model resource
type ModelRef struct {
	// Name of the Model resource
	Name string `json:"name"`

	// Namespace of the Model (defaults to ModelConfig's namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ModelParameters defines common model parameters
type ModelParameters struct {
	// Temperature controls randomness in the output (0.0 to 2.0)
	// Specified as a string to avoid floating point issues (e.g., "0.7", "1.0")
	// +optional
	Temperature string `json:"temperature,omitempty"`

	// MaxTokens is the maximum number of tokens to generate
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxTokens *int32 `json:"maxTokens,omitempty"`

	// TopP controls nucleus sampling (0.0 to 1.0)
	// Specified as a string to avoid floating point issues (e.g., "0.9", "0.95")
	// +optional
	TopP string `json:"topP,omitempty"`

	// TopK limits the number of tokens to consider for each step
	// +kubebuilder:validation:Minimum=1
	// +optional
	TopK *int32 `json:"topK,omitempty"`

	// StopSequences are sequences where the model will stop generating
	// +optional
	StopSequences []string `json:"stopSequences,omitempty"`

	// Seed for deterministic generation (where supported)
	// +optional
	Seed *int64 `json:"seed,omitempty"`
}

// OpenAIParameters defines OpenAI-specific model parameters
type OpenAIParameters struct {
	// FrequencyPenalty penalizes frequent tokens (-2.0 to 2.0)
	// Specified as a string to avoid floating point issues (e.g., "0.5", "-1.0")
	// +optional
	FrequencyPenalty string `json:"frequencyPenalty,omitempty"`

	// PresencePenalty penalizes tokens already in the context (-2.0 to 2.0)
	// Specified as a string to avoid floating point issues (e.g., "0.5", "-1.0")
	// +optional
	PresencePenalty string `json:"presencePenalty,omitempty"`

	// ReasoningEffort controls the reasoning effort for reasoning models (o1, o3, etc.)
	// +optional
	ReasoningEffort *ReasoningEffort `json:"reasoningEffort,omitempty"`

	// ResponseFormat specifies the output format
	// +optional
	ResponseFormat *OpenAIResponseFormat `json:"responseFormat,omitempty"`

	// LogProbs enables returning log probabilities
	// +optional
	LogProbs *bool `json:"logProbs,omitempty"`

	// TopLogProbs specifies how many log probabilities to return (0-20)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=20
	// +optional
	TopLogProbs *int32 `json:"topLogProbs,omitempty"`
}

// OpenAIResponseFormat specifies the response format for OpenAI models
type OpenAIResponseFormat struct {
	// Type of response format: "text" or "json_object"
	// +kubebuilder:validation:Enum=text;json_object;json_schema
	Type string `json:"type"`

	// JSONSchema for structured outputs (when type is "json_schema")
	// +optional
	JSONSchema *OpenAIJSONSchema `json:"jsonSchema,omitempty"`
}

// OpenAIJSONSchema defines the JSON schema for structured outputs
type OpenAIJSONSchema struct {
	// Name of the schema
	Name string `json:"name"`

	// Description of the schema
	// +optional
	Description string `json:"description,omitempty"`

	// Schema is the JSON schema definition
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Schema *runtime.RawExtension `json:"schema,omitempty"`

	// Strict enables strict schema validation
	// +optional
	Strict *bool `json:"strict,omitempty"`
}

// AnthropicParameters defines Anthropic-specific model parameters
type AnthropicParameters struct {
	// Thinking configuration for extended thinking models
	// +optional
	Thinking *ThinkingConfig `json:"thinking,omitempty"`
}

// ThinkingConfig defines the thinking configuration for Anthropic models
type ThinkingConfig struct {
	// Type enables or disables extended thinking
	// +kubebuilder:default=disabled
	Type ThinkingType `json:"type"`

	// BudgetTokens is the maximum number of tokens for thinking.
	// When type is "enabled", this should be set and must be >= 1024.
	// It should also be less than maxTokens, but this is not enforced by the schema.
	// +kubebuilder:validation:Minimum=1024
	// +optional
	BudgetTokens *int32 `json:"budgetTokens,omitempty"`
}

// GeminiParameters defines Gemini-specific model parameters
type GeminiParameters struct {
	// CandidateCount specifies how many response candidates to generate
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=8
	// +optional
	CandidateCount *int32 `json:"candidateCount,omitempty"`

	// ResponseMimeType specifies the output MIME type
	// +optional
	ResponseMimeType string `json:"responseMimeType,omitempty"`

	// SafetySettings configures content safety filters
	// +optional
	SafetySettings []GeminiSafetySetting `json:"safetySettings,omitempty"`
}

// GeminiSafetySetting defines a safety setting for Gemini
type GeminiSafetySetting struct {
	// Category is the harm category
	// +kubebuilder:validation:Enum=HARM_CATEGORY_HARASSMENT;HARM_CATEGORY_HATE_SPEECH;HARM_CATEGORY_SEXUALLY_EXPLICIT;HARM_CATEGORY_DANGEROUS_CONTENT
	Category string `json:"category"`

	// Threshold is the blocking threshold
	// +kubebuilder:validation:Enum=BLOCK_NONE;BLOCK_LOW_AND_ABOVE;BLOCK_MEDIUM_AND_ABOVE;BLOCK_ONLY_HIGH
	Threshold string `json:"threshold"`
}

// ModelConfigStatus defines the observed state of ModelConfig
type ModelConfigStatus struct {
	// Standard conditions
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Observed generation
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ResolvedModel contains the resolved model information
	// +optional
	ResolvedModel *ResolvedModelInfo `json:"resolvedModel,omitempty"`

	// Ready indicates if the model config is ready to be used
	// +optional
	Ready bool `json:"ready,omitempty"`
}

// ResolvedModelInfo contains information about the resolved Model
type ResolvedModelInfo struct {
	// Provider from the referenced Model
	Provider ModelProvider `json:"provider,omitempty"`

	// Model name from the referenced Model
	Model string `json:"model,omitempty"`

	// Namespace of the referenced Model
	Namespace string `json:"namespace,omitempty"`

	// Name of the referenced Model
	Name string `json:"name,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Model",type="string",JSONPath=".spec.modelRef.name"
// +kubebuilder:printcolumn:name="Provider",type="string",JSONPath=".status.resolvedModel.provider"
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ModelConfig is the Schema for the modelconfigs API.
// It references a Model and defines specific parameters for a use case (e.g., an agent).
type ModelConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelConfigSpec   `json:"spec,omitempty"`
	Status ModelConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ModelConfigList contains a list of ModelConfig
type ModelConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModelConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ModelConfig{}, &ModelConfigList{})
}
