package model

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// GoogleProviderHandler handles Google/Gemini provider configuration.
type GoogleProviderHandler struct{}

func (h *GoogleProviderHandler) Resolve(provider *agentv1alpha1.ModelProvider) (*ResolvedProvider, error) {
	// Vertex AI configuration (project/service-account) selects the
	// google-vertex model class; otherwise the Generative Language API.
	prefix := "google-gla"
	if provider.Spec.Google != nil &&
		(provider.Spec.Google.Project != "" || provider.Spec.Google.ServiceAccountKeySecretRef != nil) {
		prefix = "google-vertex"
	}

	resolved := &ResolvedProvider{
		Type:        agentv1alpha1.ProviderTypeGoogle,
		ModelPrefix: prefix,
	}

	AddAPIKeyEnvVar(resolved, provider.Spec.APIKeySecretRef, "GOOGLE_API_KEY")

	if provider.Spec.Google != nil {
		googleSpec := provider.Spec.Google

		if googleSpec.Project != "" {
			resolved.EnvVars = append(resolved.EnvVars, corev1.EnvVar{
				Name:  "GOOGLE_CLOUD_PROJECT",
				Value: googleSpec.Project,
			})
		}

		if googleSpec.Location != "" {
			resolved.EnvVars = append(resolved.EnvVars, corev1.EnvVar{
				Name:  "GOOGLE_CLOUD_REGION",
				Value: googleSpec.Location,
			})
		}

		if googleSpec.ServiceAccountKeySecretRef != nil {
			resolved.SecretEnvVars = append(resolved.SecretEnvVars, corev1.EnvVar{
				Name: "GOOGLE_APPLICATION_CREDENTIALS_JSON",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: googleSpec.ServiceAccountKeySecretRef,
				},
			})
		}
	}

	return resolved, nil
}
