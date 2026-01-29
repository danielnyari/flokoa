package controller

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// OpenAIProviderHandler handles OpenAI model configuration.
type OpenAIProviderHandler struct{}

func (h *OpenAIProviderHandler) BuildConfig(model *agentv1alpha1.Model, modelConfig *agentv1alpha1.ModelConfig) (*ModelProviderConfig, error) {
	config := buildBaseConfig(model, modelConfig)

	// Add API key as secret env var
	addAPIKeyEnvVar(config, model.Spec.APIKeySecretRef, "OPENAI_API_KEY")

	// Add OpenAI-specific configuration
	if model.Spec.OpenAI != nil {
		openaiSpec := model.Spec.OpenAI

		if openaiSpec.BaseURL != "" {
			config.Config["baseURL"] = openaiSpec.BaseURL
			// Also set as env var for SDK compatibility
			config.EnvVars = append(config.EnvVars, corev1.EnvVar{
				Name:  "OPENAI_BASE_URL",
				Value: openaiSpec.BaseURL,
			})
		}

		if openaiSpec.OrganizationID != "" {
			config.Config["organizationID"] = openaiSpec.OrganizationID
			config.EnvVars = append(config.EnvVars, corev1.EnvVar{
				Name:  "OPENAI_ORG_ID",
				Value: openaiSpec.OrganizationID,
			})
		}

		if openaiSpec.TimeoutSeconds != nil {
			config.Config["timeoutSeconds"] = *openaiSpec.TimeoutSeconds
		}
	}

	// Add OpenAI-specific parameters from ModelConfig
	if modelConfig != nil && modelConfig.Spec.OpenAI != nil {
		openaiParams := modelConfig.Spec.OpenAI
		params := make(map[string]any)

		if openaiParams.FrequencyPenalty != "" {
			params["frequencyPenalty"] = openaiParams.FrequencyPenalty
		}
		if openaiParams.PresencePenalty != "" {
			params["presencePenalty"] = openaiParams.PresencePenalty
		}
		if openaiParams.ReasoningEffort != nil {
			params["reasoningEffort"] = string(*openaiParams.ReasoningEffort)
		}
		if openaiParams.LogProbs != nil {
			params["logProbs"] = *openaiParams.LogProbs
		}
		if openaiParams.TopLogProbs != nil {
			params["topLogProbs"] = *openaiParams.TopLogProbs
		}
		if openaiParams.ResponseFormat != nil {
			params["responseFormat"] = map[string]any{
				"type": openaiParams.ResponseFormat.Type,
			}
			if openaiParams.ResponseFormat.JSONSchema != nil {
				params["responseFormat"].(map[string]any)["jsonSchema"] = map[string]any{
					"name":        openaiParams.ResponseFormat.JSONSchema.Name,
					"description": openaiParams.ResponseFormat.JSONSchema.Description,
					"strict":      openaiParams.ResponseFormat.JSONSchema.Strict,
				}
			}
		}

		if len(params) > 0 {
			if config.Parameters == nil {
				config.Parameters = &ModelParametersConfig{}
			}
			config.Parameters.OpenAI = params
		}
	}

	return config, nil
}
