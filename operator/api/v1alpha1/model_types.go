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

// ModelSpec defines the desired state of Model.
// A Model represents a specific LLM model (e.g., gpt-4o, claude-sonnet-4-20250514) with its parameters.
type ModelSpec struct {
	// Model is the name/identifier of the model (e.g., "gpt-4o", "claude-sonnet-4-20250514")
	// +kubebuilder:validation:MinLength=1
	Model string `json:"model"`

	// ProviderRef references the ModelProvider resource that defines the provider configuration
	ProviderRef ProviderRef `json:"providerRef"`

	// Parameters contains base model parameters and optional provider-specific parameters
	// +optional
	Parameters *ModelParameters `json:"parameters,omitempty"`
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

// ModelParameters defines model parameters with base settings and optional provider-specific extensions.
// Only one provider-specific block should be set, matching the provider type of the referenced ModelProvider.
type ModelParameters struct {
	// Temperature controls randomness in the output (0.0 to 2.0)
	// Specified as a string to avoid floating point issues (e.g., "0.7", "1.0")
	// +kubebuilder:validation:Pattern=`^(0(\.\d+)?|1(\.0+)?|2(\.0+)?)$`
	// +optional
	Temperature string `json:"temperature,omitempty"`

	// MaxTokens is the maximum number of tokens to generate
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxTokens *int32 `json:"maxTokens,omitempty"`

	// TopP controls nucleus sampling (0.0 to 1.0)
	// Specified as a string to avoid floating point issues (e.g., "0.9", "0.95")
	// +kubebuilder:validation:Pattern=`^(0(\.\d+)?|1(\.0+)?)$`
	// +optional
	TopP string `json:"topP,omitempty"`

	// TopK limits the number of tokens to consider for each step
	// +kubebuilder:validation:Minimum=1
	// +optional
	TopK *int32 `json:"topK,omitempty"`

	// PresencePenalty penalizes tokens already present in the context (-2.0 to 2.0)
	// Specified as a string to avoid floating point issues (e.g., "0.5", "-1.0")
	// +kubebuilder:validation:Pattern=`^-?[0-2](\.\d+)?$`
	// +optional
	PresencePenalty string `json:"presencePenalty,omitempty"`

	// FrequencyPenalty penalizes frequent tokens (-2.0 to 2.0)
	// Specified as a string to avoid floating point issues (e.g., "0.5", "-1.0")
	// +kubebuilder:validation:Pattern=`^-?[0-2](\.\d+)?$`
	// +optional
	FrequencyPenalty string `json:"frequencyPenalty,omitempty"`

	// TimeOut in seconds for the model response
	// +kubebuilder:validation:Minimum=1
	// +optional
	TimeOut *int32 `json:"timeOut,omitempty"`

	// ParallelToolCalls enables parallel tool calling where supported
	// +optional
	ParallelToolCalls *bool `json:"parallelToolCalls,omitempty"`

	// LogitBias modifies the likelihood of specified tokens appearing
	// +optional
	LogitBias map[string]int32 `json:"logitBias,omitempty"`

	// StopSequences are sequences where the model will stop generating
	// +optional
	StopSequences []string `json:"stopSequences,omitempty"`

	// Seed for deterministic generation (where supported)
	// +optional
	Seed *int64 `json:"seed,omitempty"`

	// ExtraHeaders to include in all requests
	// +optional
	ExtraHeaders map[string]string `json:"extraHeaders,omitempty"`

	// ExtraBody to include in all requests
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	ExtraBody *runtime.RawExtension `json:"extraBody,omitempty"`

	// OpenAI contains OpenAI-specific parameters (only set for OpenAI providers)
	// +optional
	OpenAI *OpenAIParameters `json:"openai,omitempty"`

	// Anthropic contains Anthropic-specific parameters (only set for Anthropic providers)
	// +optional
	Anthropic *AnthropicParameters `json:"anthropic,omitempty"`

	// Google contains Google/Gemini-specific parameters (only set for Google providers)
	// +optional
	Google *GoogleParameters `json:"google,omitempty"`

	// Bedrock contains AWS Bedrock-specific parameters (only set for Bedrock providers)
	// +optional
	Bedrock *BedrockParameters `json:"bedrock,omitempty"`
}

// =============================================================================
// OpenAI-specific Parameters
// =============================================================================

// ReasoningEffort represents the reasoning effort level for OpenAI models
// +kubebuilder:validation:Enum=none;minimal;low;medium;high;xhigh
type ReasoningEffort string

const (
	ReasoningEffortNone    ReasoningEffort = "none"
	ReasoningEffortMinimal ReasoningEffort = "minimal"
	ReasoningEffortLow     ReasoningEffort = "low"
	ReasoningEffortMedium  ReasoningEffort = "medium"
	ReasoningEffortHigh    ReasoningEffort = "high"
	ReasoningEffortXHigh   ReasoningEffort = "xhigh"
)

// OpenAIParameters contains OpenAI-specific model parameters
type OpenAIParameters struct {
	// ReasoningEffort controls the reasoning effort for reasoning models (o1, o3, etc.)
	// +optional
	ReasoningEffort *ReasoningEffort `json:"reasoningEffort,omitempty"`

	// LogProbs enables returning log probabilities
	// +optional
	LogProbs *bool `json:"logProbs,omitempty"`

	// TopLogProbs specifies the number of top log probabilities to return
	// +optional
	TopLogProbs *int32 `json:"topLogProbs,omitempty"`

	// User is the user identifier associated with the requests
	// +optional
	User string `json:"user,omitempty"`

	// ServiceTier specifies the service tier for requests
	// +kubebuilder:validation:Enum=auto;default;flex;priority
	// +optional
	ServiceTier string `json:"serviceTier,omitempty"`

	// PromptCacheKey is the key for prompt caching
	// +optional
	PromptCacheKey string `json:"promptCacheKey,omitempty"`

	// PromptRetention controls prompt caching retention
	// +optional
	PromptRetention string `json:"promptRetention,omitempty"`

	// ReasoningGenerateSummary controls reasoning summary generation
	// +kubebuilder:validation:Enum=detailed;concise
	// +optional
	ReasoningGenerateSummary *string `json:"reasoningGenerateSummary,omitempty"`

	// ReasoningSummary controls reasoning summary format
	// +kubebuilder:validation:Enum=detailed;concise;auto
	// +optional
	ReasoningSummary *string `json:"reasoningSummary,omitempty"`

	// SendReasoningIDs enables sending reasoning IDs
	// +optional
	SendReasoningIDs *bool `json:"sendReasoningIDs,omitempty"`

	// Truncation controls response truncation
	// +kubebuilder:validation:Enum=disabled;auto
	// +optional
	Truncation *string `json:"truncation,omitempty"`

	// TextVerbosity controls text verbosity level
	// +kubebuilder:validation:Enum=low;medium;high
	// +optional
	TextVerbosity *string `json:"textVerbosity,omitempty"`

	// PreviousResponseID is the ID of a previous response to continue from
	// When set to "auto", automatically uses the most recent provider_response_id
	// +optional
	PreviousResponseID string `json:"previousResponseID,omitempty"`

	// IncludeCodeExecutionOutputs includes code execution results in the response
	// +optional
	IncludeCodeExecutionOutputs *bool `json:"includeCodeExecutionOutputs,omitempty"`

	// IncludeWebSearchSources includes web search results in the response
	// +optional
	IncludeWebSearchSources *bool `json:"includeWebSearchSources,omitempty"`

	// IncludeFileSearchResults includes file search results in the response
	// +optional
	IncludeFileSearchResults *bool `json:"includeFileSearchResults,omitempty"`

	// IncludeRawAnnotations includes raw annotations in the response
	// +optional
	IncludeRawAnnotations *bool `json:"includeRawAnnotations,omitempty"`
}

// =============================================================================
// Anthropic-specific Parameters
// =============================================================================

// ThinkingType represents the type of thinking configuration for Anthropic models
// +kubebuilder:validation:Enum=enabled;disabled
type ThinkingType string

const (
	ThinkingTypeEnabled  ThinkingType = "enabled"
	ThinkingTypeDisabled ThinkingType = "disabled"
)

// AnthropicThinkingConfig defines the thinking configuration for Anthropic models
type AnthropicThinkingConfig struct {
	// Type enables or disables extended thinking
	// +kubebuilder:default=disabled
	Type ThinkingType `json:"type"`

	// BudgetTokens is the maximum number of tokens for thinking (required when enabled)
	// Must be >= 1024 and less than maxTokens
	// +kubebuilder:validation:Minimum=1024
	// +optional
	BudgetTokens *int32 `json:"budgetTokens,omitempty"`
}

// AnthropicContainerConfig defines container configuration for multi-turn conversations
type AnthropicContainerConfig struct {
	// ID is the container ID to use (e.g., "container_xxx")
	// +optional
	ID string `json:"id,omitempty"`

	// Disabled forces a fresh container (ignores any container_id from history)
	// +optional
	Disabled *bool `json:"disabled,omitempty"`
}

// AnthropicParameters contains Anthropic-specific model parameters
type AnthropicParameters struct {
	// MetadataUserID is an external identifier for the user associated with the request
	// +optional
	MetadataUserID string `json:"metadataUserID,omitempty"`

	// Thinking determines whether the model should generate a thinking block
	// See https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking
	// +optional
	Thinking *AnthropicThinkingConfig `json:"thinking,omitempty"`

	// CacheToolDefinitions adds cache_control to the last tool definition
	// When true, uses TTL='5m'. Can also specify '5m' or '1h' directly.
	// +optional
	CacheToolDefinitions string `json:"cacheToolDefinitions,omitempty"`

	// CacheInstructions adds cache_control to the last system prompt block
	// When true, uses TTL='5m'. Can also specify '5m' or '1h' directly.
	// +optional
	CacheInstructions string `json:"cacheInstructions,omitempty"`

	// CacheMessages adds a cache point to the last content block in the final user message
	// When true, uses TTL='5m'. Can also specify '5m' or '1h' directly.
	// +optional
	CacheMessages string `json:"cacheMessages,omitempty"`

	// Container configures container for multi-turn conversations
	// +optional
	Container *AnthropicContainerConfig `json:"container,omitempty"`
}

// =============================================================================
// Google-specific Parameters
// =============================================================================

// GoogleThinkingLevel represents the thinking level for Google/Gemini models
// +kubebuilder:validation:Enum=unspecified;minimal;low;medium;high
type GoogleThinkingLevel string

const (
	GoogleThinkingLevelUnspecified GoogleThinkingLevel = "unspecified"
	GoogleThinkingLevelMinimal     GoogleThinkingLevel = "minimal"
	GoogleThinkingLevelLow         GoogleThinkingLevel = "low"
	GoogleThinkingLevelMedium      GoogleThinkingLevel = "medium"
	GoogleThinkingLevelHigh        GoogleThinkingLevel = "high"
)

// GoogleMediaResolution represents the media resolution for Google/Gemini models
// +kubebuilder:validation:Enum=unspecified;low;medium;high
type GoogleMediaResolution string

const (
	GoogleMediaResolutionUnspecified GoogleMediaResolution = "unspecified"
	GoogleMediaResolutionLow         GoogleMediaResolution = "low"
	GoogleMediaResolutionMedium      GoogleMediaResolution = "medium"
	GoogleMediaResolutionHigh        GoogleMediaResolution = "high"
)

// GoogleThinkingConfig defines the thinking configuration for Google/Gemini models
type GoogleThinkingConfig struct {
	// IncludeThoughts indicates whether to include thoughts in the response
	// +optional
	IncludeThoughts *bool `json:"includeThoughts,omitempty"`

	// ThinkingBudget is the thinking budget in tokens (0=DISABLED, -1=AUTOMATIC)
	// +optional
	ThinkingBudget *int32 `json:"thinkingBudget,omitempty"`

	// ThinkingLevel controls the amount of thinking tokens the model should generate
	// +optional
	ThinkingLevel *GoogleThinkingLevel `json:"thinkingLevel,omitempty"`
}

// GoogleSafetySetting defines a safety setting for Google/Gemini models
type GoogleSafetySetting struct {
	// Category is the harm category
	// +kubebuilder:validation:Enum=HARM_CATEGORY_HARASSMENT;HARM_CATEGORY_HATE_SPEECH;HARM_CATEGORY_SEXUALLY_EXPLICIT;HARM_CATEGORY_DANGEROUS_CONTENT;HARM_CATEGORY_CIVIC_INTEGRITY
	Category string `json:"category"`

	// Threshold is the blocking threshold
	// +kubebuilder:validation:Enum=BLOCK_NONE;BLOCK_LOW_AND_ABOVE;BLOCK_MEDIUM_AND_ABOVE;BLOCK_ONLY_HIGH;OFF
	Threshold string `json:"threshold"`

	// Method specifies if the threshold is used for probability or severity score
	// +kubebuilder:validation:Enum=SEVERITY;PROBABILITY
	// +optional
	Method string `json:"method,omitempty"`
}

// GoogleParameters contains Google/Gemini-specific model parameters
type GoogleParameters struct {
	// SafetySettings configures content safety filters
	// +optional
	SafetySettings []GoogleSafetySetting `json:"safetySettings,omitempty"`

	// ThinkingConfig configures the thinking behavior for the model
	// +optional
	ThinkingConfig *GoogleThinkingConfig `json:"thinkingConfig,omitempty"`

	// Labels are user-defined metadata to break down billed charges (Vertex AI only)
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// VideoResolution specifies the video resolution to use for the model
	// +optional
	VideoResolution *GoogleMediaResolution `json:"videoResolution,omitempty"`

	// CachedContent is the name of the cached content to use for the model
	// +optional
	CachedContent string `json:"cachedContent,omitempty"`
}

// =============================================================================
// Bedrock-specific Parameters
// =============================================================================

// BedrockGuardrailConfig defines content moderation and safety settings for Bedrock
type BedrockGuardrailConfig struct {
	// GuardrailIdentifier is the unique identifier of the guardrail
	// +optional
	GuardrailIdentifier string `json:"guardrailIdentifier,omitempty"`

	// GuardrailVersion is the version of the guardrail
	// +optional
	GuardrailVersion string `json:"guardrailVersion,omitempty"`

	// Trace controls guardrail tracing level
	// +kubebuilder:validation:Enum=disabled;enabled;enabled_full
	// +optional
	Trace string `json:"trace,omitempty"`
}

// BedrockPerformanceConfig defines performance optimization settings for Bedrock
type BedrockPerformanceConfig struct {
	// Latency controls performance/cost tradeoff
	// +kubebuilder:validation:Enum=optimized;standard
	// +optional
	Latency string `json:"latency,omitempty"`
}

// BedrockServiceTier defines the service tier for Bedrock requests
type BedrockServiceTier struct {
	// Type specifies the service tier type
	// +kubebuilder:validation:Enum=default;flex;priority;reserved
	// +optional
	Type string `json:"type,omitempty"`
}

// BedrockParameters contains AWS Bedrock-specific model parameters
type BedrockParameters struct {
	// GuardrailConfig defines content moderation and safety settings
	// +optional
	GuardrailConfig *BedrockGuardrailConfig `json:"guardrailConfig,omitempty"`

	// PerformanceConfiguration defines performance optimization settings
	// +optional
	PerformanceConfiguration *BedrockPerformanceConfig `json:"performanceConfiguration,omitempty"`

	// RequestMetadata is additional metadata to attach to Bedrock API requests
	// +optional
	RequestMetadata map[string]string `json:"requestMetadata,omitempty"`

	// AdditionalModelResponseFieldsPaths are JSON paths to extract additional fields from model responses
	// +optional
	AdditionalModelResponseFieldsPaths []string `json:"additionalModelResponseFieldsPaths,omitempty"`

	// PromptVariables are variables for substitution into prompt templates
	// +optional
	PromptVariables map[string]string `json:"promptVariables,omitempty"`

	// AdditionalModelRequestFields are additional model-specific parameters to include in requests
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	AdditionalModelRequestFields *runtime.RawExtension `json:"additionalModelRequestFields,omitempty"`

	// CacheToolDefinitions adds a cache point after the last tool definition
	// +optional
	CacheToolDefinitions *bool `json:"cacheToolDefinitions,omitempty"`

	// CacheInstructions adds a cache point after the system prompt blocks
	// +optional
	CacheInstructions *bool `json:"cacheInstructions,omitempty"`

	// CacheMessages adds a cache point to the last content block in the final user message
	// +optional
	CacheMessages *bool `json:"cacheMessages,omitempty"`

	// ServiceTier controls performance and cost optimization
	// +optional
	ServiceTier *BedrockServiceTier `json:"serviceTier,omitempty"`
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
// It defines a specific LLM model with its parameters, referencing a ModelProvider for connection configuration.
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
