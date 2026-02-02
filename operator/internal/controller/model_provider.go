package controller

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// ResolvedModelConfig contains the resolved configuration for a model.
// This is serialized to JSON and stored in a ConfigMap for the agent to consume.
type ResolvedModelConfig struct {
	// Provider contains provider type and connection configuration
	Provider ProviderConfig `json:"provider"`

	// Model is the model identifier (e.g., "gpt-4o", "claude-sonnet-4-20250514")
	Model string `json:"model"`

	// Parameters contains all model parameters
	Parameters *agentv1alpha1.ModelParameters `json:"parameters,omitempty"`

	// Internal fields (not serialized to JSON)

	// ConfigMapName is the name of the ConfigMap containing the model config
	ConfigMapName string `json:"-"`

	// EnvVars are non-secret environment variables to set on the container
	EnvVars []corev1.EnvVar `json:"-"`

	// SecretEnvVars are environment variables sourced from secrets
	SecretEnvVars []corev1.EnvVar `json:"-"`
}

// ProviderConfig contains the provider type and connection configuration.
// Only one of OpenAI, Anthropic, Google, or Bedrock will be set based on the Type.
type ProviderConfig struct {
	Type agentv1alpha1.ProviderType `json:"type"`

	// OpenAI provider configuration (only set for OpenAI providers)
	OpenAI *agentv1alpha1.OpenAIProviderSpec `json:"openai,omitempty"`

	// Anthropic provider configuration (only set for Anthropic providers)
	Anthropic *agentv1alpha1.AnthropicProviderSpec `json:"anthropic,omitempty"`

	// Google provider configuration (only set for Google providers)
	Google *agentv1alpha1.GoogleProviderSpec `json:"google,omitempty"`

	// Bedrock provider configuration (only set for Bedrock providers)
	Bedrock *agentv1alpha1.BedrockProviderSpec `json:"bedrock,omitempty"`

	// DefaultHeaders from ModelProvider
	DefaultHeaders map[string]string `json:"defaultHeaders,omitempty"`
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
	providerType := provider.GetProviderType()

	config := &ResolvedModelConfig{
		Provider: ProviderConfig{
			Type:           providerType,
			DefaultHeaders: provider.Spec.DefaultHeaders,
		},
		Model:      model.Spec.Model,
		Parameters: model.Spec.Parameters,
	}

	// Set the appropriate provider spec based on type
	switch providerType {
	case agentv1alpha1.ProviderTypeOpenAI:
		config.Provider.OpenAI = provider.Spec.OpenAI
	case agentv1alpha1.ProviderTypeAnthropic:
		config.Provider.Anthropic = provider.Spec.Anthropic
	case agentv1alpha1.ProviderTypeGoogle:
		config.Provider.Google = provider.Spec.Google
	case agentv1alpha1.ProviderTypeBedrock:
		config.Provider.Bedrock = provider.Spec.Bedrock
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
