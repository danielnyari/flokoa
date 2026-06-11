package model

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// BedrockProviderHandler handles AWS Bedrock provider configuration.
type BedrockProviderHandler struct{}

func (h *BedrockProviderHandler) Resolve(provider *agentv1alpha1.ModelProvider) (*ResolvedProvider, error) {
	resolved := &ResolvedProvider{
		Type:        agentv1alpha1.ProviderTypeBedrock,
		ModelPrefix: "bedrock",
	}

	if provider.Spec.Bedrock != nil && provider.Spec.Bedrock.Region != "" {
		resolved.EnvVars = append(resolved.EnvVars, corev1.EnvVar{
			Name:  "AWS_REGION",
			Value: provider.Spec.Bedrock.Region,
		})
	}

	// AWS credentials are expected from the pod environment (IRSA, instance
	// profiles, or explicitly injected AWS_* env) — never from flokoa config.

	return resolved, nil
}
