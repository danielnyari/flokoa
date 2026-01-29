package controller

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// OllamaProviderHandler handles Ollama model configuration.
type OllamaProviderHandler struct{}

func (h *OllamaProviderHandler) BuildConfig(model *agentv1alpha1.Model, modelConfig *agentv1alpha1.ModelConfig) (*ModelProviderConfig, error) {
	config := buildBaseConfig(model, modelConfig)

	// Ollama typically doesn't require an API key, but support it if provided
	if model.Spec.APIKeySecretRef != nil {
		addAPIKeyEnvVar(config, model.Spec.APIKeySecretRef, "OLLAMA_API_KEY")
	}

	// Add Ollama-specific configuration
	if model.Spec.Ollama != nil {
		ollamaSpec := model.Spec.Ollama

		host := ollamaSpec.Host
		if host == "" {
			host = "http://localhost:11434"
		}

		config.Config["host"] = host
		config.EnvVars = append(config.EnvVars, corev1.EnvVar{
			Name:  "OLLAMA_HOST",
			Value: host,
		})

		if len(ollamaSpec.Options) > 0 {
			config.Config["options"] = ollamaSpec.Options
		}
	}

	return config, nil
}
