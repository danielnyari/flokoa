package model

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// AnthropicProviderHandler handles Anthropic model configuration.
type AnthropicProviderHandler struct{}

func (h *AnthropicProviderHandler) BuildConfig(provider *agentv1alpha1.ModelProvider, model *agentv1alpha1.Model) (*ResolvedModelConfig, error) {
	config := BuildBaseConfig(provider, model)

	// Add API key as secret env var
	AddAPIKeyEnvVar(config, provider.Spec.APIKeySecretRef, "ANTHROPIC_API_KEY")

	// Add Anthropic-specific environment variables for SDK compatibility
	if provider.Spec.Anthropic != nil {
		anthropicSpec := provider.Spec.Anthropic

		if anthropicSpec.BaseURL != "" {
			config.EnvVars = append(config.EnvVars, corev1.EnvVar{
				Name:  "ANTHROPIC_BASE_URL",
				Value: anthropicSpec.BaseURL,
			})
		}
	}

	return config, nil
}
