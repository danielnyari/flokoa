package controller

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// AnthropicProviderHandler handles Anthropic model configuration.
type AnthropicProviderHandler struct{}

func (h *AnthropicProviderHandler) BuildConfig(provider *agentv1alpha1.ModelProvider, model *agentv1alpha1.Model) (*ResolvedModelConfig, error) {
	config := buildBaseConfig(provider, model)

	// Add API key as secret env var
	addAPIKeyEnvVar(config, provider.Spec.APIKeySecretRef, "ANTHROPIC_API_KEY")

	// Add Anthropic-specific provider configuration
	if provider.Spec.Anthropic != nil {
		anthropicSpec := provider.Spec.Anthropic

		if anthropicSpec.BaseURL != "" {
			config.Config["baseURL"] = anthropicSpec.BaseURL
			config.EnvVars = append(config.EnvVars, corev1.EnvVar{
				Name:  "ANTHROPIC_BASE_URL",
				Value: anthropicSpec.BaseURL,
			})
		}

		if anthropicSpec.TimeoutSeconds != nil {
			config.Config["timeoutSeconds"] = *anthropicSpec.TimeoutSeconds
		}
	}

	// Add Anthropic-specific parameters from Model
	if model.Spec.Parameters != nil && model.Spec.Parameters.Anthropic != nil {
		anthropicParams := model.Spec.Parameters.Anthropic
		params := make(map[string]any)

		if anthropicParams.MetadataUserID != "" {
			params["metadataUserID"] = anthropicParams.MetadataUserID
		}

		if anthropicParams.Thinking != nil {
			thinking := map[string]any{
				"type": string(anthropicParams.Thinking.Type),
			}
			if anthropicParams.Thinking.BudgetTokens != nil {
				thinking["budgetTokens"] = *anthropicParams.Thinking.BudgetTokens
			}
			params["thinking"] = thinking
		}

		if anthropicParams.CacheToolDefinitions != "" {
			params["cacheToolDefinitions"] = anthropicParams.CacheToolDefinitions
		}

		if anthropicParams.CacheInstructions != "" {
			params["cacheInstructions"] = anthropicParams.CacheInstructions
		}

		if anthropicParams.CacheMessages != "" {
			params["cacheMessages"] = anthropicParams.CacheMessages
		}

		if anthropicParams.Container != nil {
			container := make(map[string]any)
			if anthropicParams.Container.ID != "" {
				container["id"] = anthropicParams.Container.ID
			}
			if anthropicParams.Container.Disabled != nil {
				container["disabled"] = *anthropicParams.Container.Disabled
			}
			params["container"] = container
		}

		if len(params) > 0 {
			if config.Parameters == nil {
				config.Parameters = &ModelParametersConfig{}
			}
			config.Parameters.Anthropic = params
		}
	}

	return config, nil
}
