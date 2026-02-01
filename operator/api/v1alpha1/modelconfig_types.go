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

	// OpenAI-specific parameters (includes base ModelParameters via embedding)
	// +optional
	OpenAI *OpenAIResponsesModelParameters `json:"openai,omitempty"`

	// Anthropic-specific parameters (includes base ModelParameters via embedding)
	// +optional
	Anthropic *AnthropicModelParameters `json:"anthropic,omitempty"`

	// Google/Gemini-specific parameters (includes base ModelParameters via embedding)
	// +optional
	Google *GoogleModelParameters `json:"google,omitempty"`

	// Bedrock-specific parameters (includes base ModelParameters via embedding)
	// +optional
	Bedrock *BedrockModelParameters `json:"bedrock,omitempty"`
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

	// TimeOut in seconds for the model response
	// +kubebuilder:validation:Minimum=1
	// +optional
	TimeOut *int32 `json:"timeOut,omitempty"`

	// ParallelToolCalls
	// +optional
	ParallelToolCalls *bool `json:"parallelToolCalls,omitempty"`

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

	// Logit Bias
	// +optional
	LogitBias map[string]int32 `json:"logitBias,omitempty"`

	// StopSequences are sequences where the model will stop generating
	// +optional
	StopSequences []string `json:"stopSequences,omitempty"`

	// ExtraHeaders to include in all requests
	// +optional
	ExtraHeaders map[string]string `json:"extraHeaders,omitempty"`

	// ExtraBody to include in all requests
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	ExtraBody *runtime.RawExtension `json:"extraBody,omitempty"`

	// Seed for deterministic generation (where supported)
	// +optional
	Seed *int64 `json:"seed,omitempty"`
}

// OpenAIChatModelParameters extends ModelParameters for OpenAI Chat Completions API
type OpenAIChatModelParameters struct {
	// Embed base parameters
	ModelParameters `json:",inline"`

	// ReasoningEffort controls the reasoning effort for reasoning models (o1, o3, etc.)
	// +optional
	OpenAIReasoningEffort *ReasoningEffort `json:"reasoningEffort,omitempty"`

	// LogProbs
	// +optional
	OpenAILogProbs *bool `json:"logProbs,omitempty"`

	// TopLogProbs
	// +optional
	OpenAITopLogProbs *int32 `json:"topLogProbs,omitempty"`

	// OpenAIUser associated with the requests
	// +optional
	OpenAIUser string `json:"openAIUser,omitempty"`

	// OpenAIServiceTier
	// +kubebuilder:validation:Enum=auto;default;flex;priority
	// +optional
	OpenAIServiceTier string `json:"openAIServiceTier,omitempty"`

	// +optional
	OpenAIPromptCacheKey string `json:"openAIPromptCacheKey,omitempty"`

	// OpenAIPromptRetention controls prompt caching retention
	// +optional
	OpenAIPromptRetention string `json:"openAIPromptRetention,omitempty"`

	// ResponseFormat specifies the output format
	// +optional
	ResponseFormat *OpenAIResponseFormat `json:"responseFormat,omitempty"`
}

// OpenAIResponsesModelParameters extends OpenAIChatModelParameters for OpenAI Responses API
type OpenAIResponsesModelParameters struct {
	// Embed chat parameters (which includes base)
	OpenAIChatModelParameters `json:",inline"`

	// OpenAIReasoningGenerateSummary
	// +kubebuilder:validation:Enum=detailed;concise
	// +optional
	OpenAIReasoningGenerateSummary *string `json:"openAIReasoningGenerateSummary,omitempty"`

	// OpenAIReasoningSummary
	// +kubebuilder:validation:Enum=detailed;concise;auto
	// +optional
	OpenAIReasoningSummary *string `json:"openAIReasoningSummary,omitempty"`

	// OpenAISendReasoningIDs
	// +optional
	OpenAISendReasoningIDs *bool `json:"openAISendReasoningIDs,omitempty"`

	// OpenAITruncation
	// +kubebuilder:validation:Enum=disabled;auto
	// +optional
	OpenAITruncation *string `json:"openAITruncation,omitempty"`

	// OpenAITextVerbosity
	// +kubebuilder:validation:Enum=low;medium;high
	// +optional
	OpenAITextVerbosity *string `json:"openAITextVerbosity,omitempty"`

	// OpenAIPreviousResponseID is the ID of a previous response to continue from.
	// When set to "auto", automatically uses the most recent provider_response_id
	// from message history and omits earlier messages. Enables server-side
	// conversation state and faithful reference to previous reasoning.
	// +optional
	OpenAIPreviousResponseID string `json:"openAIPreviousResponseID,omitempty"`

	// OpenAIIncludeCodeExecutionOutputs includes code execution results in the response.
	// Corresponds to the code_interpreter_call.outputs value of the include parameter.
	// +optional
	OpenAIIncludeCodeExecutionOutputs *bool `json:"openAIIncludeCodeExecutionOutputs,omitempty"`

	// OpenAIIncludeWebSearchSources includes web search results in the response.
	// Corresponds to the web_search_call.action.sources value of the include parameter.
	// +optional
	OpenAIIncludeWebSearchSources *bool `json:"openAIIncludeWebSearchSources,omitempty"`

	// OpenAIIncludeFileSearchResults includes file search results in the response.
	// Corresponds to the file_search_call.results value of the include parameter.
	// +optional
	OpenAIIncludeFileSearchResults *bool `json:"openAIIncludeFileSearchResults,omitempty"`

	// OpenAIIncludeRawAnnotations includes raw annotations (e.g., citations from web search)
	// in TextPart.provider_details['annotations'].
	// +optional
	OpenAIIncludeRawAnnotations *bool `json:"openAIIncludeRawAnnotations,omitempty"`
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
	// IncludeThoughts indicates whether to include thoughts in the response.
	// If true, thoughts are returned only if the model supports thought and thoughts are available.
	// +optional
	IncludeThoughts *bool `json:"includeThoughts,omitempty"`

	// ThinkingBudget is the thinking budget in tokens.
	// 0 means DISABLED, -1 means AUTOMATIC.
	// The default values and allowed ranges are model dependent.
	// +optional
	ThinkingBudget *int32 `json:"thinkingBudget,omitempty"`

	// ThinkingLevel controls the amount of thinking tokens the model should generate.
	// +optional
	ThinkingLevel *GoogleThinkingLevel `json:"thinkingLevel,omitempty"`
}

// GoogleModelParameters extends ModelParameters for Google/Gemini
type GoogleModelParameters struct {
	// Embed base parameters
	ModelParameters `json:",inline"`

	// GoogleSafetySettings configures content safety filters.
	// See https://ai.google.dev/gemini-api/docs/safety-settings for more information.
	// +optional
	GoogleSafetySettings []GoogleSafetySetting `json:"googleSafetySettings,omitempty"`

	// GoogleThinkingConfig configures the thinking behavior for the model.
	// See https://ai.google.dev/gemini-api/docs/thinking for more information.
	// +optional
	GoogleThinkingConfig *GoogleThinkingConfig `json:"googleThinkingConfig,omitempty"`

	// GoogleLabels are user-defined metadata to break down billed charges.
	// Only supported by the Vertex AI API.
	// See https://cloud.google.com/vertex-ai/generative-ai/docs/multimodal/add-labels-to-api-calls
	// +optional
	GoogleLabels map[string]string `json:"googleLabels,omitempty"`

	// GoogleVideoResolution specifies the video resolution to use for the model.
	// See https://ai.google.dev/api/generate-content#MediaResolution for more information.
	// +optional
	GoogleVideoResolution *GoogleMediaResolution `json:"googleVideoResolution,omitempty"`

	// GoogleCachedContent is the name of the cached content to use for the model.
	// See https://ai.google.dev/gemini-api/docs/caching for more information.
	// +optional
	GoogleCachedContent string `json:"googleCachedContent,omitempty"`
}

// GoogleSafetySetting defines a safety setting for Google/Gemini models
type GoogleSafetySetting struct {
	// Category is the harm category
	// +kubebuilder:validation:Enum=HARM_CATEGORY_HARASSMENT;HARM_CATEGORY_HATE_SPEECH;HARM_CATEGORY_SEXUALLY_EXPLICIT;HARM_CATEGORY_DANGEROUS_CONTENT;HARM_CATEGORY_CIVIC_INTEGRITY
	Category string `json:"category"`

	// Threshold is the blocking threshold
	// +kubebuilder:validation:Enum=BLOCK_NONE;BLOCK_LOW_AND_ABOVE;BLOCK_MEDIUM_AND_ABOVE;BLOCK_ONLY_HIGH;OFF
	Threshold string `json:"threshold"`

	// Method specifies if the threshold is used for probability or severity score.
	// If not specified, the threshold is used for probability score.
	// +kubebuilder:validation:Enum=SEVERITY;PROBABILITY
	// +optional
	Method string `json:"method,omitempty"`
}

// BedrockGuardrailConfig defines content moderation and safety settings for Bedrock
// See https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_GuardrailConfiguration.html
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
// See https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_PerformanceConfiguration.html
type BedrockPerformanceConfig struct {
	// Latency controls performance/cost tradeoff
	// +kubebuilder:validation:Enum=optimized;standard
	// +optional
	Latency string `json:"latency,omitempty"`
}

// BedrockServiceTier defines the service tier for Bedrock requests
// See https://docs.aws.amazon.com/bedrock/latest/userguide/service-tiers-inference.html
type BedrockServiceTier struct {
	// Type specifies the service tier type
	// +kubebuilder:validation:Enum=default;flex;priority;reserved
	// +optional
	Type string `json:"type,omitempty"`
}

// BedrockModelParameters extends ModelParameters for AWS Bedrock
// See https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_Converse.html
type BedrockModelParameters struct {
	// Embed base parameters
	ModelParameters `json:",inline"`

	// BedrockGuardrailConfig defines content moderation and safety settings.
	// See https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_GuardrailConfiguration.html
	// +optional
	BedrockGuardrailConfig *BedrockGuardrailConfig `json:"bedrockGuardrailConfig,omitempty"`

	// BedrockPerformanceConfiguration defines performance optimization settings.
	// See https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_PerformanceConfiguration.html
	// +optional
	BedrockPerformanceConfiguration *BedrockPerformanceConfig `json:"bedrockPerformanceConfiguration,omitempty"`

	// BedrockRequestMetadata is additional metadata to attach to Bedrock API requests.
	// +optional
	BedrockRequestMetadata map[string]string `json:"bedrockRequestMetadata,omitempty"`

	// BedrockAdditionalModelResponseFieldsPaths are JSON paths to extract additional fields from model responses.
	// See https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters.html
	// +optional
	BedrockAdditionalModelResponseFieldsPaths []string `json:"bedrockAdditionalModelResponseFieldsPaths,omitempty"`

	// BedrockPromptVariables are variables for substitution into prompt templates.
	// Each key maps to a text value.
	// See https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_PromptVariableValues.html
	// +optional
	BedrockPromptVariables map[string]string `json:"bedrockPromptVariables,omitempty"`

	// BedrockAdditionalModelRequestFields are additional model-specific parameters to include in requests.
	// See https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters.html
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	BedrockAdditionalModelRequestFields *runtime.RawExtension `json:"bedrockAdditionalModelRequestFields,omitempty"`

	// BedrockCacheToolDefinitions adds a cache point after the last tool definition.
	// See https://docs.aws.amazon.com/bedrock/latest/userguide/prompt-caching.html
	// +optional
	BedrockCacheToolDefinitions *bool `json:"bedrockCacheToolDefinitions,omitempty"`

	// BedrockCacheInstructions adds a cache point after the system prompt blocks.
	// See https://docs.aws.amazon.com/bedrock/latest/userguide/prompt-caching.html
	// +optional
	BedrockCacheInstructions *bool `json:"bedrockCacheInstructions,omitempty"`

	// BedrockCacheMessages adds a cache point to the last content block in the final user message.
	// Useful for caching conversation history in multi-turn conversations.
	// Note: Uses 1 of Bedrock's 4 available cache points per request.
	// See https://docs.aws.amazon.com/bedrock/latest/userguide/prompt-caching.html
	// +optional
	BedrockCacheMessages *bool `json:"bedrockCacheMessages,omitempty"`

	// BedrockServiceTier controls performance and cost optimization.
	// See https://docs.aws.amazon.com/bedrock/latest/userguide/service-tiers-inference.html
	// +optional
	BedrockServiceTier *BedrockServiceTier `json:"bedrockServiceTier,omitempty"`
}

// AnthropicThinkingConfig defines the thinking configuration for Anthropic models
type AnthropicThinkingConfig struct {
	// Type enables or disables extended thinking
	// +kubebuilder:validation:Enum=enabled;disabled
	// +kubebuilder:default=disabled
	Type ThinkingType `json:"type"`

	// BudgetTokens is the maximum number of tokens for thinking (required when enabled)
	// Must be >= 1024 and less than maxTokens
	// See https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking
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

// AnthropicModelParameters extends ModelParameters for Anthropic
// See https://docs.anthropic.com/en/api/messages
type AnthropicModelParameters struct {
	// Embed base parameters
	ModelParameters `json:",inline"`

	// AnthropicMetadataUserID is an external identifier for the user associated with the request.
	// +optional
	AnthropicMetadataUserID string `json:"anthropicMetadataUserID,omitempty"`

	// AnthropicThinking determines whether the model should generate a thinking block.
	// See https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking
	// +optional
	AnthropicThinking *AnthropicThinkingConfig `json:"anthropicThinking,omitempty"`

	// AnthropicCacheToolDefinitions adds cache_control to the last tool definition.
	// When true, uses TTL='5m'. Can also specify '5m' or '1h' directly.
	// See https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching
	// +optional
	AnthropicCacheToolDefinitions string `json:"anthropicCacheToolDefinitions,omitempty"`

	// AnthropicCacheInstructions adds cache_control to the last system prompt block.
	// When true, uses TTL='5m'. Can also specify '5m' or '1h' directly.
	// See https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching
	// +optional
	AnthropicCacheInstructions string `json:"anthropicCacheInstructions,omitempty"`

	// AnthropicCacheMessages adds a cache point to the last content block in the final user message.
	// Useful for caching conversation history in multi-turn conversations.
	// When true, uses TTL='5m'. Can also specify '5m' or '1h' directly.
	// Note: Uses 1 of Anthropic's 4 available cache points per request.
	// See https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching
	// +optional
	AnthropicCacheMessages string `json:"anthropicCacheMessages,omitempty"`

	// AnthropicContainer configures container for multi-turn conversations.
	// By default, if previous messages contain a container_id, it will be reused automatically.
	// +optional
	AnthropicContainer *AnthropicContainerConfig `json:"anthropicContainer,omitempty"`
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
