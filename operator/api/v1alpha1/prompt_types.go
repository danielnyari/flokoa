// api/v1alpha1/prompt_types.go

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PromptSource defines where the prompt template content is sourced from.
// Exactly one of Value or ValueFrom must be specified.
// This is designed to be extended with additional external sources (e.g., Langfuse, Langsmith) in the future.
type PromptSource struct {
	// Value contains the prompt template inline as a Jinja-style template string.
	// +optional
	Value *string `json:"value,omitempty"`

	// ValueFrom references a ConfigMap key containing the prompt template.
	// +optional
	ValueFrom *corev1.ConfigMapKeySelector `json:"valueFrom,omitempty"`
}

// PromptVariable defines a variable that can be used in the prompt template.
type PromptVariable struct {
	// Name is the variable name used in the template (e.g., "user_name").
	// Must be a valid identifier: start with a letter or underscore, followed by letters, digits, or underscores.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[a-zA-Z_][a-zA-Z0-9_]*$`
	Name string `json:"name"`

	// Description explains the purpose of this variable.
	// +optional
	Description string `json:"description,omitempty"`

	// Default is the default value if the variable is not provided at render time.
	// +optional
	Default string `json:"default,omitempty"`

	// Required indicates whether this variable must be provided at render time.
	// +optional
	Required bool `json:"required,omitempty"`
}

// PromptSpec defines the desired state of a Prompt (Jinja-style template).
type PromptSpec struct {
	// Source defines where the prompt template content comes from.
	// Exactly one of Value or ValueFrom must be specified.
	// +kubebuilder:validation:Required
	Source PromptSource `json:"source"`

	// Variables defines the template variables available for use in the prompt template.
	// Variable names must be unique.
	// +optional
	Variables []PromptVariable `json:"variables,omitempty"`
}

// PromptStatus defines the observed state of a Prompt.
type PromptStatus struct {
	// ConfigMapName is the name of the ConfigMap containing the resolved prompt template.
	// +optional
	ConfigMapName string `json:"configMapName,omitempty"`

	// Conditions represent the latest available observations of the Prompt's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ConfigMap",type="string",JSONPath=".status.configMapName"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Prompt is the Schema for the prompts API.
// It represents a Jinja-style prompt template that can be shared across agents.
// Unlike Instruction (which holds a static system prompt), Prompt holds a
// templateable prompt that supports variable substitution at render time.
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
