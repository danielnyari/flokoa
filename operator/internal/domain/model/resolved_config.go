package model

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// ResolvedModelConfig contains the resolved configuration for a model.
// This is serialized to JSON and stored in a ConfigMap for the agent to consume.
type ResolvedModelConfig struct {
	// Provider contains provider type and connection configuration
	Provider ProviderConfig `json:"provider"`

	// Model is the model identifier (e.g., "gpt-4o", "claude-sonnet-4-20250514")
	Model string `json:"model"`

	// Parameters contains all model parameters
	Parameters *agentv1alpha1.ModelParameters `json:"parameters,omitempty"`

	// Internal fields (not serialized to JSON)

	// ConfigMapName is the name of the ConfigMap containing the model config
	ConfigMapName string `json:"-"`

	// EnvVars are non-secret environment variables to set on the container
	EnvVars []corev1.EnvVar `json:"-"`

	// SecretEnvVars are environment variables sourced from secrets
	SecretEnvVars []corev1.EnvVar `json:"-"`
}

// ProviderConfig contains the provider type and connection configuration.
// Only one of OpenAI, Anthropic, Google, or Bedrock will be set based on the Type.
type ProviderConfig struct {
	Type agentv1alpha1.ProviderType `json:"type"`

	// OpenAI provider configuration (only set for OpenAI providers)
	OpenAI *agentv1alpha1.OpenAIProviderSpec `json:"openai,omitempty"`

	// Anthropic provider configuration (only set for Anthropic providers)
	Anthropic *agentv1alpha1.AnthropicProviderSpec `json:"anthropic,omitempty"`

	// Google provider configuration (only set for Google providers)
	Google *agentv1alpha1.GoogleProviderSpec `json:"google,omitempty"`

	// Bedrock provider configuration (only set for Bedrock providers)
	Bedrock *agentv1alpha1.BedrockProviderSpec `json:"bedrock,omitempty"`

	// DefaultHeaders from ModelProvider
	DefaultHeaders map[string]string `json:"defaultHeaders,omitempty"`
}
