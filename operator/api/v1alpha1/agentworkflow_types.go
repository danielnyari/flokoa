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

// EngineType represents the workflow execution backend.
// +kubebuilder:validation:Enum=argo;temporal
type EngineType string

const (
	// EngineTypeArgo uses Argo Workflows as the execution backend.
	EngineTypeArgo EngineType = "argo"
	// EngineTypeTemporal uses Temporal as the execution backend.
	EngineTypeTemporal EngineType = "temporal"
)

// WorkflowPhase represents the current phase of the workflow execution.
// +kubebuilder:validation:Enum=Pending;Compiling;Running;Succeeded;Failed;Error
type WorkflowPhase string

const (
	WorkflowPhasePending   WorkflowPhase = "Pending"
	WorkflowPhaseCompiling WorkflowPhase = "Compiling"
	WorkflowPhaseRunning   WorkflowPhase = "Running"
	WorkflowPhaseSucceeded WorkflowPhase = "Succeeded"
	WorkflowPhaseFailed    WorkflowPhase = "Failed"
	WorkflowPhaseError     WorkflowPhase = "Error"
)

// AgentWorkflowSpec defines the desired state of AgentWorkflow.
type AgentWorkflowSpec struct {
	// Description is a human-readable description of the workflow.
	// +optional
	Description string `json:"description,omitempty"`

	// Engine specifies the workflow execution backend.
	// Defaults to platform configuration if not set.
	// +optional
	// +kubebuilder:default=argo
	Engine EngineType `json:"engine,omitempty"`

	// Params are workflow-level parameters that can be referenced in expressions.
	// +optional
	Params []WorkflowParam `json:"params,omitempty"`

	// Tasks defines the workflow tasks to execute.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Tasks []WorkflowTask `json:"tasks"`

	// Timeout is the maximum duration for the entire workflow.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// RetryStrategy is the default retry policy for all tasks.
	// Individual tasks can override this.
	// +optional
	RetryStrategy *WorkflowRetryStrategy `json:"retryStrategy,omitempty"`
}

// WorkflowParam defines a workflow-level parameter.
type WorkflowParam struct {
	// Name of the parameter.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Value is the parameter value.
	// +optional
	Value string `json:"value,omitempty"`
}

// WorkflowTask defines a single task in the workflow.
// Exactly one of Agent, AgentTask, WaitForSignal, or Switch must be specified.
type WorkflowTask struct {
	// Name is the unique identifier for this task within the workflow.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9][-a-zA-Z0-9]*$`
	Name string `json:"name"`

	// Agent calls a deployed Agent CR via the A2A protocol.
	// +optional
	Agent *AgentCall `json:"agent,omitempty"`

	// AgentTask runs agent code in an ephemeral container.
	// +optional
	AgentTask *EphemeralAgentTask `json:"agentTask,omitempty"`

	// WaitForSignal pauses the workflow until an external signal is received.
	// Only supported when engine is "temporal".
	// +optional
	WaitForSignal *WaitForSignalSpec `json:"waitForSignal,omitempty"`

	// Switch routes to different tasks based on the output of a previous task.
	// +optional
	Switch []SwitchCase `json:"switch,omitempty"`

	// DependsOn lists task names that must complete before this task starts.
	// Defines DAG edges.
	// +optional
	DependsOn []string `json:"dependsOn,omitempty"`

	// Timeout is the maximum duration for this task.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// RetryStrategy overrides the workflow-level retry policy for this task.
	// +optional
	RetryStrategy *WorkflowRetryStrategy `json:"retryStrategy,omitempty"`

	// Condition is an expression that must evaluate to true for this task to run.
	// If false, the task is skipped.
	// +optional
	Condition string `json:"condition,omitempty"`

	// Loop enables iterative execution of this task until a condition is met.
	// Only supported when engine is "temporal".
	// +optional
	Loop *LoopSpec `json:"loop,omitempty"`
}

// AgentCall defines a task that calls a deployed Agent CR via the A2A protocol.
type AgentCall struct {
	// Name is the name of the Agent CR to call.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace is the namespace of the Agent CR.
	// Defaults to the workflow namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Message is the message to send to the agent.
	// Supports expressions like {{params.topic}} or {{tasks.prev.output}}.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Message string `json:"message"`
}

// EphemeralAgentTask defines a task that runs agent code in a short-lived container.
type EphemeralAgentTask struct {
	// Entrypoint is the script or module to execute.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Entrypoint string `json:"entrypoint"`

	// Image is the container image to use. Defaults to the flokoa runtime image.
	// +optional
	Image string `json:"image,omitempty"`

	// Framework is the AI framework used by the agent code.
	// +optional
	// +kubebuilder:validation:Enum=pydantic-ai;langchain;crewai;custom
	Framework string `json:"framework,omitempty"`

	// Tools is a list of Tool CR names to inject into the agent environment.
	// +optional
	Tools []string `json:"tools,omitempty"`

	// Context is a list of AgentContext CR names to mount.
	// +optional
	Context []string `json:"context,omitempty"`

	// Env is a list of additional environment variables for the container.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Input is data passed to the agent. Supports expressions.
	// +optional
	Input string `json:"input,omitempty"`

	// Resources specifies compute resource requirements for the container.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// WaitForSignalSpec pauses the workflow until an external signal is received.
// Only supported when engine is "temporal".
type WaitForSignalSpec struct {
	// Name is the signal name to wait for.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Timeout is the maximum duration to wait for the signal.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

// SwitchCase defines a conditional branch in a switch task.
// Exactly one of Condition+Then or Default must be set.
type SwitchCase struct {
	// Condition is an expression evaluated against available task outputs.
	// +optional
	Condition string `json:"condition,omitempty"`

	// Then is the task name to run if the condition is true.
	// +optional
	Then string `json:"then,omitempty"`

	// Default is the fallback task name if no other condition matches.
	// +optional
	Default string `json:"default,omitempty"`
}

// LoopSpec enables iterative execution of a task.
// Only supported when engine is "temporal".
type LoopSpec struct {
	// Until is an expression that when true, stops the loop.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Until string `json:"until"`

	// MaxIterations is the maximum number of loop iterations.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Required
	MaxIterations int32 `json:"maxIterations"`
}

// WorkflowRetryStrategy defines retry behavior for tasks.
type WorkflowRetryStrategy struct {
	// Limit is the maximum number of retry attempts.
	// +kubebuilder:validation:Minimum=0
	Limit int32 `json:"limit"`

	// Backoff configures the backoff strategy between retries.
	// +optional
	Backoff *WorkflowBackoff `json:"backoff,omitempty"`
}

// WorkflowBackoff configures exponential backoff between retries.
type WorkflowBackoff struct {
	// Duration is the initial backoff duration (e.g., "30s", "5m").
	// +kubebuilder:validation:Required
	Duration string `json:"duration"`

	// Factor is the multiplier applied to the duration after each retry.
	// +optional
	Factor *int32 `json:"factor,omitempty"`
}

// AgentWorkflowStatus defines the observed state of AgentWorkflow.
type AgentWorkflowStatus struct {
	// Phase represents the current lifecycle phase of the workflow.
	// +optional
	Phase WorkflowPhase `json:"phase,omitempty"`

	// ArgoWorkflowName is the name of the generated Argo Workflow CR.
	// +optional
	ArgoWorkflowName string `json:"argoWorkflowName,omitempty"`

	// StartTime is when the workflow execution started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the workflow execution completed.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// TaskStatuses contains the status of individual tasks.
	// +optional
	TaskStatuses []WorkflowTaskStatus `json:"taskStatuses,omitempty"`

	// Conditions represent the latest available observations of the workflow's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// WorkflowTaskStatus contains the status of an individual task.
type WorkflowTaskStatus struct {
	// Name is the task name.
	Name string `json:"name"`

	// Phase is the current phase of this task.
	// +optional
	Phase WorkflowPhase `json:"phase,omitempty"`

	// StartTime is when the task started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the task completed.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Message is a human-readable message about the task status.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=awf
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Engine",type="string",JSONPath=".spec.engine"
// +kubebuilder:printcolumn:name="Argo Workflow",type="string",JSONPath=".status.argoWorkflowName"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AgentWorkflow is the Schema for the agentworkflows API.
// It defines a declarative, agent-native workflow for orchestrating AI agent tasks.
type AgentWorkflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentWorkflowSpec   `json:"spec,omitempty"`
	Status AgentWorkflowStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentWorkflowList contains a list of AgentWorkflow.
type AgentWorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentWorkflow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentWorkflow{}, &AgentWorkflowList{})
}
