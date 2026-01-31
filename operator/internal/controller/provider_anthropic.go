package controller

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// AnthropicProviderHandler handles Anthropic model configuration.
type AnthropicProviderHandler struct{}

func (h *AnthropicProviderHandler) BuildConfig(model *agentv1alpha1.Model, modelConfig *agentv1alpha1.ModelConfig) (*ModelProviderConfig, error) {
	config := buildBaseConfig(model, modelConfig)

	// Add API key as secret env var
	addAPIKeyEnvVar(config, model.Spec.APIKeySecretRef, "ANTHROPIC_API_KEY")

	// Add Anthropic-specific configuration
	if model.Spec.Anthropic != nil {
		anthropicSpec := model.Spec.Anthropic

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

	// Add Anthropic-specific parameters from ModelConfig
	if modelConfig != nil && modelConfig.Spec.Anthropic != nil {
		anthropicParams := modelConfig.Spec.Anthropic
		params := make(map[string]any)

		if anthropicParams.AnthropicMetadataUserID != "" {
			params["metadataUserID"] = anthropicParams.AnthropicMetadataUserID
		}

		if anthropicParams.AnthropicThinking != nil {
			thinking := map[string]any{
				"type": string(anthropicParams.AnthropicThinking.Type),
			}
			if anthropicParams.AnthropicThinking.BudgetTokens != nil {
				thinking["budgetTokens"] = *anthropicParams.AnthropicThinking.BudgetTokens
			}
			params["thinking"] = thinking
		}

		if anthropicParams.AnthropicCacheToolDefinitions != "" {
			params["cacheToolDefinitions"] = anthropicParams.AnthropicCacheToolDefinitions
		}

		if anthropicParams.AnthropicCacheInstructions != "" {
			params["cacheInstructions"] = anthropicParams.AnthropicCacheInstructions
		}

		if anthropicParams.AnthropicCacheMessages != "" {
			params["cacheMessages"] = anthropicParams.AnthropicCacheMessages
		}

		if anthropicParams.AnthropicContainer != nil {
			container := make(map[string]any)
			if anthropicParams.AnthropicContainer.ID != "" {
				container["id"] = anthropicParams.AnthropicContainer.ID
			}
			if anthropicParams.AnthropicContainer.Disabled != nil {
				container["disabled"] = *anthropicParams.AnthropicContainer.Disabled
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
