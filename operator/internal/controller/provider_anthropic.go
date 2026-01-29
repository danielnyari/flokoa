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

		if anthropicParams.Thinking != nil {
			thinking := map[string]any{
				"type": string(anthropicParams.Thinking.Type),
			}
			if anthropicParams.Thinking.BudgetTokens != nil {
				thinking["budgetTokens"] = *anthropicParams.Thinking.BudgetTokens
			}
			params["thinking"] = thinking
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

// AnthropicVertexProviderHandler handles Anthropic on Vertex AI model configuration.
type AnthropicVertexProviderHandler struct{}

func (h *AnthropicVertexProviderHandler) BuildConfig(model *agentv1alpha1.Model, modelConfig *agentv1alpha1.ModelConfig) (*ModelProviderConfig, error) {
	config := buildBaseConfig(model, modelConfig)

	// Vertex AI configuration
	if model.Spec.VertexAI != nil {
		vertexSpec := model.Spec.VertexAI

		config.Config["project"] = vertexSpec.Project
		config.Config["location"] = vertexSpec.Location

		config.EnvVars = append(config.EnvVars,
			corev1.EnvVar{
				Name:  "GOOGLE_CLOUD_PROJECT",
				Value: vertexSpec.Project,
			},
			corev1.EnvVar{
				Name:  "GOOGLE_CLOUD_REGION",
				Value: vertexSpec.Location,
			},
		)

		// Add service account key as secret env var
		if vertexSpec.ServiceAccountKeySecretRef != nil {
			config.SecretEnvVars = append(config.SecretEnvVars, corev1.EnvVar{
				Name: "GOOGLE_APPLICATION_CREDENTIALS_JSON",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: vertexSpec.ServiceAccountKeySecretRef,
				},
			})
		}
	}

	// Add Anthropic-specific parameters from ModelConfig (same as regular Anthropic)
	if modelConfig != nil && modelConfig.Spec.Anthropic != nil {
		anthropicParams := modelConfig.Spec.Anthropic
		params := make(map[string]any)

		if anthropicParams.Thinking != nil {
			thinking := map[string]any{
				"type": string(anthropicParams.Thinking.Type),
			}
			if anthropicParams.Thinking.BudgetTokens != nil {
				thinking["budgetTokens"] = *anthropicParams.Thinking.BudgetTokens
			}
			params["thinking"] = thinking
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
