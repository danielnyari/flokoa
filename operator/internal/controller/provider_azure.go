package controller

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// AzureOpenAIProviderHandler handles Azure OpenAI model configuration.
type AzureOpenAIProviderHandler struct{}

func (h *AzureOpenAIProviderHandler) BuildConfig(model *agentv1alpha1.Model, modelConfig *agentv1alpha1.ModelConfig) (*ModelProviderConfig, error) {
	config := buildBaseConfig(model, modelConfig)

	// Add API key as secret env var
	addAPIKeyEnvVar(config, model.Spec.APIKeySecretRef, "AZURE_OPENAI_API_KEY")

	// Add Azure OpenAI-specific configuration
	if model.Spec.AzureOpenAI != nil {
		azureSpec := model.Spec.AzureOpenAI

		config.Config["endpoint"] = azureSpec.Endpoint
		config.Config["deploymentName"] = azureSpec.DeploymentName

		config.EnvVars = append(config.EnvVars,
			corev1.EnvVar{
				Name:  "AZURE_OPENAI_ENDPOINT",
				Value: azureSpec.Endpoint,
			},
			corev1.EnvVar{
				Name:  "AZURE_OPENAI_DEPLOYMENT",
				Value: azureSpec.DeploymentName,
			},
		)

		if azureSpec.APIVersion != "" {
			config.Config["apiVersion"] = azureSpec.APIVersion
			config.EnvVars = append(config.EnvVars, corev1.EnvVar{
				Name:  "AZURE_OPENAI_API_VERSION",
				Value: azureSpec.APIVersion,
			})
		}

		if azureSpec.TimeoutSeconds != nil {
			config.Config["timeoutSeconds"] = *azureSpec.TimeoutSeconds
		}
	}

	// Azure OpenAI uses the same parameters as OpenAI
	if modelConfig != nil && modelConfig.Spec.OpenAI != nil {
		openaiParams := modelConfig.Spec.OpenAI
		params := make(map[string]any)

		if openaiParams.FrequencyPenalty != "" {
			params["frequencyPenalty"] = openaiParams.FrequencyPenalty
		}
		if openaiParams.PresencePenalty != "" {
			params["presencePenalty"] = openaiParams.PresencePenalty
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
