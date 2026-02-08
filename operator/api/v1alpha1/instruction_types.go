// api/v1alpha1/instruction_types.go

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InstructionSpec defines the desired state of an Instruction (system prompt).
type InstructionSpec struct {
	// Content is the system prompt text that defines the agent's behavior.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Content string `json:"content"`
}

// InstructionStatus defines the observed state of an Instruction.
type InstructionStatus struct {
	// ConfigMapName is the name of the ConfigMap containing the instruction text.
	// +optional
	ConfigMapName string `json:"configMapName,omitempty"`

	// Conditions represent the latest available observations of the Instruction's state.
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

// Instruction is the Schema for the instructions API.
// It represents a system prompt that can be shared across agents.
type Instruction struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InstructionSpec   `json:"spec,omitempty"`
	Status InstructionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// InstructionList contains a list of Instruction
type InstructionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Instruction `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Instruction{}, &InstructionList{})
}
