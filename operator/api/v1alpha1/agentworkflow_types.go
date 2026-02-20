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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentWorkflowSpec defines the desired state of AgentWorkflow.
type AgentWorkflowSpec struct {
	// Description is a human-readable description of the workflow.
	// +optional
	Description string `json:"description,omitempty"`

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

	// Description of what this parameter is used for.
	// +optional
	Description string `json:"description,omitempty"`

	// Value is the default parameter value. If empty, the parameter must be
	// provided at workflow submission time.
	// +optional
	Value string `json:"value,omitempty"`
}

// WorkflowTask defines a single task in the workflow.
// Exactly one of Agent, AgentTask, or Switch must be specified.
type WorkflowTask struct {
	// Name is the unique identifier for this task within the workflow.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9][-a-zA-Z0-9]*$`
	Name string `json:"name"`

	// Agent calls a deployed Agent CR via the A2A protocol.
	// +optional
	Agent *AgentCall `json:"agent,omitempty"`

	// AgentTask runs a Marvin-powered task in an ephemeral container.
	// +optional
	AgentTask *AgentTask `json:"agentTask,omitempty"`

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

	// Message is the A2A message to send to the agent.
	// +kubebuilder:validation:Required
	Message AgentMessage `json:"message"`

	// Config configures how the message is sent to the agent.
	// +optional
	Config *MessageSendConfig `json:"config,omitempty"`
}

// MessageRole represents the role of a message sender.
// Aligns with a2a.MessageRole from github.com/a2aproject/a2a-go.
// +kubebuilder:validation:Enum=user;agent
type MessageRole string

const (
	MessageRoleUser  MessageRole = "user"
	MessageRoleAgent MessageRole = "agent"
)

// AgentMessage represents an A2A protocol message.
// Aligns with a2a.Message from github.com/a2aproject/a2a-go.
type AgentMessage struct {
	// Role of the message sender.
	// +optional
	// +kubebuilder:default=user
	Role MessageRole `json:"role,omitempty"`

	// Parts is the content of the message. At least one part is required.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Parts []MessagePart `json:"parts"`

	// ContextID for conversation threading across agent interactions.
	// Aligns with a2a.Message.ContextID.
	// +optional
	ContextID string `json:"contextId,omitempty"`

	// ReferenceTaskIDs links this message to previous A2A task IDs.
	// Aligns with a2a.Message.ReferenceTasks.
	// +optional
	ReferenceTaskIDs []string `json:"referenceTaskIds,omitempty"`

	// Extensions lists A2A extension URIs to activate for this message.
	// Aligns with a2a.Message.Extensions.
	// +optional
	Extensions []string `json:"extensions,omitempty"`

	// TaskID continues an existing A2A task instead of creating a new one.
	// Aligns with a2a.Message.TaskID.
	// +optional
	TaskID string `json:"taskId,omitempty"`

	// Metadata is arbitrary key-value metadata attached to the message.
	// Aligns with a2a.Message.Metadata.
	// +optional
	Metadata map[string]apiextensionsv1.JSON `json:"metadata,omitempty"`
}

// MessagePart represents a content part in an A2A message.
// Exactly one of Text, Data, or File must be set.
// Aligns with a2a.Part (TextPart, DataPart, FilePart) from github.com/a2aproject/a2a-go.
type MessagePart struct {
	// Text content part.
	// +optional
	Text *TextPart `json:"text,omitempty"`

	// Data content part (structured JSON).
	// +optional
	Data *DataPart `json:"data,omitempty"`

	// File content part.
	// +optional
	File *FilePart `json:"file,omitempty"`
}

// TextPart contains text content. Supports DSL expressions.
// Aligns with a2a.TextPart from github.com/a2aproject/a2a-go.
type TextPart struct {
	// Text content. Supports expressions like {{params.topic}} or {{tasks.prev.output}}.
	// +kubebuilder:validation:Required
	Text string `json:"text"`

	// Metadata for this part.
	// +optional
	Metadata map[string]apiextensionsv1.JSON `json:"metadata,omitempty"`
}

// DataPart contains structured JSON data.
// Aligns with a2a.DataPart from github.com/a2aproject/a2a-go.
type DataPart struct {
	// Data is arbitrary JSON data to send to the agent.
	// +kubebuilder:validation:Required
	Data apiextensionsv1.JSON `json:"data"`

	// Metadata for this part.
	// +optional
	Metadata map[string]apiextensionsv1.JSON `json:"metadata,omitempty"`
}

// FilePart contains file content.
// Aligns with a2a.FilePart from github.com/a2aproject/a2a-go.
type FilePart struct {
	// File content (either inline bytes or a URI reference).
	// +kubebuilder:validation:Required
	File FileContent `json:"file"`

	// Metadata for this part.
	// +optional
	Metadata map[string]apiextensionsv1.JSON `json:"metadata,omitempty"`
}

// FileContent represents file data, either as inline base64 bytes or a URI reference.
// Aligns with a2a.FileBytes / a2a.FileURI / a2a.FileMeta from github.com/a2aproject/a2a-go.
type FileContent struct {
	// Name of the file.
	// +optional
	Name string `json:"name,omitempty"`

	// MimeType of the file content.
	// +optional
	MimeType string `json:"mimeType,omitempty"`

	// Bytes is the base64-encoded file content.
	// Exactly one of Bytes or URI must be set.
	// +optional
	Bytes string `json:"bytes,omitempty"`

	// URI is a reference to the file location.
	// Exactly one of Bytes or URI must be set.
	// +optional
	URI string `json:"uri,omitempty"`
}

// MessageSendConfig configures how a message is sent to an A2A agent.
// Aligns with a2a.MessageSendConfig from github.com/a2aproject/a2a-go.
type MessageSendConfig struct {
	// AcceptedOutputModes restricts the agent's output format.
	// +optional
	AcceptedOutputModes []string `json:"acceptedOutputModes,omitempty"`

	// Blocking indicates whether to wait synchronously for task completion.
	// +optional
	Blocking *bool `json:"blocking,omitempty"`

	// HistoryLength limits the conversation history returned with the response.
	// +optional
	HistoryLength *int32 `json:"historyLength,omitempty"`

	// PushNotificationConfig configures push notifications for async task updates.
	// Aligns with a2a.MessageSendConfig.PushNotificationConfig.
	// +optional
	PushNotificationConfig *PushNotificationConfig `json:"pushNotificationConfig,omitempty"`
}

// PushNotificationConfig configures push notifications for A2A task updates.
// Aligns with a2a.PushNotificationConfig from github.com/a2aproject/a2a-go.
type PushNotificationConfig struct {
	// URL is the webhook endpoint to receive push notifications.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`

	// ID is an optional identifier for this push notification configuration.
	// +optional
	ID string `json:"id,omitempty"`

	// Token is an opaque token the agent includes in push notifications for verification.
	// +optional
	Token string `json:"token,omitempty"`

	// Authentication configures authentication for push notification delivery.
	// +optional
	Authentication *PushNotificationAuth `json:"authentication,omitempty"`
}

// PushNotificationAuth configures authentication for push notification delivery.
// Aligns with a2a.PushNotificationAuthInfo from github.com/a2aproject/a2a-go.
type PushNotificationAuth struct {
	// Schemes lists the authentication schemes supported (e.g., "Bearer").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Schemes []string `json:"schemes"`

	// Credentials is the authentication credential (e.g., a bearer token).
	// +optional
	Credentials string `json:"credentials,omitempty"`
}

// MarvinTaskType represents the Marvin operation to perform.
// +kubebuilder:validation:Enum=run;classify;extract;cast;generate
type MarvinTaskType string

const (
	// MarvinTaskTypeRun executes a general-purpose task via marvin.run().
	MarvinTaskTypeRun MarvinTaskType = "run"
	// MarvinTaskTypeClassify classifies input into predefined labels via marvin.classify().
	MarvinTaskTypeClassify MarvinTaskType = "classify"
	// MarvinTaskTypeExtract extracts entities from input via marvin.extract().
	MarvinTaskTypeExtract MarvinTaskType = "extract"
	// MarvinTaskTypeCast transforms input into a target type via marvin.cast().
	MarvinTaskTypeCast MarvinTaskType = "cast"
	// MarvinTaskTypeGenerate generates structured objects via marvin.generate().
	MarvinTaskTypeGenerate MarvinTaskType = "generate"
)

// AgentTask defines a Marvin-powered task that runs in a short-lived container.
// The Type field determines which marvin.* function is called.
type AgentTask struct {
	// Type is the Marvin operation: run, classify, extract, cast, or generate.
	// +kubebuilder:validation:Required
	Type MarvinTaskType `json:"type"`

	// Instruction provides the prompt or guidance for the task.
	// - run: the main prompt (required)
	// - classify/extract/cast/generate: optional guidance instructions
	// Supports inline text (template) or reference to an Instruction CR (instructionRef).
	// +optional
	Instruction *InstructionEntry `json:"instruction,omitempty"`

	// Input is the data to process.
	// - classify: data to classify (required)
	// - extract: data to extract from (required)
	// - cast: data to transform (required)
	// - run/generate: not used
	// Supports expressions like {{tasks.prev.output}}.
	// +optional
	Input string `json:"input,omitempty"`

	// ResultType constrains the output via JSON Schema.
	// - run: optional (defaults to str)
	// - extract: target type (defaults to str)
	// - cast: target type (required)
	// - generate: target type (required)
	// - classify: not used (labels define the output)
	// +optional
	ResultType *StructuredIOSchema `json:"resultType,omitempty"`

	// Labels for classify operations.
	// Maps to marvin.classify(labels=[...]).
	// Required when type=classify.
	// +optional
	Labels []string `json:"labels,omitempty"`

	// MultiLabel enables multi-label classification (returns list instead of single label).
	// Maps to marvin.classify(multi_label=True).
	// Only used when type=classify.
	// +optional
	MultiLabel *bool `json:"multiLabel,omitempty"`

	// Count is the number of items to generate.
	// Maps to marvin.generate(n=...).
	// Only used when type=generate. Defaults to 1.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Count *int32 `json:"count,omitempty"`

	// Model references a Model CR for the LLM configuration.
	// +optional
	Model *AgentModelRef `json:"model,omitempty"`

	// Tools available to the task. Can be inline definitions or references to AgentTool CRs.
	// Primarily used with type=run.
	// +optional
	Tools []ToolEntry `json:"tools,omitempty"`

	// Context is key-value data passed to the Marvin task.
	// Values support expressions like {{tasks.prev.output}}.
	// +optional
	Context map[string]string `json:"context,omitempty"`

	// Agent defines an inline Marvin agent for execution.
	// Maps to the agent parameter in marvin.run(), marvin.classify(), etc.
	// +optional
	Agent *MarvinAgentSpec `json:"agent,omitempty"`

	// Image overrides the container image. Defaults to the flokoa managed-task runtime.
	// +optional
	Image string `json:"image,omitempty"`

	// Env is additional environment variables for the container.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Resources specifies compute resource requirements for the container.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// MarvinAgentSpec defines an inline Marvin agent configuration.
// Maps to marvin.Agent() constructor parameters.
type MarvinAgentSpec struct {
	// Name of the agent.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Instructions are behavioral instructions for this agent.
	// +optional
	Instructions string `json:"instructions,omitempty"`

	// Model references a Model CR for this agent's LLM.
	// If not specified, inherits the task-level model.
	// +optional
	Model *AgentModelRef `json:"model,omitempty"`
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
// The AgentWorkflow compiles to an Argo WorkflowTemplate; individual runs
// are Argo Workflow CRs created from the template and tracked separately.
type AgentWorkflowStatus struct {
	// Ready indicates the WorkflowTemplate has been successfully compiled and applied.
	Ready bool `json:"ready"`

	// WorkflowTemplateName is the name of the generated Argo WorkflowTemplate CR.
	// +optional
	WorkflowTemplateName string `json:"workflowTemplateName,omitempty"`

	// SpecHash is a hash of the AgentWorkflow spec, used for drift detection.
	// If the spec has not changed since last reconcile, recompilation is skipped.
	// +optional
	SpecHash string `json:"specHash,omitempty"`

	// Conditions represent the latest available observations of the workflow's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=awf
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Template",type="string",JSONPath=".status.workflowTemplateName"
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
