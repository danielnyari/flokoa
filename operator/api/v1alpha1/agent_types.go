// api/v1alpha1/agent_types.go

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentSpec defines the desired state of an Agent
type AgentSpec struct {
	// Runtime configuration - maps to the underlying Deployment
	Runtime RuntimeSpec `json:"runtime"`

	// Explicit framework declaration (for observability/tooling)
	// +kubebuilder:validation:Enum=pydantic-ai;langchain;crewai;marvin;autogen;custom
	// +optional
	Framework string `json:"framework,omitempty"`
}

// RuntimeSpec defines the Deployment configuration for the agent.
// Uses corev1 types directly where possible.
type RuntimeSpec struct {
	// Number of replicas
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Container spec
	Container corev1.Container `json:"container"`

	// Volumes to mount into the pod
	// +optional
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// Image pull secrets
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Service account to use
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Pod-level security context
	// +optional
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`

	// Node selector
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity rules
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
}

// AgentStatus defines the observed state of Agent
type AgentStatus struct {
	// Current phase: Pending, Running, Failed
	// +kubebuilder:validation:Enum=Pending;Running;Failed
	Phase string `json:"phase,omitempty"`

	// Backend being used (core, knative)
	Backend string `json:"backend,omitempty"`

	// Endpoint URL for invoking the agent
	URL string `json:"url,omitempty"`

	// Current replica count
	Replicas int32 `json:"replicas,omitempty"`

	// Available replicas
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// Last time tools were synced
	LastToolSync *metav1.Time `json:"lastToolSync,omitempty"`

	// Detected framework (if auto-detected)
	DetectedFramework string `json:"detectedFramework,omitempty"`

	// Standard conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Observed generation
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Backend",type="string",JSONPath=".status.backend"
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.availableReplicas"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Agent is the Schema for the agents API
type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSpec   `json:"spec,omitempty"`
	Status AgentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentList contains a list of Agent
type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Agent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Agent{}, &AgentList{})
}
