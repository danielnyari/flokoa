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
// +kubebuilder:validation:Enum=openai;anthropic;azure-openai;ollama;gemini;gemini-vertex;anthropic-vertex;bedrock
type ModelProvider string

const (
	ModelProviderOpenAI          ModelProvider = "openai"
	ModelProviderAnthropic       ModelProvider = "anthropic"
	ModelProviderAzureOpenAI     ModelProvider = "azure-openai"
	ModelProviderOllama          ModelProvider = "ollama"
	ModelProviderGemini          ModelProvider = "gemini"
	ModelProviderGeminiVertex    ModelProvider = "gemini-vertex"
	ModelProviderAnthropicVertex ModelProvider = "anthropic-vertex"
	ModelProviderBedrock         ModelProvider = "bedrock"
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

	// Azure OpenAI-specific configuration
	// +optional
	AzureOpenAI *AzureOpenAIModelSpec `json:"azureOpenAI,omitempty"`

	// Ollama-specific configuration
	// +optional
	Ollama *OllamaModelSpec `json:"ollama,omitempty"`

	// Gemini-specific configuration
	// +optional
	Gemini *GeminiModelSpec `json:"gemini,omitempty"`

	// Vertex AI-specific configuration (for gemini-vertex and anthropic-vertex)
	// +optional
	VertexAI *VertexAIModelSpec `json:"vertexAI,omitempty"`

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

// AzureOpenAIModelSpec defines Azure OpenAI-specific configuration
type AzureOpenAIModelSpec struct {
	// Endpoint is the Azure OpenAI resource endpoint
	// e.g., "https://<resource-name>.openai.azure.com"
	Endpoint string `json:"endpoint"`

	// DeploymentName is the name of the model deployment
	DeploymentName string `json:"deploymentName"`

	// APIVersion is the Azure OpenAI API version
	// +kubebuilder:default="2024-02-01"
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// AzureADTokenSecretRef references a secret containing an Azure AD token
	// Use this instead of apiKeySecretRef for Azure AD authentication
	// +optional
	AzureADTokenSecretRef *corev1.SecretKeySelector `json:"azureADTokenSecretRef,omitempty"`

	// Timeout in seconds for API requests
	// +kubebuilder:default=60
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

// OllamaModelSpec defines Ollama-specific configuration
type OllamaModelSpec struct {
	// Host is the Ollama server address
	// +kubebuilder:default="http://localhost:11434"
	// +optional
	Host string `json:"host,omitempty"`

	// Options are additional Ollama-specific options
	// +optional
	Options map[string]string `json:"options,omitempty"`
}

// GeminiModelSpec defines Google Gemini-specific configuration
type GeminiModelSpec struct {
	// Timeout in seconds for API requests
	// +kubebuilder:default=60
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

// VertexAIModelSpec defines Vertex AI-specific configuration
type VertexAIModelSpec struct {
	// Project is the Google Cloud project ID
	Project string `json:"project"`

	// Location is the Google Cloud region (e.g., "us-central1")
	Location string `json:"location"`

	// ServiceAccountKeySecretRef references a secret containing the service account JSON key
	// +optional
	ServiceAccountKeySecretRef *corev1.SecretKeySelector `json:"serviceAccountKeySecretRef,omitempty"`
}

// BedrockModelSpec defines AWS Bedrock-specific configuration
type BedrockModelSpec struct {
	// Region is the AWS region
	Region string `json:"region"`

	// AccessKeySecretRef references a secret containing the AWS access key ID
	// +optional
	AccessKeySecretRef *corev1.SecretKeySelector `json:"accessKeySecretRef,omitempty"`

	// SecretKeySecretRef references a secret containing the AWS secret access key
	// +optional
	SecretKeySecretRef *corev1.SecretKeySelector `json:"secretKeySecretRef,omitempty"`

	// SessionTokenSecretRef references a secret containing the AWS session token (optional)
	// +optional
	SessionTokenSecretRef *corev1.SecretKeySelector `json:"sessionTokenSecretRef,omitempty"`

	// RoleARN is an optional IAM role to assume
	// +optional
	RoleARN string `json:"roleARN,omitempty"`
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
