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

	// Add Google-specific environment variables for SDK compatibility
	if provider.Spec.Google != nil {
		googleSpec := provider.Spec.Google

		if googleSpec.Project != "" {
			config.EnvVars = append(config.EnvVars, corev1.EnvVar{
				Name:  "GOOGLE_CLOUD_PROJECT",
				Value: googleSpec.Project,
			})
		}

		if googleSpec.Location != "" {
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

	return config, nil
}
