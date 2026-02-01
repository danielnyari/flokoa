package controller

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// ModelProviderConfig contains the resolved configuration for a model provider.
// This is used to configure the agent deployment with the appropriate
// environment variables, secrets, and non-sensitive config.
type ModelProviderConfig struct {
	// Provider is the model provider type
	Provider agentv1alpha1.ModelProvider `json:"provider"`

	// Model is the model identifier (e.g., "gpt-4o", "claude-sonnet-4-20250514")
	Model string `json:"model"`

	// ConfigMapName is the name of the ConfigMap containing non-sensitive model config
	ConfigMapName string `json:"-"`

	// EnvVars are non-secret environment variables to set on the container
	EnvVars []corev1.EnvVar `json:"-"`

	// SecretEnvVars are environment variables sourced from secrets
	SecretEnvVars []corev1.EnvVar `json:"-"`

	// Config contains provider-specific non-sensitive configuration (serialized to JSON)
	Config map[string]any `json:"config,omitempty"`

	// Parameters contains model parameters from ModelConfig (if used)
	Parameters *ModelParametersConfig `json:"parameters,omitempty"`
}

// ModelParametersConfig contains the model parameters (from ModelConfig).
type ModelParametersConfig struct {
	Temperature      string   `json:"temperature,omitempty"`
	MaxTokens        *int32   `json:"maxTokens,omitempty"`
	TopP             string   `json:"topP,omitempty"`
	TopK             *int32   `json:"topK,omitempty"`
	PresencePenalty  string   `json:"presencePenalty,omitempty"`
	FrequencyPenalty string   `json:"frequencyPenalty,omitempty"`
	StopSequences    []string `json:"stopSequences,omitempty"`
	Seed             *int64   `json:"seed,omitempty"`

	// Provider-specific parameters
	OpenAI    map[string]any `json:"openai,omitempty"`
	Anthropic map[string]any `json:"anthropic,omitempty"`
	Google    map[string]any `json:"google,omitempty"`
	Bedrock   map[string]any `json:"bedrock,omitempty"`
}

// ModelProviderHandler defines the interface for provider-specific model configuration.
// Each provider implements this interface to handle its specific requirements.
type ModelProviderHandler interface {
	// BuildConfig builds the provider-specific configuration.
	// Returns the non-sensitive config map and any environment variables needed.
	BuildConfig(model *agentv1alpha1.Model, modelConfig *agentv1alpha1.ModelConfig) (*ModelProviderConfig, error)
}

// modelProviderRegistry maps providers to their handlers
var modelProviderRegistry = map[agentv1alpha1.ModelProvider]ModelProviderHandler{
	agentv1alpha1.ModelProviderOpenAI:    &OpenAIProviderHandler{},
	agentv1alpha1.ModelProviderAnthropic: &AnthropicProviderHandler{},
	agentv1alpha1.ModelProviderGoogle:    &GoogleProviderHandler{},
	agentv1alpha1.ModelProviderBedrock:   &BedrockProviderHandler{},
}

// GetProviderHandler returns the handler for the given provider.
func GetProviderHandler(provider agentv1alpha1.ModelProvider) (ModelProviderHandler, bool) {
	handler, ok := modelProviderRegistry[provider]
	return handler, ok
}

// buildBaseConfig creates the base ModelProviderConfig with common fields.
func buildBaseConfig(model *agentv1alpha1.Model, modelConfig *agentv1alpha1.ModelConfig) *ModelProviderConfig {
	config := &ModelProviderConfig{
		Provider: model.Spec.Provider,
		Model:    model.Spec.Model,
		Config:   make(map[string]any),
	}

	// Extract base model parameters from the provider-specific structs (which embed ModelParameters)
	if modelConfig != nil {
		var baseParams *agentv1alpha1.ModelParameters

		// Each provider-specific struct embeds ModelParameters, extract it based on which one is set
		switch {
		case modelConfig.Spec.OpenAI != nil:
			baseParams = &modelConfig.Spec.OpenAI.ModelParameters
		case modelConfig.Spec.Anthropic != nil:
			baseParams = &modelConfig.Spec.Anthropic.ModelParameters
		case modelConfig.Spec.Google != nil:
			baseParams = &modelConfig.Spec.Google.ModelParameters
		case modelConfig.Spec.Bedrock != nil:
			baseParams = &modelConfig.Spec.Bedrock.ModelParameters
		}

		if baseParams != nil {
			config.Parameters = &ModelParametersConfig{
				Temperature:      baseParams.Temperature,
				MaxTokens:        baseParams.MaxTokens,
				TopP:             baseParams.TopP,
				TopK:             baseParams.TopK,
				PresencePenalty:  baseParams.PresencePenalty,
				FrequencyPenalty: baseParams.FrequencyPenalty,
				StopSequences:    baseParams.StopSequences,
				Seed:             baseParams.Seed,
			}
		}
	}

	// Add default headers if present
	if len(model.Spec.DefaultHeaders) > 0 {
		config.Config["defaultHeaders"] = model.Spec.DefaultHeaders
	}

	return config
}

// addAPIKeyEnvVar adds the API key secret reference as an environment variable.
func addAPIKeyEnvVar(config *ModelProviderConfig, secretRef *corev1.SecretKeySelector, envVarName string) {
	if secretRef == nil {
		return
	}

	config.SecretEnvVars = append(config.SecretEnvVars, corev1.EnvVar{
		Name: envVarName,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: secretRef,
		},
	})
}
