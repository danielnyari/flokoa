package controller

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// OpenAIProviderHandler handles OpenAI model configuration.
type OpenAIProviderHandler struct{}

func (h *OpenAIProviderHandler) BuildConfig(provider *agentv1alpha1.ModelProvider, model *agentv1alpha1.Model) (*ResolvedModelConfig, error) {
	config := buildBaseConfig(provider, model)

	// Add API key as secret env var
	addAPIKeyEnvVar(config, provider.Spec.APIKeySecretRef, "OPENAI_API_KEY")

	// Add OpenAI-specific environment variables for SDK compatibility
	if provider.Spec.OpenAI != nil {
		openaiSpec := provider.Spec.OpenAI

		if openaiSpec.BaseURL != "" {
			config.EnvVars = append(config.EnvVars, corev1.EnvVar{
				Name:  "OPENAI_BASE_URL",
				Value: openaiSpec.BaseURL,
			})
		}

		if openaiSpec.OrganizationID != "" {
			config.EnvVars = append(config.EnvVars, corev1.EnvVar{
				Name:  "OPENAI_ORG_ID",
				Value: openaiSpec.OrganizationID,
			})
		}
	}

	return config, nil
}
