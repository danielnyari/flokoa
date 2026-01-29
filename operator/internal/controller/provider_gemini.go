package controller

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// GeminiProviderHandler handles Google Gemini (AI Studio) model configuration.
type GeminiProviderHandler struct{}

func (h *GeminiProviderHandler) BuildConfig(model *agentv1alpha1.Model, modelConfig *agentv1alpha1.ModelConfig) (*ModelProviderConfig, error) {
	config := buildBaseConfig(model, modelConfig)

	// Add API key as secret env var
	addAPIKeyEnvVar(config, model.Spec.APIKeySecretRef, "GOOGLE_API_KEY")

	// Add Gemini-specific configuration
	if model.Spec.Gemini != nil {
		geminiSpec := model.Spec.Gemini

		if geminiSpec.TimeoutSeconds != nil {
			config.Config["timeoutSeconds"] = *geminiSpec.TimeoutSeconds
		}
	}

	// Add Gemini-specific parameters from ModelConfig
	if modelConfig != nil && modelConfig.Spec.Gemini != nil {
		geminiParams := modelConfig.Spec.Gemini
		params := make(map[string]any)

		if geminiParams.CandidateCount != nil {
			params["candidateCount"] = *geminiParams.CandidateCount
		}
		if geminiParams.ResponseMimeType != "" {
			params["responseMimeType"] = geminiParams.ResponseMimeType
		}
		if len(geminiParams.SafetySettings) > 0 {
			safetySettings := make([]map[string]string, 0, len(geminiParams.SafetySettings))
			for _, s := range geminiParams.SafetySettings {
				safetySettings = append(safetySettings, map[string]string{
					"category":  s.Category,
					"threshold": s.Threshold,
				})
			}
			params["safetySettings"] = safetySettings
		}

		if len(params) > 0 {
			if config.Parameters == nil {
				config.Parameters = &ModelParametersConfig{}
			}
			config.Parameters.Gemini = params
		}
	}

	return config, nil
}

// VertexAIProviderHandler handles Gemini on Vertex AI model configuration.
type VertexAIProviderHandler struct{}

func (h *VertexAIProviderHandler) BuildConfig(model *agentv1alpha1.Model, modelConfig *agentv1alpha1.ModelConfig) (*ModelProviderConfig, error) {
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

	// Add Gemini-specific parameters from ModelConfig
	if modelConfig != nil && modelConfig.Spec.Gemini != nil {
		geminiParams := modelConfig.Spec.Gemini
		params := make(map[string]any)

		if geminiParams.CandidateCount != nil {
			params["candidateCount"] = *geminiParams.CandidateCount
		}
		if geminiParams.ResponseMimeType != "" {
			params["responseMimeType"] = geminiParams.ResponseMimeType
		}
		if len(geminiParams.SafetySettings) > 0 {
			safetySettings := make([]map[string]string, 0, len(geminiParams.SafetySettings))
			for _, s := range geminiParams.SafetySettings {
				safetySettings = append(safetySettings, map[string]string{
					"category":  s.Category,
					"threshold": s.Threshold,
				})
			}
			params["safetySettings"] = safetySettings
		}

		if len(params) > 0 {
			if config.Parameters == nil {
				config.Parameters = &ModelParametersConfig{}
			}
			config.Parameters.Gemini = params
		}
	}

	return config, nil
}
