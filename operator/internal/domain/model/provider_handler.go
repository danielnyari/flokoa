package model

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// ResolvedProvider is what a ModelProvider contributes to a compiled agent:
// the pydantic-ai provider prefix for the model identifier, and the
// environment variables (connection config + API-key secret projections) the
// runner pod needs for that provider's SDK.
type ResolvedProvider struct {
	// Type is the provider type.
	Type agentv1alpha1.ProviderType

	// ModelPrefix is the pydantic-ai model identifier prefix
	// (e.g. "openai" for "openai:gpt-5-mini").
	ModelPrefix string

	// EnvVars are non-secret environment variables to set on the container.
	EnvVars []corev1.EnvVar

	// SecretEnvVars are environment variables sourced from secrets.
	SecretEnvVars []corev1.EnvVar
}

// ProviderHandler resolves provider-specific connection configuration.
type ProviderHandler interface {
	// Resolve derives the model prefix and environment projection from the
	// ModelProvider's connection configuration.
	Resolve(provider *agentv1alpha1.ModelProvider) (*ResolvedProvider, error)
}

// providerRegistry maps providers to their handlers
var providerRegistry = map[agentv1alpha1.ProviderType]ProviderHandler{
	agentv1alpha1.ProviderTypeOpenAI:    &OpenAIProviderHandler{},
	agentv1alpha1.ProviderTypeAnthropic: &AnthropicProviderHandler{},
	agentv1alpha1.ProviderTypeGoogle:    &GoogleProviderHandler{},
	agentv1alpha1.ProviderTypeBedrock:   &BedrockProviderHandler{},
}

// GetProviderHandler returns the handler for the given provider type.
func GetProviderHandler(providerType agentv1alpha1.ProviderType) (ProviderHandler, bool) {
	handler, ok := providerRegistry[providerType]
	return handler, ok
}

// AddAPIKeyEnvVar adds the API key secret reference as an environment variable.
func AddAPIKeyEnvVar(resolved *ResolvedProvider, secretRef *corev1.SecretKeySelector, envVarName string) {
	if secretRef == nil {
		return
	}

	resolved.SecretEnvVars = append(resolved.SecretEnvVars, corev1.EnvVar{
		Name: envVarName,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: secretRef,
		},
	})
}

// QualifiedModelName joins a provider prefix with a model identifier into a
// pydantic-ai model string (e.g. "openai" + "gpt-5-mini" → "openai:gpt-5-mini").
// Identifiers that already carry a prefix are returned unchanged.
func QualifiedModelName(prefix, model string) string {
	if strings.Contains(model, ":") {
		return model
	}
	return prefix + ":" + model
}
