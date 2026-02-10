// api/v1alpha1/agent_types.go

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Framework represents the AI framework used by the agent.
// +kubebuilder:validation:Enum=pydantic-ai;langchain;crewai;marvin;autogen;a2a
type Framework string

const (
	// FrameworkPydanticAI represents the Pydantic AI framework.
	FrameworkPydanticAI Framework = "pydantic-ai"
	FrameworkLangChain  Framework = "langchain"
	FrameworkADK        Framework = "google-adk"
	FrameworkMarvin     Framework = "marvin"
	FrameworkAutogen    Framework = "autogen"
	FrameworkA2A        Framework = "a2a"
)

// AgentPhase represents the current phase of the agent lifecycle.
// +kubebuilder:validation:Enum=Pending;Running;Failed
type AgentPhase string

const (
	// AgentPhasePending indicates the agent is waiting to be scheduled.
	AgentPhasePending AgentPhase = "Pending"
	// AgentPhaseRunning indicates the agent is running and available.
	AgentPhaseRunning AgentPhase = "Running"
	// AgentPhaseFailed indicates the agent has failed to start or run.
	AgentPhaseFailed AgentPhase = "Failed"
)

// RuntimeType represents the type of runtime backend for the agent.
// +kubebuilder:validation:Enum=standard;managed
type RuntimeType string

const (
	// RuntimeTypeStandard uses a Kubernetes Deployment for the agent runtime.
	// The user provides their own container image.
	RuntimeTypeStandard RuntimeType = "standard"
	// RuntimeTypeManaged uses a generic runtime image fully managed by the operator.
	// The agent's behavior is defined entirely in the CR via instructions and output schema.
	RuntimeTypeManaged RuntimeType = "managed"
)

// AgentSkill describes a specific capability or function the agent can perform.
// Based on the A2A protocol AgentSkill definition.
type AgentSkill struct {
	// Unique identifier for the skill
	ID string `json:"id"`

	// Human-readable name for the skill
	Name string `json:"name"`

	// Detailed description of what the skill does
	Description string `json:"description"`

	// Keywords for categorization and discovery
	Tags []string `json:"tags"`

	// Sample prompts or use cases demonstrating the skill
	// +optional
	Examples []string `json:"examples,omitempty"`

	// Supported input MIME types for this skill, overriding the agent's defaults
	// +optional
	InputModes []string `json:"inputModes,omitempty"`

	// Supported output MIME types for this skill, overriding the agent's defaults
	// +optional
	OutputModes []string `json:"outputModes,omitempty"`

	// Security schemes necessary for the agent to leverage this skill
	// Each map entry represents security schemes that must be used together (AND)
	// Multiple entries represent alternatives (OR)
	// +optional
	Security []map[string][]string `json:"security,omitempty"`
}

type InputOutputMode string

const (
	InputOutputModeText InputOutputMode = "text"
	InputOutputModeJSON InputOutputMode = "application/json"
)

type AgentExtension struct {
	Description string                          `json:"description,omitempty"`
	Params      map[string]apiextensionsv1.JSON `json:"params,omitempty"`
	Required    bool                            `json:"required,omitempty"`
	URI         string                          `json:"uri,omitempty"`
}

type AgentCapabilities struct {
	Extensions             []AgentExtension `json:"extensions,omitempty"`
	PushNotifications      bool             `json:"pushNotifications,omitempty"`
	StateTransitionHistory bool             `json:"stateTransitionHistory,omitempty"`
	Streaming              bool             `json:"streaming,omitempty"`
}

type AgentCardOverride struct {
	Name string `json:"name"`

	Description string `json:"description"`

	Version string `json:"version"`

	// +kubebuilder:default={"application/json"}
	DefaultInputModes []InputOutputMode `json:"defaultInputModes"`

	// +kubebuilder:default={"application/json"}
	DefaultOutputModes []InputOutputMode `json:"defaultOutputModes"`

	// +kubebuilder:default={streaming: false}
	Capabilities AgentCapabilities `json:"capabilities"`

	Skills []AgentSkill `json:"skills"`
}

// AgentSpec defines the desired state of an Agent
type AgentSpec struct {
	CardOverride AgentCardOverride `json:"card"`

	// Runtime configuration - specifies the backend and configuration
	Runtime RuntimeSpec `json:"runtime"`

	// Model specifies the LLM model to use for this agent.
	// Can reference a Model resource directly (uses defaults) or a ModelConfig resource (full parameters).
	// +optional
	Model *AgentModelRef `json:"model,omitempty"`

	// Instruction defines the system prompt for this agent.
	// Can be defined inline (creates an Instruction CR) or reference an existing Instruction resource.
	// Supported by both standard and managed runtime types.
	// +optional
	Instruction *InstructionEntry `json:"instruction,omitempty"`

	// Framework explicitly declares the AI framework used by the agent.
	// Used for observability and tooling integration.
	// +optional
	Framework Framework `json:"framework,omitempty"`

	// Tools available to this agent. Can be inline definitions or references to AgentTool resources.
	// +optional
	Tools []ToolEntry `json:"tools,omitempty"`
}

// InstructionEntry represents either an inline instruction or a reference to an Instruction resource.
// Exactly one of Inline or InstructionRef must be specified.
type InstructionEntry struct {
	// Template defines the instruction content directly in the Agent spec.
	// When set, the operator creates a child Instruction CR.
	// +optional
	Template string `json:"template,omitempty"`

	// InstructionRef references an existing Instruction resource.
	// +optional
	InstructionRef *NamespacedRef `json:"instructionRef,omitempty"`
}

// ToolEntry represents either an inline tool definition or a reference to an AgentTool resource.
// Exactly one of Inline or ToolRef must be specified.
type ToolEntry struct {
	// Name of the tool. Required for inline tools, used as identifier.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Name string `json:"name,omitempty"`

	// Template defines the tool directly in the Agent spec.
	// Uses the same spec as AgentTool for consistency.
	// +optional
	Template *AgentToolSpec `json:"template,omitempty"`

	// ToolRef references an existing AgentTool resource.
	// +optional
	ToolRef *ToolRef `json:"toolRef,omitempty"`
}

// ToolRef references an existing AgentTool resource.
type ToolRef struct {
	// Name of the AgentTool resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace of the AgentTool. Defaults to the Agent's namespace if not specified.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// AgentModelRef specifies the model to use for the agent.
// References a Model resource which defines the model name and parameters.
type AgentModelRef struct {
	// Name of the Model resource.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace of the Model resource. Defaults to the Agent's namespace if not specified.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// NamespacedRef references a resource by name and optional namespace.
type NamespacedRef struct {
	// Name of the resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace of the resource. Defaults to the referencing resource's namespace if not specified.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// RuntimeSpec defines the runtime backend and its configuration.
// When type is "standard", the Standard field must be provided.
// When type is "managed", the Managed field must be provided.
type RuntimeSpec struct {
	// Type specifies the runtime backend to use.
	// +kubebuilder:default=standard
	// +kubebuilder:validation:Required
	Type RuntimeType `json:"type"`

	// Standard contains the standard runtime configuration (container-based).
	// Required when type is "standard".
	// +optional
	Standard *StandardRuntimeSpec `json:"standard,omitempty"`

	// Template contains the managed runtime configuration.
	// The operator generates a deployment using a generic runtime image with
	// the agent behavior defined by instructions and output schema.
	// Required when type is "managed".
	// +optional
	Template *TemplatedRuntimeSpec `json:"template,omitempty"`
}

// DeploymentOverrides contains pod-level scheduling and infrastructure fields
// shared across all runtime types.
type DeploymentOverrides struct {
	// Replicas is the number of desired pod replicas.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ImagePullSecrets is a list of references to secrets for pulling container images.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// ServiceAccountName is the name of the ServiceAccount to use for the pod.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// SecurityContext holds pod-level security attributes.
	// +optional
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`

	// NodeSelector is a selector for scheduling pods to nodes matching specific labels.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations allow the pod to schedule onto nodes with matching taints.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity specifies scheduling constraints for the pod.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
}

// StandardRuntimeSpec defines the configuration for the standard (Deployment-based) runtime.
// Uses corev1 types directly where possible for maximum compatibility.
type StandardRuntimeSpec struct {
	DeploymentOverrides `json:",inline"`

	// Container defines the main container spec for the agent pod.
	// +kubebuilder:validation:Required
	Container corev1.Container `json:"container"`

	// Volumes to mount into the pod.
	// +optional
	Volumes []corev1.Volume `json:"volumes,omitempty"`
}

type StructuredIOSchema struct {
	JSONSchema *apiextensionsv1.JSON `json:"jsonSchema"`

	Name string `json:"name"`

	Description string `json:"description"`
}

// TemplatedAgentConfig contains agent-specific configuration for the templated runtime.
// This configuration is serialized to JSON and mounted as a ConfigMap for the runtime to consume.
type TemplatedAgentConfig struct {

	// OutputSchema defines the JSON Schema for constraining agent response format.
	OutputSchema *StructuredIOSchema `json:"outputSchema"`
}

// TemplatedRuntimeSpec defines the configuration for a managed agent where the operator
// generates the deployment using a generic runtime image. The agent's behavior is
// defined via spec.instruction and output schema.
type TemplatedRuntimeSpec struct {
	DeploymentOverrides `json:",inline"`

	// Config contains agent-specific configuration (schemas, etc.) that is mounted
	// as a ConfigMap for the templated runtime to consume.
	Config *TemplatedAgentConfig `json:"config"`

	// Env allows injecting additional environment variables into the generated container.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Resources specifies compute resource requirements for the generated container.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// AgentStatus defines the observed state of Agent.
type AgentStatus struct {
	// Phase represents the current lifecycle phase of the agent.
	// +optional
	Phase AgentPhase `json:"phase,omitempty"`

	// Backend indicates the runtime backend being used (e.g., standard).
	// +optional
	Backend string `json:"backend,omitempty"`

	// URL is the endpoint for invoking the agent.
	// +optional
	URL string `json:"url,omitempty"`

	// Replicas is the current number of pod replicas.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// AvailableReplicas is the number of replicas that are ready and available.
	// +optional
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// LastToolSync is the last time tools were synchronized to the agent.
	// +optional
	LastToolSync *metav1.Time `json:"lastToolSync,omitempty"`

	// DetectedFramework is the AI framework detected from the container image.
	// +optional
	DetectedFramework Framework `json:"detectedFramework,omitempty"`

	// Conditions represent the latest available observations of the agent's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
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
