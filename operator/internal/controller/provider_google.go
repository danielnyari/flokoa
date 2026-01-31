package controller

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// GoogleProviderHandler handles Google/Gemini model configuration.
type GoogleProviderHandler struct{}

func (h *GoogleProviderHandler) BuildConfig(model *agentv1alpha1.Model, modelConfig *agentv1alpha1.ModelConfig) (*ModelProviderConfig, error) {
	config := buildBaseConfig(model, modelConfig)

	// Add API key as secret env var
	addAPIKeyEnvVar(config, model.Spec.APIKeySecretRef, "GOOGLE_API_KEY")

	// Add Google-specific configuration
	if model.Spec.Google != nil {
		googleSpec := model.Spec.Google

		if googleSpec.TimeoutSeconds != nil {
			config.Config["timeoutSeconds"] = *googleSpec.TimeoutSeconds
		}

		// Vertex AI configuration
		if googleSpec.Project != "" {
			config.Config["project"] = googleSpec.Project
			config.EnvVars = append(config.EnvVars, corev1.EnvVar{
				Name:  "GOOGLE_CLOUD_PROJECT",
				Value: googleSpec.Project,
			})
		}

		if googleSpec.Location != "" {
			config.Config["location"] = googleSpec.Location
			config.EnvVars = append(config.EnvVars, corev1.EnvVar{
				Name:  "GOOGLE_CLOUD_REGION",
				Value: googleSpec.Location,
			})
		}

		// Add service account key as secret env var (for Vertex AI)
		if googleSpec.ServiceAccountKeySecretRef != nil {
			config.SecretEnvVars = append(config.SecretEnvVars, corev1.EnvVar{
				Name: "GOOGLE_APPLICATION_CREDENTIALS_JSON",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: googleSpec.ServiceAccountKeySecretRef,
				},
			})
		}
	}

	// Add Google-specific parameters from ModelConfig
	if modelConfig != nil && modelConfig.Spec.Google != nil {
		googleParams := modelConfig.Spec.Google
		params := make(map[string]any)

		if len(googleParams.GoogleSafetySettings) > 0 {
			safetySettings := make([]map[string]string, 0, len(googleParams.GoogleSafetySettings))
			for _, s := range googleParams.GoogleSafetySettings {
				setting := map[string]string{
					"category":  s.Category,
					"threshold": s.Threshold,
				}
				if s.Method != "" {
					setting["method"] = s.Method
				}
				safetySettings = append(safetySettings, setting)
			}
			params["safetySettings"] = safetySettings
		}

		if googleParams.GoogleThinkingConfig != nil {
			thinking := make(map[string]any)
			if googleParams.GoogleThinkingConfig.IncludeThoughts != nil {
				thinking["includeThoughts"] = *googleParams.GoogleThinkingConfig.IncludeThoughts
			}
			if googleParams.GoogleThinkingConfig.ThinkingBudget != nil {
				thinking["thinkingBudget"] = *googleParams.GoogleThinkingConfig.ThinkingBudget
			}
			if googleParams.GoogleThinkingConfig.ThinkingLevel != nil {
				thinking["thinkingLevel"] = string(*googleParams.GoogleThinkingConfig.ThinkingLevel)
			}
			params["thinkingConfig"] = thinking
		}

		if len(googleParams.GoogleLabels) > 0 {
			params["labels"] = googleParams.GoogleLabels
		}

		if googleParams.GoogleVideoResolution != nil {
			params["videoResolution"] = string(*googleParams.GoogleVideoResolution)
		}

		if googleParams.GoogleCachedContent != "" {
			params["cachedContent"] = googleParams.GoogleCachedContent
		}

		if len(params) > 0 {
			if config.Parameters == nil {
				config.Parameters = &ModelParametersConfig{}
			}
			config.Parameters.Google = params
		}
	}

	return config, nil
}
