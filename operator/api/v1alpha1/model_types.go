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

// ModelProvider represents the LLM provider
// +kubebuilder:validation:Enum=openai;anthropic;google;bedrock
type ModelProvider string

const (
	ModelProviderOpenAI    ModelProvider = "openai"
	ModelProviderAnthropic ModelProvider = "anthropic"
	ModelProviderGoogle    ModelProvider = "google"
	ModelProviderBedrock   ModelProvider = "bedrock"
)

// ModelSpec defines the desired state of Model
type ModelSpec struct {
	// Provider specifies the LLM provider
	Provider ModelProvider `json:"provider"`

	// Model is the name/identifier of the model (e.g., "gpt-4o", "claude-sonnet-4-20250514")
	Model string `json:"model"`

	// APIKeySecretRef references a secret containing the API key
	// The secret should have a key named "api-key" unless specified otherwise
	// +optional
	APIKeySecretRef *corev1.SecretKeySelector `json:"apiKeySecretRef,omitempty"`

	// OpenAI-specific configuration
	// +optional
	OpenAI *OpenAIModelSpec `json:"openai,omitempty"`

	// Anthropic-specific configuration
	// +optional
	Anthropic *AnthropicModelSpec `json:"anthropic,omitempty"`

	// Google-specific configuration
	// +optional
	Google *GoogleModelSpec `json:"google,omitempty"`

	// Bedrock-specific configuration
	// +optional
	Bedrock *BedrockModelSpec `json:"bedrock,omitempty"`

	// TLS configuration for custom endpoints
	// +optional
	TLS *TLSConfig `json:"tls,omitempty"`

	// DefaultHeaders to include in all requests
	// +optional
	DefaultHeaders map[string]string `json:"defaultHeaders,omitempty"`
}

// OpenAIModelSpec defines OpenAI-specific configuration
type OpenAIModelSpec struct {
	// BaseURL overrides the default OpenAI API endpoint
	// +optional
	BaseURL string `json:"baseURL,omitempty"`

	// Organization ID for OpenAI API requests
	// +optional
	OrganizationID string `json:"organizationID,omitempty"`

	// Timeout in seconds for API requests
	// +kubebuilder:default=60
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

// AnthropicModelSpec defines Anthropic-specific configuration
type AnthropicModelSpec struct {
	// BaseURL overrides the default Anthropic API endpoint
	// +optional
	BaseURL string `json:"baseURL,omitempty"`

	// Timeout in seconds for API requests
	// +kubebuilder:default=60
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

// GoogleModelSpec defines Google/Gemini-specific configuration
type GoogleModelSpec struct {
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

// BedrockModelSpec defines AWS Bedrock-specific configuration
type BedrockModelSpec struct {
	// Region is the AWS region
	Region string `json:"region"`

	// InferenceProfileARN is the ARN of the Bedrock inference profile to use
	// +optional
	InferenceProfileARN string `json:"inferenceProfileARN,omitempty"`
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

// ModelStatus defines the observed state of Model
type ModelStatus struct {
	// Standard conditions
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Observed generation
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// SecretHash is a hash of the referenced secrets for change detection
	// +optional
	SecretHash string `json:"secretHash,omitempty"`

	// Ready indicates if the model is ready to be used
	// +optional
	Ready bool `json:"ready,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Provider",type="string",JSONPath=".spec.provider"
// +kubebuilder:printcolumn:name="Model",type="string",JSONPath=".spec.model"
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Model is the Schema for the models API.
// It defines an LLM model from a specific provider and how to connect to it.
type Model struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelSpec   `json:"spec,omitempty"`
	Status ModelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ModelList contains a list of Model
type ModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Model `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Model{}, &ModelList{})
}
