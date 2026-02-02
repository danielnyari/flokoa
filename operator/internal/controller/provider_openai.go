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

	// Add OpenAI-specific provider configuration
	if provider.Spec.OpenAI != nil {
		openaiSpec := provider.Spec.OpenAI

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

	// Add OpenAI-specific parameters from Model
	if model.Spec.Parameters != nil && model.Spec.Parameters.OpenAI != nil {
		openaiParams := model.Spec.Parameters.OpenAI
		params := make(map[string]any)

		if openaiParams.ReasoningEffort != nil {
			params["reasoningEffort"] = string(*openaiParams.ReasoningEffort)
		}
		if openaiParams.LogProbs != nil {
			params["logProbs"] = *openaiParams.LogProbs
		}
		if openaiParams.TopLogProbs != nil {
			params["topLogProbs"] = *openaiParams.TopLogProbs
		}
		if openaiParams.User != "" {
			params["user"] = openaiParams.User
		}
		if openaiParams.ServiceTier != "" {
			params["serviceTier"] = openaiParams.ServiceTier
		}
		if openaiParams.PromptCacheKey != "" {
			params["promptCacheKey"] = openaiParams.PromptCacheKey
		}
		if openaiParams.PromptRetention != "" {
			params["promptRetention"] = openaiParams.PromptRetention
		}
		if openaiParams.ReasoningGenerateSummary != nil {
			params["reasoningGenerateSummary"] = *openaiParams.ReasoningGenerateSummary
		}
		if openaiParams.ReasoningSummary != nil {
			params["reasoningSummary"] = *openaiParams.ReasoningSummary
		}
		if openaiParams.SendReasoningIDs != nil {
			params["sendReasoningIDs"] = *openaiParams.SendReasoningIDs
		}
		if openaiParams.Truncation != nil {
			params["truncation"] = *openaiParams.Truncation
		}
		if openaiParams.TextVerbosity != nil {
			params["textVerbosity"] = *openaiParams.TextVerbosity
		}
		if openaiParams.PreviousResponseID != "" {
			params["previousResponseID"] = openaiParams.PreviousResponseID
		}
		if openaiParams.IncludeCodeExecutionOutputs != nil {
			params["includeCodeExecutionOutputs"] = *openaiParams.IncludeCodeExecutionOutputs
		}
		if openaiParams.IncludeWebSearchSources != nil {
			params["includeWebSearchSources"] = *openaiParams.IncludeWebSearchSources
		}
		if openaiParams.IncludeFileSearchResults != nil {
			params["includeFileSearchResults"] = *openaiParams.IncludeFileSearchResults
		}
		if openaiParams.IncludeRawAnnotations != nil {
			params["includeRawAnnotations"] = *openaiParams.IncludeRawAnnotations
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
