package model

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// AnthropicProviderHandler handles Anthropic provider configuration.
type AnthropicProviderHandler struct{}

func (h *AnthropicProviderHandler) Resolve(provider *agentv1alpha1.ModelProvider) (*ResolvedProvider, error) {
	resolved := &ResolvedProvider{
		Type:        agentv1alpha1.ProviderTypeAnthropic,
		ModelPrefix: "anthropic",
	}

	AddAPIKeyEnvVar(resolved, provider.Spec.APIKeySecretRef, "ANTHROPIC_API_KEY")

	if provider.Spec.Anthropic != nil && provider.Spec.Anthropic.BaseURL != "" {
		resolved.EnvVars = append(resolved.EnvVars, corev1.EnvVar{
			Name:  "ANTHROPIC_BASE_URL",
			Value: provider.Spec.Anthropic.BaseURL,
		})
	}

	return resolved, nil
}
