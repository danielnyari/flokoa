package controller

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// ResolvedModelConfig contains the resolved configuration for a model.
// This is used to configure the agent deployment with the appropriate
// environment variables, secrets, and non-sensitive config.
type ResolvedModelConfig struct {
	// Provider is the model provider type
	Provider agentv1alpha1.ProviderType `json:"provider"`

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

	// Parameters contains model parameters from Model
	Parameters *ModelParametersConfig `json:"parameters,omitempty"`
}

// ModelParametersConfig contains the model parameters (from Model).
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

// ProviderHandler defines the interface for provider-specific model configuration.
// Each provider implements this interface to handle its specific requirements.
type ProviderHandler interface {
	// BuildConfig builds the provider-specific configuration.
	// Takes the ModelProvider (connection config) and Model (model + parameters).
	// Returns the resolved config map and any environment variables needed.
	BuildConfig(provider *agentv1alpha1.ModelProvider, model *agentv1alpha1.Model) (*ResolvedModelConfig, error)
}

// providerRegistry maps providers to their handlers
var providerRegistry = map[agentv1alpha1.ProviderType]ProviderHandler{
	agentv1alpha1.ProviderTypeOpenAI:    &OpenAIProviderHandler{},
	agentv1alpha1.ProviderTypeAnthropic: &AnthropicProviderHandler{},
	agentv1alpha1.ProviderTypeGoogle:    &GoogleProviderHandler{},
	agentv1alpha1.ProviderTypeBedrock:   &BedrockProviderHandler{},
}

// GetProviderHandler returns the handler for the given provider type.
func GetProviderHandler(providerType agentv1alpha1.ProviderType) (ProviderHandler, bool) {
	handler, ok := providerRegistry[providerType]
	return handler, ok
}

// buildBaseConfig creates the base ResolvedModelConfig with common fields.
func buildBaseConfig(provider *agentv1alpha1.ModelProvider, model *agentv1alpha1.Model) *ResolvedModelConfig {
	config := &ResolvedModelConfig{
		Provider: provider.GetProviderType(),
		Model:    model.Spec.Model,
		Config:   make(map[string]any),
	}

	// Extract base model parameters from Model.Spec.Parameters
	if model.Spec.Parameters != nil {
		params := model.Spec.Parameters
		config.Parameters = &ModelParametersConfig{
			Temperature:      params.Temperature,
			MaxTokens:        params.MaxTokens,
			TopP:             params.TopP,
			TopK:             params.TopK,
			PresencePenalty:  params.PresencePenalty,
			FrequencyPenalty: params.FrequencyPenalty,
			StopSequences:    params.StopSequences,
			Seed:             params.Seed,
		}
	}

	// Add default headers if present in ModelProvider
	if len(provider.Spec.DefaultHeaders) > 0 {
		config.Config["defaultHeaders"] = provider.Spec.DefaultHeaders
	}

	return config
}

// addAPIKeyEnvVar adds the API key secret reference as an environment variable.
func addAPIKeyEnvVar(config *ResolvedModelConfig, secretRef *corev1.SecretKeySelector, envVarName string) {
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
