package model

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// OpenAIProviderHandler handles OpenAI provider configuration.
type OpenAIProviderHandler struct{}

func (h *OpenAIProviderHandler) Resolve(provider *agentv1alpha1.ModelProvider) (*ResolvedProvider, error) {
	resolved := &ResolvedProvider{
		Type:        agentv1alpha1.ProviderTypeOpenAI,
		ModelPrefix: "openai",
	}

	AddAPIKeyEnvVar(resolved, provider.Spec.APIKeySecretRef, "OPENAI_API_KEY")

	if provider.Spec.OpenAI != nil && provider.Spec.OpenAI.BaseURL != "" {
		resolved.EnvVars = append(resolved.EnvVars, corev1.EnvVar{
			Name:  "OPENAI_BASE_URL",
			Value: provider.Spec.OpenAI.BaseURL,
		})
	}

	return resolved, nil
}
