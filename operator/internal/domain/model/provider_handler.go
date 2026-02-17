package model

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

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

// BuildBaseConfig creates the base ResolvedModelConfig with common fields.
func BuildBaseConfig(provider *agentv1alpha1.ModelProvider, model *agentv1alpha1.Model) *ResolvedModelConfig {
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

// AddAPIKeyEnvVar adds the API key secret reference as an environment variable.
func AddAPIKeyEnvVar(config *ResolvedModelConfig, secretRef *corev1.SecretKeySelector, envVarName string) {
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
