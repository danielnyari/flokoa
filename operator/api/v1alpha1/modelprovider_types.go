/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProviderType represents the LLM provider type
// +kubebuilder:validation:Enum=openai;anthropic;google;bedrock
type ProviderType string

const (
	ProviderTypeOpenAI    ProviderType = "openai"
	ProviderTypeAnthropic ProviderType = "anthropic"
	ProviderTypeGoogle    ProviderType = "google"
	ProviderTypeBedrock   ProviderType = "bedrock"
)

// ModelProviderSpec defines the desired state of ModelProvider.
// Exactly one of OpenAI, Anthropic, Google, or Bedrock must be specified.
// The provider type is inferred from which block is set.
type ModelProviderSpec struct {
	// APIKeySecretRef references a secret containing the API key
	// The secret should have a key named "api-key" unless specified otherwise
	// +optional
	APIKeySecretRef *corev1.SecretKeySelector `json:"apiKeySecretRef,omitempty"`

	// OpenAI-specific configuration. If set, provider is OpenAI.
	// +optional
	OpenAI *OpenAIProviderSpec `json:"openai,omitempty"`

	// Anthropic-specific configuration. If set, provider is Anthropic.
	// +optional
	Anthropic *AnthropicProviderSpec `json:"anthropic,omitempty"`

	// Google-specific configuration. If set, provider is Google/Gemini.
	// +optional
	Google *GoogleProviderSpec `json:"google,omitempty"`

	// Bedrock-specific configuration. If set, provider is AWS Bedrock.
	// +optional
	Bedrock *BedrockProviderSpec `json:"bedrock,omitempty"`

	// TLS configuration for custom endpoints
	// +optional
	TLS *TLSConfig `json:"tls,omitempty"`

	// DefaultHeaders to include in all requests
	// +optional
	DefaultHeaders map[string]string `json:"defaultHeaders,omitempty"`
}

// OpenAIProviderSpec defines OpenAI-specific provider configuration
type OpenAIProviderSpec struct {
	// BaseURL overrides the default OpenAI API endpoint
	// +optional
	BaseURL string `json:"baseURL,omitempty"`

	// Timeout in seconds for API requests
	// +kubebuilder:default=60
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

// AnthropicProviderSpec defines Anthropic-specific provider configuration
type AnthropicProviderSpec struct {
	// BaseURL overrides the default Anthropic API endpoint
	// +optional
	BaseURL string `json:"baseURL,omitempty"`

	// Timeout in seconds for API requests
	// +kubebuilder:default=60
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

// GoogleProviderSpec defines Google/Gemini-specific provider configuration
type GoogleProviderSpec struct {
	// Timeout in seconds for API requests
	// +kubebuilder:default=60
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	// Project is the Google Cloud project ID (for Vertex AI)
	// +optional
	Project string `json:"project,omitempty"`

	// Location is the Google Cloud region (e.g., "us-central1", for Vertex AI)
	// +optional
	Location string `json:"location,omitempty"`

	// ServiceAccountKeySecretRef references a secret containing the service account JSON key
	// +optional
	ServiceAccountKeySecretRef *corev1.SecretKeySelector `json:"serviceAccountKeySecretRef,omitempty"`
}

// BedrockProviderSpec defines AWS Bedrock-specific provider configuration
type BedrockProviderSpec struct {
	// Region is the AWS region
	// +optional
	Region string `json:"region,omitempty"`
}

// TLSConfig defines TLS settings for custom endpoints
type TLSConfig struct {
	// InsecureSkipVerify disables TLS certificate verification
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`

	// CASecretRef references a secret containing a custom CA certificate
	// The secret should have a key named "ca.crt"
	// +optional
	CASecretRef *corev1.SecretKeySelector `json:"caSecretRef,omitempty"`

	// UseSystemCAs includes system CA certificates in addition to any custom CA
	// +kubebuilder:default=true
	// +optional
	UseSystemCAs *bool `json:"useSystemCAs,omitempty"`
}

// ModelProviderStatus defines the observed state of ModelProvider
type ModelProviderStatus struct {
	// Provider is the resolved provider type (inferred from which spec block is set)
	// +optional
	Provider ProviderType `json:"provider,omitempty"`

	// Standard conditions
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Observed generation
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// SecretHash is a hash of the referenced secrets for change detection
	// +optional
	SecretHash string `json:"secretHash,omitempty"`

	// Ready indicates if the model provider is ready to be used
	// +optional
	Ready bool `json:"ready,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Provider",type="string",JSONPath=".status.provider"
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ModelProvider is the Schema for the modelproviders API.
// It defines connection configuration for an LLM provider (OpenAI, Anthropic, Google, Bedrock).
// The provider type is inferred from which configuration block is set.
type ModelProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelProviderSpec   `json:"spec,omitempty"`
	Status ModelProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ModelProviderList contains a list of ModelProvider
type ModelProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModelProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ModelProvider{}, &ModelProviderList{})
}

// GetProviderType returns the provider type based on which spec block is set.
// Returns empty string if no provider block is set.
func (mp *ModelProvider) GetProviderType() ProviderType {
	switch {
	case mp.Spec.OpenAI != nil:
		return ProviderTypeOpenAI
	case mp.Spec.Anthropic != nil:
		return ProviderTypeAnthropic
	case mp.Spec.Google != nil:
		return ProviderTypeGoogle
	case mp.Spec.Bedrock != nil:
		return ProviderTypeBedrock
	default:
		return ""
	}
}
