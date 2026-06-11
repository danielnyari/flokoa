// api/v1alpha1/agent_types.go

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// IsolationTier selects how sessions map onto runner pods.
// +kubebuilder:validation:Enum=shared;session
type IsolationTier string

const (
	// IsolationShared serves all sessions from a shared pool of runner pods.
	IsolationShared IsolationTier = "shared"
	// IsolationSession gives each A2A context its own sandbox pod (P1; the
	// admission webhook rejects it until the session router and pools ship).
	IsolationSession IsolationTier = "session"
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

// AgentSpec defines the desired state of an Agent.
//
// An Agent is the composition root of a pydantic-ai AgentSpec: the inline
// fragment plus Model/Instruction/AgentTool/Capability references compile into
// one resolved spec (merge precedence: referenced CRs compose in declared
// order, inline scalars win conflicts, list fields append), validated against
// the runner's pinned AgentSpec JSON Schema and delivered to the generic
// runner as a single ConfigMap.
type AgentSpec struct {
	// Spec is an inline pydantic-ai AgentSpec fragment (typed where upstream
	// is stable; see AgentSpecFragment).
	// +optional
	Spec *AgentSpecFragment `json:"spec,omitempty"`

	// ModelRef references a Model CR. The fragment's inline model wins over
	// the reference if both are set.
	// +optional
	ModelRef *NamespacedRef `json:"modelRef,omitempty"`

	// InstructionRefs compose Instruction CRs, in order, before the
	// fragment's instructions.
	// +optional
	InstructionRefs []NamespacedRef `json:"instructionRefs,omitempty"`

	// Tools reference AgentTool CRs (declarative MCP endpoints), compiled to
	// MCP capability entries after the fragment's capabilities.
	// +optional
	Tools []NamespacedRef `json:"tools,omitempty"`

	// Capabilities reference Capability CRs with per-agent config, validated
	// against the capability's published schema at admission.
	// Capability CRs ship in a later release (roadmap 08); the webhook
	// rejects entries until then.
	// +optional
	Capabilities []CapabilityAttachment `json:"capabilities,omitempty"`

	// SecretRefs names the secrets resolvable via ${secret:NAME} placeholders
	// in the compiled spec (runtime contract §3). Values are projected as
	// FLOKOA_SECRET_* environment variables and resolved in the runner;
	// they never appear in the compiled spec ConfigMap.
	// +optional
	SecretRefs map[string]corev1.SecretKeySelector `json:"secretRefs,omitempty"`

	// Card configures the published A2A agent card.
	Card AgentCardOverride `json:"card"`

	// Runtime configures how the compiled spec runs.
	// +optional
	Runtime AgentRuntime `json:"runtime,omitempty"`
}

// AgentSpecFragment mirrors the stable subset of pydantic-ai's AgentSpec with
// typed fields. Anything additive upstream flows through Extra; the compiled
// spec is always validated against the pinned AgentSpec JSON Schema (the
// backstop that keeps this fragment honest across runner bumps).
type AgentSpecFragment struct {
	// Model is a pydantic-ai model identifier (e.g. "openai:gpt-5-mini").
	// Wins over modelRef when both are set.
	// +optional
	Model string `json:"model,omitempty"`

	// Name overrides the agent name in the compiled spec (defaults to the CR name).
	// +optional
	Name string `json:"name,omitempty"`

	// Description of the agent, used by sub-agent and handoff tooling.
	// +optional
	Description string `json:"description,omitempty"`

	// Instructions are appended after all instructionRefs content.
	// +optional
	Instructions []string `json:"instructions,omitempty"`

	// ModelSettings overrides settings from the referenced Model per-key
	// (inline scalars win conflicts).
	// +optional
	ModelSettings *ModelSettings `json:"modelSettings,omitempty"`

	// OutputSchema constrains the agent's output to a JSON Schema
	// (compiles to AgentSpec output_schema).
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	OutputSchema *apiextensionsv1.JSON `json:"outputSchema,omitempty"`

	// Capabilities are native pydantic-ai capability entries (WebSearch,
	// WebFetch, MCP, Thinking, ToolSearch, …) by name plus config. Harness
	// and third-party capability class paths are rejected here — they ship
	// through Capability CRs only.
	// +optional
	Capabilities []NativeCapabilityEntry `json:"capabilities,omitempty"`

	// Extra passes additional top-level AgentSpec fields through to the
	// compiled spec (additive upstream fields such as retries or
	// end_strategy). Keys here lose to typed fields on conflict.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Extra *apiextensionsv1.JSON `json:"extra,omitempty"`
}

// NativeCapabilityEntry names a capability from the runner baseline with its
// configuration. It compiles to the capability-spec form of the pinned
// pydantic-ai version ("Name" or {"Name": config}).
type NativeCapabilityEntry struct {
	// Name of the native capability as serialized in AgentSpec (e.g.
	// "WebSearch", "Thinking", "MCP").
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Config holds the capability's keyword arguments, validated against the
	// embedded AgentSpec schema at compile time.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Config *apiextensionsv1.JSON `json:"config,omitempty"`
}

// CapabilityAttachment references a Capability CR with per-agent config.
type CapabilityAttachment struct {
	Ref NamespacedRef `json:"ref"`

	// Config is validated against the Capability's published JSON Schema at
	// admission.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Config *apiextensionsv1.JSON `json:"config,omitempty"`
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

// AgentRuntime configures how the compiled spec runs.
type AgentRuntime struct {
	// Image overrides the generic runner image (escape hatch; custom images
	// own their bootstrap and must honor the runtime contract's interface).
	// +optional
	Image string `json:"image,omitempty"`

	// RunnerVersion pins a runner release. Defaults to the operator's
	// current runner version; only versions with an embedded AgentSpec
	// schema are accepted.
	// +optional
	RunnerVersion string `json:"runnerVersion,omitempty"`

	// Isolation selects the session isolation tier.
	// +kubebuilder:default=shared
	// +optional
	Isolation IsolationTier `json:"isolation,omitempty"`

	// Env injects additional environment variables into the runner container.
	// User entries win over operator-injected ones on name conflicts.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Resources specifies compute resource requirements for the runner container.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	DeploymentOverrides `json:",inline"`
}

// DeploymentOverrides contains pod-level scheduling and infrastructure fields.
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

// AgentStatus defines the observed state of Agent.
type AgentStatus struct {
	// Phase represents the current lifecycle phase of the agent.
	// +optional
	Phase AgentPhase `json:"phase,omitempty"`

	// URL is the published endpoint for invoking the agent. Callers must
	// treat it as opaque: port, path, and backing topology may change behind
	// it (virtual endpoint identity).
	// +optional
	URL string `json:"url,omitempty"`

	// SpecHash is the hash of the last successfully compiled AgentSpec.
	// Changes whenever any part of the composition graph changes.
	// +optional
	SpecHash string `json:"specHash,omitempty"`

	// RunnerVersion is the runner release the compiled spec was validated
	// against.
	// +optional
	RunnerVersion string `json:"runnerVersion,omitempty"`

	// InjectedCapabilities lists the platform capability entries the
	// operator appended to the compiled spec.
	// +optional
	InjectedCapabilities []string `json:"injectedCapabilities,omitempty"`

	// Replicas is the current number of pod replicas.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// AvailableReplicas is the number of replicas that are ready and available.
	// +optional
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

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
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.availableReplicas"
// +kubebuilder:printcolumn:name="SpecHash",type="string",JSONPath=".status.specHash"
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
