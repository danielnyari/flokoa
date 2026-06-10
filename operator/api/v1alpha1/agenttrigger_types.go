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

// AgentTriggerSpec defines the desired state of an AgentTrigger.
type AgentTriggerSpec struct {
	// EventSource references the Argo Events EventSource and specific event
	// to consume from. The EventSource must exist in the same namespace.
	// +kubebuilder:validation:Required
	EventSource EventSourceRef `json:"eventSource"`

	// EventBus optionally references a non-default Argo Events EventBus.
	// If omitted, the Sensor uses the "default" EventBus in the namespace.
	// +optional
	EventBus *EventBusRef `json:"eventBus,omitempty"`

	// Filter defines which events from the EventSource should invoke the agent.
	// Uses Argo Events Sensor data filter semantics -- supports payload-level
	// filtering with JSONPath expressions, not just CloudEvent attribute matching.
	// If empty, all events from the named EventSource event are forwarded.
	// +optional
	Filter *TriggerFilter `json:"filter,omitempty"`

	// Agent references the flokoa Agent to invoke when a matching event arrives.
	// The controller resolves this to the Agent's A2A endpoint URL (status.url).
	// +kubebuilder:validation:Required
	Agent AgentRef `json:"agent"`

	// Task configures how the event is translated to an A2A task.
	// +optional
	Task *TriggerTaskConfig `json:"task,omitempty"`

	// PushNotification configures where the Agent should deliver results
	// via A2A push notifications. If omitted, results are fire-and-forget
	// (the agent still processes, but nobody receives the outcome).
	// +optional
	PushNotification *PushNotificationTarget `json:"pushNotification,omitempty"`

	// Limits defines rate and cost controls for this trigger.
	// +optional
	Limits *TriggerLimits `json:"limits,omitempty"`
}

// EventSourceRef references an Argo Events EventSource and a specific event within it.
type EventSourceRef struct {
	// Name of the Argo Events EventSource in the same namespace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// EventName is the specific event within the EventSource to consume.
	// This corresponds to the key in the EventSource spec (e.g., the webhook
	// endpoint name, the Kafka topic key, the SQS queue key).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	EventName string `json:"eventName"`
}

// EventBusRef references an Argo Events EventBus.
type EventBusRef struct {
	// Name of the Argo Events EventBus. Defaults to "default" if omitted
	// from the AgentTrigger spec entirely.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// TriggerFilter specifies which events should be delivered to the agent.
// Uses Argo Events Sensor data filter semantics, which are more expressive
// than CloudEvent attribute matching -- supports JSONPath-based filtering
// on the event payload itself.
type TriggerFilter struct {
	// Data specifies filters on the event payload using JSONPath expressions.
	// All filters must match for an event to pass (AND semantics).
	// Each filter can match against multiple acceptable values (OR within a filter).
	// +optional
	Data []DataFilter `json:"data,omitempty"`

	// Exprs specifies filters using expression evaluation (cel-go).
	// More flexible than data filters for complex boolean logic.
	// +optional
	Exprs []ExprFilter `json:"exprs,omitempty"`
}

// DataFilter defines a single payload-level filter using JSONPath.
type DataFilter struct {
	// Path is a JSONPath expression evaluated against the event data payload.
	// Examples: "body.type", "body.data.object.customer", "body.amount"
	// +kubebuilder:validation:Required
	Path string `json:"path"`

	// Type is the expected data type of the value at the path.
	// +kubebuilder:validation:Enum=string;float;bool
	Type string `json:"type"`

	// Value is a list of acceptable values. The filter passes if the value
	// at the path matches ANY of the listed values (OR semantics).
	// +kubebuilder:validation:MinItems=1
	Value []string `json:"value"`

	// Comparator is the comparison operator.
	// Defaults to "=" for string, supports ">", "<", ">=", "<=" for float.
	// +optional
	// +kubebuilder:validation:Enum="=";"!=";">";"<";">=";"<="
	Comparator string `json:"comparator,omitempty"`
}

// ExprFilter defines a filter using a CEL expression for complex logic.
type ExprFilter struct {
	// Expr is a CEL expression that must evaluate to true for the event to pass.
	// The event data is available as the variable "body".
	// Example: "body.amount > 1000 && body.currency == 'usd'"
	// +kubebuilder:validation:Required
	Expr string `json:"expr"`

	// Fields maps variable names to JSONPath expressions for use in the expression.
	// Example: {"amount": "body.data.object.amount", "currency": "body.data.object.currency"}
	// +optional
	Fields map[string]string `json:"fields,omitempty"`
}

// AgentRef references a flokoa Agent by name.
// The controller resolves this to the Agent's internal A2A endpoint (status.url).
type AgentRef struct {
	// Name of the flokoa Agent resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace of the Agent. Defaults to the AgentTrigger's namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// TriggerTaskConfig controls how events are translated to A2A tasks.
type TriggerTaskConfig struct {
	// SessionKeyFrom is a JSONPath expression evaluated against the event
	// data payload. The extracted value becomes the A2A contextId, enabling
	// conversation continuity across events from the same entity.
	//
	// Example: "$.customer_id" -- events from the same customer share a
	// conversation context, so the agent can reference prior interactions.
	//
	// If omitted or if the expression yields no value, each event starts
	// a new A2A context (no conversation continuity).
	// +optional
	SessionKeyFrom string `json:"sessionKeyFrom,omitempty"`

	// Metadata is a set of static key-value pairs attached to every A2A task
	// created by this trigger. Useful for routing, tagging, or tenant identification.
	// +optional
	Metadata map[string]string `json:"metadata,omitempty"`
}

// PushNotificationTarget defines where the Agent should deliver async results.
// Exactly one of AgentRef or URL must be specified.
// This maps directly to the A2A PushNotificationConfig attached to each SendMessage request.
//
// When AgentRef is used, the push notification URL points at flokoa-server's
// gateway endpoint. flokoa-server acts as a full agent gateway, proxying the
// notification to the target agent with logging, metrics, and trace propagation.
type PushNotificationTarget struct {
	// AgentRef routes push notifications to another flokoa Agent's A2A endpoint
	// via flokoa-server (gateway mode). Enables agent chaining with full
	// observability: Event -> Agent A (process) -> flokoa-server -> Agent B (act).
	// +optional
	AgentRef *AgentRef `json:"agentRef,omitempty"`

	// URL is an external HTTPS webhook that receives A2A push notification POSTs.
	// The payload is a standard A2A push notification containing a Task object.
	// Must be HTTPS. The controller does not validate URL reachability.
	// +optional
	// +kubebuilder:validation:Pattern=`^https://`
	URL string `json:"url,omitempty"`

	// Authentication configures how the Agent authenticates to the push
	// notification destination. Maps directly to A2A PushNotificationConfig.authentication.
	// +optional
	Authentication *TriggerPushAuth `json:"authentication,omitempty"`

	// TokenRef references a Secret containing an opaque token included in every
	// push notification for the receiver to validate. Maps to A2A
	// PushNotificationConfig.token.
	// +optional
	TokenRef *corev1.SecretKeySelector `json:"tokenRef,omitempty"`
}

// TriggerPushAuth specifies how the agent authenticates to the push notification
// destination. Uses Schemes (plural) to align with a2a.PushAuthInfo from
// github.com/a2aproject/a2a-go and the existing PushNotificationAuth type in
// agentworkflow_types.go. Uses SecretKeySelector for credentials instead of
// inline strings (unlike PushNotificationAuth in AgentWorkflow, which uses
// inline credentials for workflow-scoped, ephemeral push configs).
type TriggerPushAuth struct {
	// Schemes lists the authentication schemes supported (e.g., "Bearer", "Basic").
	// Maps to A2A AuthenticationInfo.schemes. Scheme names are case-insensitive
	// per RFC 9110 S11.1.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Schemes []string `json:"schemes"`

	// CredentialsRef references a Secret containing the credentials value.
	// The Secret key's value is used as the credentials string in the A2A
	// AuthenticationInfo.
	// +optional
	CredentialsRef *corev1.SecretKeySelector `json:"credentialsRef,omitempty"`
}

// TriggerLimits defines rate and cost controls for event-driven agent invocations.
// When a limit is exceeded, the event is routed to the dead letter sink (if configured)
// or dropped with a metric increment.
type TriggerLimits struct {
	// MaxInvocationsPerHour is the maximum number of A2A tasks this trigger
	// can create per rolling hour. Protects against noisy event sources.
	// 0 means unlimited.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MaxInvocationsPerHour *int32 `json:"maxInvocationsPerHour,omitempty"`

	// MaxConcurrentTasks is the maximum number of in-flight A2A tasks
	// (status: submitted or working) for this trigger at any time.
	// New events are queued or dropped when this limit is reached.
	// 0 means unlimited.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MaxConcurrentTasks *int32 `json:"maxConcurrentTasks,omitempty"`

	// TokenBudgetPerEvent is the maximum token count (input + output) the agent
	// may consume for a single event-triggered task. Injected as A2A task
	// metadata; the agent runtime SHOULD honor it when set. Optional --
	// if omitted, no per-event token limit is applied.
	// +optional
	// +kubebuilder:validation:Minimum=0
	TokenBudgetPerEvent *int32 `json:"tokenBudgetPerEvent,omitempty"`

	// TokenBudgetPerHour is the aggregate token budget across all invocations
	// from this trigger in a rolling hour. When exhausted, new events are
	// dropped until the window resets. 0 means unlimited.
	// +optional
	// +kubebuilder:validation:Minimum=0
	TokenBudgetPerHour *int64 `json:"tokenBudgetPerHour,omitempty"`

	// DeadLetterSink is a URL to receive events that were dropped due to
	// limit violations or agent errors. flokoa-server forwards the original
	// event payload with an error reason header.
	// +optional
	DeadLetterSink *DeadLetterSinkRef `json:"deadLetterSink,omitempty"`
}

// DeadLetterSinkRef references a destination for dropped events.
type DeadLetterSinkRef struct {
	// URI is an absolute URL to send dead-lettered events to.
	// flokoa-server POSTs the original event payload with additional headers:
	// X-Flokoa-Drop-Reason, X-Flokoa-Trigger-Name, X-Flokoa-Trigger-Namespace.
	// +kubebuilder:validation:Pattern=`^https?://`
	URI string `json:"uri"`
}

// AgentTriggerStatus defines the observed state of an AgentTrigger.
type AgentTriggerStatus struct {
	// Phase represents the current lifecycle phase.
	// +optional
	Phase AgentTriggerPhase `json:"phase,omitempty"`

	// SensorName is the name of the child Argo Events Sensor created
	// by the controller.
	// +optional
	SensorName string `json:"sensorName,omitempty"`

	// AgentEndpoint is the resolved A2A endpoint URL for the target agent.
	// +optional
	AgentEndpoint string `json:"agentEndpoint,omitempty"`

	// Invocations tracks invocation counters for observability.
	// +optional
	Invocations *InvocationCounters `json:"invocations,omitempty"`

	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:validation:Enum=Pending;Running;Failed
type AgentTriggerPhase string

const (
	AgentTriggerPhasePending AgentTriggerPhase = "Pending"
	AgentTriggerPhaseRunning AgentTriggerPhase = "Running"
	AgentTriggerPhaseFailed  AgentTriggerPhase = "Failed"
)

// InvocationCounters tracks trigger activity. Reset on controller restart.
// Authoritative metrics come from Prometheus, not these counters.
type InvocationCounters struct {
	// Total events received by flokoa-server for this trigger.
	Total int64 `json:"total,omitempty"`

	// Events that resulted in a successful A2A SendMessage invocation.
	Delivered int64 `json:"delivered,omitempty"`

	// Events dropped due to rate or cost limit violations.
	Dropped int64 `json:"dropped,omitempty"`

	// Events routed to the dead letter sink.
	DeadLettered int64 `json:"deadLettered,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=atr
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Agent",type="string",JSONPath=".spec.agent.name"
// +kubebuilder:printcolumn:name="EventSource",type="string",JSONPath=".spec.eventSource.name"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AgentTrigger is the Schema for the agenttriggers API.
// It bridges Argo Events to flokoa Agents, creating an A2A task
// for each matching event with optional push notification delivery.
type AgentTrigger struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentTriggerSpec   `json:"spec,omitempty"`
	Status AgentTriggerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentTriggerList contains a list of AgentTrigger.
type AgentTriggerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentTrigger `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentTrigger{}, &AgentTriggerList{})
}
