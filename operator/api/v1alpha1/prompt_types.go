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

// PromptSpec defines the desired state of Prompt.
// Exactly one of Langfuse, Langsmith, or Inline must be specified in the Source.
type PromptSpec struct {
	// Source defines where the prompt content comes from.
	// Exactly one of Langfuse, Langsmith, or Inline must be specified.
	// +kubebuilder:validation:Required
	Source PromptSource `json:"source"`

	// Sync defines optional refresh behavior for non-inline prompts.
	// +optional
	Sync *PromptSync `json:"sync,omitempty"`
}

// PromptSource defines the source of the prompt content.
// Exactly one of Langfuse, Langsmith, or Inline must be specified.
type PromptSource struct {
	// Langfuse specifies a prompt from Langfuse.
	// +optional
	Langfuse *LangfuseSource `json:"langfuse,omitempty"`

	// Langsmith specifies a prompt from Langsmith.
	// +optional
	Langsmith *LangsmithSource `json:"langsmith,omitempty"`

	// Inline specifies the prompt content directly.
	// +optional
	Inline *InlineSource `json:"inline,omitempty"`
}

// LangfuseSource defines configuration for fetching prompts from Langfuse.
type LangfuseSource struct {
	// Endpoint is the Langfuse API endpoint.
	// +kubebuilder:default="https://cloud.langfuse.com"
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// CredentialsSecretRef references a secret containing Langfuse credentials.
	// The secret should contain keys: "publicKey" and "secretKey".
	// +kubebuilder:validation:Required
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`

	// PromptName is the name of the prompt in Langfuse.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	PromptName string `json:"promptName"`

	// Version is the version of the prompt to fetch.
	// Use "latest" to always fetch the latest version.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`
}

// LangsmithSource defines configuration for fetching prompts from Langsmith.
type LangsmithSource struct {
	// CredentialsSecretRef references a secret containing Langsmith credentials.
	// The secret should contain a key: "apiKey".
	// +kubebuilder:validation:Required
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`

	// PromptName is the name of the prompt in Langsmith.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	PromptName string `json:"promptName"`

	// CommitHash is the specific commit hash to fetch.
	// Use "latest" to always fetch the latest commit.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	CommitHash string `json:"commitHash"`
}

// InlineSource defines an inline prompt content.
type InlineSource struct {
	// Content is the prompt content.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Content string `json:"content"`
}

// PromptSync defines refresh behavior for external prompt sources.
type PromptSync struct {
	// Interval is the duration between sync attempts.
	// Format: "5m", "1h", "30s", etc.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([0-9]+(\.[0-9]+)?(s|m|h))+$`
	Interval string `json:"interval"`
}

// PromptStatus defines the observed state of Prompt.
type PromptStatus struct {
	// ResolvedContent is the current resolved prompt content.
	// +optional
	ResolvedContent string `json:"resolvedContent,omitempty"`

	// ResolvedAt is the timestamp when the content was last resolved.
	// +optional
	ResolvedAt *metav1.Time `json:"resolvedAt,omitempty"`

	// SourceVersion is the version/commit from the external source.
	// +optional
	SourceVersion string `json:"sourceVersion,omitempty"`

	// Checksum is the SHA256 hash of the resolved content.
	// +optional
	Checksum string `json:"checksum,omitempty"`

	// Conditions represent the latest available observations of the prompt's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Source",type="string",JSONPath=".status.sourceVersion"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Prompt is the Schema for the prompts API.
// It defines a prompt with its content source (Langfuse, Langsmith, or inline).
type Prompt struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PromptSpec   `json:"spec,omitempty"`
	Status PromptStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PromptList contains a list of Prompt
type PromptList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Prompt `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Prompt{}, &PromptList{})
}
