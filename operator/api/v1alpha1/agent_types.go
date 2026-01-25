// api/v1alpha1/agent_types.go

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Framework represents the AI framework used by the agent
// +kubebuilder:validation:Enum=pydantic-ai;langchain;crewai;marvin;autogen;custom
type Framework string

const (
	FrameworkPydanticAI Framework = "pydantic-ai"
	FrameworkLangChain  Framework = "langchain"
	FrameworkCrewAI     Framework = "crewai"
	FrameworkMarvin     Framework = "marvin"
	FrameworkAutogen    Framework = "autogen"
	FrameworkA2A        Framework = "a2a"
)

// AgentPhase represents the current phase of the agent lifecycle
// +kubebuilder:validation:Enum=Pending;Running;Failed
type AgentPhase string

const (
	AgentPhasePending AgentPhase = "Pending"
	AgentPhaseRunning AgentPhase = "Running"
	AgentPhaseFailed  AgentPhase = "Failed"
)

// RuntimeType represents the type of runtime backend for the agent
// +kubebuilder:validation:Enum=standard
type RuntimeType string

const (
	RuntimeTypeStandard RuntimeType = "standard"
)

// AgentSpec defines the desired state of an Agent
type AgentSpec struct {
	// Runtime configuration - specifies the backend and configuration
	Runtime RuntimeSpec `json:"runtime"`

	// Explicit framework declaration (for observability/tooling)
	// +optional
	Framework Framework `json:"framework,omitempty"`

	// Tools available to this agent - can be inline definitions or references to AgentTool resources
	// +optional
	Tools []ToolEntry `json:"tools,omitempty"`
}

// ToolEntry represents either an inline tool definition or a reference to an AgentTool resource
type ToolEntry struct {
	// Name of the tool (required for inline tools, used as identifier)
	// +optional
	Name string `json:"name,omitempty"`

	// Inline tool definition - defines the tool directly in the Agent spec
	// +optional
	Inline *InlineToolSpec `json:"inline,omitempty"`

	// Reference to an existing AgentTool resource
	// +optional
	ToolRef *ToolRef `json:"toolRef,omitempty"`
}

// InlineToolSpec defines an inline tool specification (mirrors AgentToolSpec)
type InlineToolSpec struct {
	// Type of tool
	Type AgentToolType `json:"type"`

	// Human-readable description for the LLM
	Description string `json:"description"`

	// HTTP API specific configuration
	// +optional
	HTTPApi *HTTPApiSpec `json:"httpApi,omitempty"`

	// Input schema - JSON Schema defining what the agent provides
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	InputSchema *runtime.RawExtension `json:"inputSchema,omitempty"`

	// Output schema - JSON Schema defining what the tool returns
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	OutputSchema *runtime.RawExtension `json:"outputSchema,omitempty"`

	// Reference to an OpenAPI spec (alternative to inputSchema/outputSchema)
	// +optional
	OpenApiSchemaRef *OpenApiSchemaRef `json:"openApiSchemaRef,omitempty"`
}

// ToolRef references an existing AgentTool resource
type ToolRef struct {
	// Name of the AgentTool resource
	Name string `json:"name"`

	// Namespace of the AgentTool (defaults to Agent's namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// RuntimeSpec defines the runtime backend and its configuration
type RuntimeSpec struct {
	// Type of runtime backend
	// +kubebuilder:default=standard
	Type RuntimeType `json:"type"`

	// Spec contains the runtime-specific configuration
	// +optional
	Spec *StandardRuntimeSpec `json:"spec,omitempty"`
}

// StandardRuntimeSpec defines the configuration for the standard (Deployment-based) runtime.
// Uses corev1 types directly where possible.
type StandardRuntimeSpec struct {
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
	Phase AgentPhase `json:"phase,omitempty"`

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
	DetectedFramework Framework `json:"detectedFramework,omitempty"`

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
