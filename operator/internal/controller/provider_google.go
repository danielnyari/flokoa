package controller

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// GoogleProviderHandler handles Google/Gemini model configuration.
type GoogleProviderHandler struct{}

func (h *GoogleProviderHandler) BuildConfig(provider *agentv1alpha1.ModelProvider, model *agentv1alpha1.Model) (*ResolvedModelConfig, error) {
	config := buildBaseConfig(provider, model)

	// Add API key as secret env var
	addAPIKeyEnvVar(config, provider.Spec.APIKeySecretRef, "GOOGLE_API_KEY")

	// Add Google-specific provider configuration
	if provider.Spec.Google != nil {
		googleSpec := provider.Spec.Google

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

	// Add Google-specific parameters from Model
	if model.Spec.Parameters != nil && model.Spec.Parameters.Google != nil {
		googleParams := model.Spec.Parameters.Google
		params := make(map[string]any)

		if len(googleParams.SafetySettings) > 0 {
			safetySettings := make([]map[string]string, 0, len(googleParams.SafetySettings))
			for _, s := range googleParams.SafetySettings {
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

		if googleParams.ThinkingConfig != nil {
			thinking := make(map[string]any)
			if googleParams.ThinkingConfig.IncludeThoughts != nil {
				thinking["includeThoughts"] = *googleParams.ThinkingConfig.IncludeThoughts
			}
			if googleParams.ThinkingConfig.ThinkingBudget != nil {
				thinking["thinkingBudget"] = *googleParams.ThinkingConfig.ThinkingBudget
			}
			if googleParams.ThinkingConfig.ThinkingLevel != nil {
				thinking["thinkingLevel"] = string(*googleParams.ThinkingConfig.ThinkingLevel)
			}
			params["thinkingConfig"] = thinking
		}

		if len(googleParams.Labels) > 0 {
			params["labels"] = googleParams.Labels
		}

		if googleParams.VideoResolution != nil {
			params["videoResolution"] = string(*googleParams.VideoResolution)
		}

		if googleParams.CachedContent != "" {
			params["cachedContent"] = googleParams.CachedContent
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
