// api/v1alpha1/agent_types.go

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentSpec defines the desired state of an Agent
type AgentSpec struct {
	// Runtime configuration - how to run the agent
	Runtime RuntimeSpec `json:"runtime"`

	// Tools to inject into the agent
	// +optional
	Tools []ToolReference `json:"tools,omitempty"`

	// MCP servers to connect
	// +optional
	MCPServers []MCPServerReference `json:"mcpServers,omitempty"`

	// Resource constraints
	// +optional
	Resources *AgentResources `json:"resources,omitempty"`

	// Scaling configuration (Knative backend only)
	// +optional
	Scaling *ScalingSpec `json:"scaling,omitempty"`

	// State backend for durable execution
	// +optional
	StateBackend *StateBackendSpec `json:"stateBackend,omitempty"`

	// Health check configuration
	// +optional
	HealthCheck *HealthCheckSpec `json:"healthCheck,omitempty"`
}

// RuntimeSpec defines how the agent is executed
type RuntimeSpec struct {
	// Container image containing the agent code
	Image string `json:"image"`

	// Lambda-style entrypoint: module.submodule.handler
	// The function should accept TaskInput and return TaskOutput
	Entrypoint string `json:"entrypoint"`

	// Explicit framework declaration (auto-detected if omitted)
	// +kubebuilder:validation:Enum=pydantic-ai;langchain;crewai;marvin;autogen;custom
	// +optional
	Framework string `json:"framework,omitempty"`

	// Override container command
	// +optional
	Command []string `json:"command,omitempty"`

	// Override container args
	// +optional
	Args []string `json:"args,omitempty"`

	// Environment variables
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Environment variables from ConfigMaps/Secrets
	// +optional
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

	// Image pull policy
	// +kubebuilder:default=IfNotPresent
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// Image pull secrets
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Service account to use
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// ToolReference references a Tool CRD
type ToolReference struct {
	// Name of the Tool resource
	ToolRef string `json:"toolRef"`

	// Namespace of the Tool (defaults to agent's namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// MCPServerReference references an MCPServer CRD
type MCPServerReference struct {
	// Name of the MCPServer resource
	MCPRef string `json:"mcpRef"`

	// Namespace of the MCPServer (defaults to agent's namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// AgentResources defines resource constraints
type AgentResources struct {
	// Standard k8s resource requests/limits
	// +optional
	Requests corev1.ResourceList `json:"requests,omitempty"`

	// +optional
	Limits corev1.ResourceList `json:"limits,omitempty"`

	// Agent-specific limits
	// +optional
	AgentLimits *AgentLimits `json:"agentLimits,omitempty"`
}

// AgentLimits defines agent-specific resource constraints
type AgentLimits struct {
	// Max tokens per invocation
	// +optional
	MaxTokensPerRun *int64 `json:"maxTokensPerRun,omitempty"`

	// Max cost per invocation in USD (as string for precision)
	// +optional
	MaxCostPerRunUSD *resource.Quantity `json:"maxCostPerRunUSD,omitempty"`

	// Max tool calls per invocation
	// +optional
	MaxToolCalls *int32 `json:"maxToolCalls,omitempty"`

	// Max run duration
	// +optional
	MaxRunDuration *metav1.Duration `json:"maxRunDuration,omitempty"`
}

// ScalingSpec defines autoscaling behavior (Knative backend)
type ScalingSpec struct {
	// Minimum replicas (0 enables scale-to-zero)
	// +kubebuilder:default=0
	// +optional
	MinScale *int32 `json:"minScale,omitempty"`

	// Maximum replicas
	// +kubebuilder:default=10
	// +optional
	MaxScale *int32 `json:"maxScale,omitempty"`

	// Target concurrent requests per replica
	// +optional
	TargetConcurrency *int32 `json:"targetConcurrency,omitempty"`

	// Scale-to-zero grace period
	// +optional
	ScaleDownDelay *metav1.Duration `json:"scaleDownDelay,omitempty"`
}

// StateBackendSpec defines where agent state is persisted
type StateBackendSpec struct {
	// Backend type
	// +kubebuilder:validation:Enum=redis;s3;postgres;memory
	Type string `json:"type"`

	// Secret containing connection credentials
	// +optional
	SecretRef string `json:"secretRef,omitempty"`

	// S3-specific configuration
	// +optional
	S3 *S3StateConfig `json:"s3,omitempty"`

	// TTL for state data
	// +optional
	TTL *metav1.Duration `json:"ttl,omitempty"`
}

// S3StateConfig defines S3-specific state backend options
type S3StateConfig struct {
	Bucket string `json:"bucket"`
	Prefix string `json:"prefix,omitempty"`
	Region string `json:"region,omitempty"`
}

// HealthCheckSpec defines health check configuration
type HealthCheckSpec struct {
	// Health check path
	// +kubebuilder:default="/health"
	// +optional
	Path string `json:"path,omitempty"`

	// Health check port
	// +kubebuilder:default=8080
	// +optional
	Port int32 `json:"port,omitempty"`

	// Check interval
	// +optional
	IntervalSeconds *int32 `json:"intervalSeconds,omitempty"`

	// Check timeout
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

// AgentStatus defines the observed state of Agent
type AgentStatus struct {
	// Current phase
	// +kubebuilder:validation:Enum=Pending;Running;Failed
	Phase string `json:"phase,omitempty"`

	// Backend being used
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