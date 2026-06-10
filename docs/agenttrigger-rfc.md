# RFC: AgentTrigger CRD — Event-Driven Agent Invocation

**Status**: Draft
**Author**: flokoa
**Date**: 2026-03-01
**API Group**: `agent.flokoa.ai/v1alpha1`
**Depends on**: Argo Events >= 1.9, A2A Protocol >= 1.0 (initial impl: a2a-go v0.3.6 JSON-RPC)

> **A2A Protocol Compatibility**: This RFC targets the A2A v1 HTTP+JSON binding. The initial implementation will use `github.com/a2aproject/a2a-go v0.3.6` (JSON-RPC transport). The a2a-go library's `SendMessage()` API will be used under the hood, with the v1 HTTP binding adopted when the library adds support. All v1 terminology in this document (e.g., `POST /message:send`, `application/a2a+json`, `ROLE_USER`) describes the target protocol semantics; the implementation will map these to the current a2a-go JSON-RPC equivalents. See `plugins/a2a/plugin/plugin.go` for the current a2a-go usage patterns.

## 1. Motivation

flokoa positions itself as a user-facing agent runtime, distinct from DevOps-oriented frameworks like kagent. User-facing agents are reactive to business events, not human CLI commands. A customer's payment fails, a support ticket arrives, a signup completes — the agent should respond automatically, asynchronously, and with full cost and observability controls.

Today, flokoa agents can only be invoked synchronously (request/response or SSE via the A2A gateway). There is no way to wire an agent to an external event source, control invocation costs, or trace the full path from event origin to agent completion.

The AgentTrigger CRD fills this gap by acting as a declarative bridge between Argo Events and the A2A protocol. It is not a workflow engine. It is not a custom event bus. It is a thin CRD that tells the operator: "when this event arrives, send it to this Agent as an A2A task, and push the result here."

### Design principles

1. **Argo Events native** — the operator creates real Argo Events Sensors. No custom event routing. The full Argo Events source ecosystem is available (Kafka, SQS, SNS, PubSub, GitHub, webhooks, cron, S3, AMQP, NATS, Redis Streams, and more).
2. **A2A protocol native** — agent invocation uses SendMessage (A2A v1 HTTP+JSON binding: `POST /message:send`), results come back via A2A push notifications. No proprietary transport.
3. **Lean controller** — the controller translates CRDs to Argo Events resources and syncs status. The data plane (event -> A2A translation) runs in flokoa-server. Business logic is delegated to an app service layer following the Agent controller's layered architecture.
4. **Boring infrastructure** — builds on Argo Events, CloudEvents, and A2A. No new protocols.

### What this enables that kagent cannot do

* Event-driven agent invocation from any Argo Events source (Kafka, SQS, SNS, PubSub, webhooks, cron, S3, GitHub, and 20+ others)
* A2A push notification delivery for async/long-running results
* Per-trigger cost and rate controls to prevent LLM budget blowout
* Session continuity across events from the same entity (customer, ticket, user)
* End-to-end distributed traces from event source through agent execution
* Payload-level event filtering (not just CloudEvent attribute matching)

## 2. Architecture Overview

```
                    +----------------------------------+
                    |     External Event Sources        |
                    |  Kafka . SQS . SNS . PubSub      |
                    |  Webhook . GitHub . Cron . S3     |
                    +--------------+-------------------+
                                   |
                                   v
                    +----------------------------------+
                    |   Argo Events EventSource         |
                    |   (user-managed)                  |
                    |   Publishes to EventBus           |
                    +--------------+-------------------+
                                   | events
                                   v
                    +----------------------------------+
                    |       Argo Events EventBus        |
                    |   (NATS JetStream / Kafka)        |
                    |   (user-managed, one per ns)      |
                    +--------------+-------------------+
                                   |
                                   v
                    +----------------------------------+
                    |   Argo Events Sensor (child)      |
                    |   Created by AgentTrigger ctrl    |
                    |   dependency -> EventSource       |
                    |   data filters from spec.filter   |
                    |   HTTP trigger -> flokoa-server   |
                    +--------------+-------------------+
                                   | HTTP POST
                                   v
+--------------------------------------------------------------+
|                      flokoa-server                            |
|                                                               |
|  1. Receives event payload from Sensor HTTP trigger           |
|  2. Looks up AgentTrigger config (agent ref, push config,     |
|     session key, limits)                                      |
|  3. Enforces rate/cost limits                                 |
|  4. Extracts session key from payload (JSONPath)              |
|  5. Constructs A2A SendMessage request with:                  |
|     - Event payload as Part (data content + media_type)       |
|     - pushNotificationConfig from trigger spec                |
|     - contextId from session key (conversation continuity)    |
|  6. Injects W3C traceparent into A2A request                  |
|  7. Sends to Agent's A2A endpoint via gateway                 |
|  8. Records invocation metrics                                |
+----------------------+---------------------------------------+
                       | A2A SendMessage
                       v
          +----------------------------+
          |    Agent Pod (A2A)         |
          |    billing-agent           |
          |    +--------------------+  |
          |    | Process task       |  |
          |    | (may take mins)    |  |
          |    +--------+-----------+  |
          +-------------+--------------+
                        | A2A push notification
                        v
          +----------------------------+
          |   Push Notification        |
          |   Destination              |
          |                            |
          |   . Another Agent (A2A)    |
          |     (via flokoa-server     |
          |      gateway)              |
          |   . External webhook URL   |
          +----------------------------+
```

### Component responsibilities

| Component | Role |
|---|---|
| **AgentTrigger controller** | Watches AgentTrigger CRDs. Creates/updates/deletes child Argo Events Sensors. Resolves agent references. Syncs status from Sensor + Agent readiness. Delegates business logic to `triggerapp.Service` following the layered architecture pattern. |
| **flokoa-server** | Data plane and agent gateway. Receives events from Sensor HTTP triggers. Translates to A2A SendMessage. Enforces rate/cost limits. Injects trace context. Records metrics. Proxies push notifications for agentRef targets (full gateway role). |
| **Argo Events EventBus** | User-provisioned. NATS JetStream, Kafka, or Redis Streams backed. One per namespace (typically). Not managed by flokoa. |
| **Argo Events EventSource** | User-provisioned. Webhook, Kafka, SQS, SNS, PubSub, S3, cron, GitHub, etc. Publishes to the EventBus. Not managed by flokoa. |
| **Agent Pod** | Existing flokoa Agent. Must have `capabilities.pushNotifications: true` in its card to support async triggers. |

## 3. CRD Specification

### 3.1 AgentTriggerSpec

```go
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
	Task *TaskConfig `json:"task,omitempty"`

	// PushNotification configures where the Agent should deliver results
	// via A2A push notifications. If omitted, results are fire-and-forget
	// (the agent still processes, but nobody receives the outcome).
	// +optional
	PushNotification *PushNotificationTarget `json:"pushNotification,omitempty"`

	// Limits defines rate and cost controls for this trigger.
	// +optional
	Limits *TriggerLimits `json:"limits,omitempty"`
}
```

### 3.2 EventSourceRef

```go
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
```

### 3.3 EventBusRef

```go
// EventBusRef references an Argo Events EventBus.
type EventBusRef struct {
	// Name of the Argo Events EventBus. Defaults to "default" if omitted
	// from the AgentTrigger spec entirely.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}
```

### 3.4 TriggerFilter

```go
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
```

### 3.5 AgentRef

```go
// AgentRef references a flokoa Agent by name.
// The controller resolves this to the Agent's internal A2A endpoint (status.url).
// Follows the same pattern as AgentModelRef, ToolRef, and ProviderRef.
type AgentRef struct {
	// Name of the flokoa Agent resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace of the Agent. Defaults to the AgentTrigger's namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}
```

### 3.6 TaskConfig

```go
// TaskConfig controls how events are translated to A2A tasks.
type TaskConfig struct {
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
```

### 3.7 PushNotificationTarget

```go
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
	// Uses Schemes (plural) to align with the A2A protocol's PushAuthInfo and
	// the existing PushNotificationAuth type in agentworkflow_types.go.
	// Credentials are referenced via SecretKeySelector (not inline) because
	// operator-managed CRDs should not store secrets directly.
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
```

#### Push notification security

flokoa-server acts as the A2A client that configures push notifications on behalf of the trigger. When the agent completes (or reaches INPUT_REQUIRED, AUTH_REQUIRED, or any terminal state), it POSTs a standard A2A push notification to the configured webhook URL.

When push target is an agentRef, the notification URL points at flokoa-server's gateway endpoint (`/api/v1alpha1/namespaces/{namespace}/agents/{target-agent-name}/push`). flokoa-server receives the notification, logs it, records metrics, propagates trace context, and forwards it to the target agent's A2A endpoint. This gives full observability on the result path without the sending agent needing to know the target agent's internal URL.

Because flokoa-server constructs the PushNotificationConfig (included inline in the MessageSendConfig) and the agent pod makes the outbound POST, both sides have security responsibilities:

**SSRF prevention** (agent pod -> push URL). The agent will POST to whatever URL the trigger specifies. Malicious or misconfigured triggers could point at internal cluster services (SSRF) or use the agent as a DDoS amplifier against external targets. Mitigations:

* The admission webhook SHOULD reject push URLs pointing at cluster-internal addresses (`*.svc.cluster.local`, RFC 1918 ranges) unless explicitly allowed.
* NetworkPolicy on agent pods SHOULD restrict egress to known external destinations.
* Future: challenge-response ownership verification for webhook URLs (out of scope for v1).

**Replay prevention** (push destination -> verify notification). Push notification receivers should guard against replayed notifications:

* The A2A spec recommends notifications include timestamps; receivers SHOULD reject notifications older than 5 minutes.
* For critical flows, receivers should use unique notification IDs (e.g., JWT jti claim) to deduplicate.
* flokoa-server injects a unique event ID from the originating event into the A2A task metadata, which can serve as a correlation ID for deduplication.

**Asymmetric key authentication**. For production deployments, the recommended pattern is JWT + JWKS: the agent signs push notifications with a private key, the receiver verifies against the agent's published JWKS endpoint. The `TriggerPushAuth.Schemes` field supports `["Bearer"]` for this flow. Simpler deployments can use shared secrets via `credentialsRef`.

### 3.8 TriggerLimits

```go
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
```

### 3.9 AgentTriggerStatus

```go
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
```

### 3.10 Condition types

| Type | Meaning |
|---|---|
| `Ready` | All sub-conditions are true; the trigger is fully operational. |
| `EventSourceReady` | The referenced Argo Events EventSource exists and the named event is defined. |
| `EventBusReady` | The EventBus (default or named) exists and is healthy. |
| `AgentReady` | The referenced Agent exists, is Running, and supports pushNotifications (if push is configured). |
| `SensorReady` | The child Argo Events Sensor was created and reports Ready. |

Condition types and reasons are defined in `internal/domain/trigger/status.go` following the Agent controller pattern (`internal/domain/agent/status.go`). A degraded state (e.g., Agent temporarily not Ready but Sensor healthy) is expressed as `Ready=False` with an appropriate reason, not as a separate phase.

### 3.11 Full CRD registration

```go
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
```

## 4. Controller Reconciliation

### 4.1 Architecture

The AgentTrigger controller follows the layered architecture established by the Agent controller:

```
internal/controller/agenttrigger_controller.go  -- Controller shell (fetch, finalizer, status, error classification)
        |
        v
internal/app/trigger/reconcile.go               -- App service (Deps struct, orchestration)
        |
        v
internal/domain/trigger/                        -- Domain logic (validate.go, status.go, labels.go)
internal/infra/repo/                            -- Infrastructure (repo interfaces, builders)
```

The controller shell delegates all business logic to `triggerapp.Service`, which uses a `Deps` struct with repository interfaces for testability:

```go
type Deps struct {
	Agents       repo.AgentReader
	Sensors      repo.SensorWriter     // Argo Events Sensor CRUD
	ConfigMaps   repo.ConfigMapRepo
	Secrets      repo.SecretReader
	EventSources repo.EventSourceReader // Argo Events EventSource reads
}
```

**Error classification** follows `internal/errors/`:
- `flokoaerrors.NewPermanent(err)` -- invalid spec, validation failures (do not requeue)
- `flokoaerrors.NewDependency(err)` -- referenced Agent/EventSource not found or not ready (requeue after 30s)
- Default -- transient errors (controller-runtime exponential backoff)

**Finalizer**: `agent.flokoa.ai/agenttrigger-finalizer`

**Status updates** use `updateStatusWithRetry()` from `internal/controller/status.go` to handle concurrent update conflicts.

### 4.2 Reconcile loop

```
AgentTrigger created/updated
         |
         v
    +--- Resolve Agent ---+
    |  GET Agent CR        |
    |  Validate phase=Running |
    |  If pushNotification set: |
    |    validate capabilities.pushNotifications=true |
    |  Extract A2A endpoint URL (status.url) |
    +---------+-----------+
              |
              v
    +--- Resolve EventSource --+
    |  GET Argo EventSource CR  |
    |  Validate named event     |
    |  exists in spec           |
    +---------+----------------+
              |
              v
    +--- Resolve EventBus ----+
    |  GET EventBus (named or  |
    |  "default" in namespace) |
    |  Validate healthy        |
    +---------+---------------+
              |
              v
    +--- Resolve Push Target -+
    |  If agentRef: resolve    |
    |    target agent endpoint |
    |    construct gateway URL  |
    |    /api/v1alpha1/namespaces/ |
    |    {ns}/agents/{agent}/push |
    |  If url: validate HTTPS  |
    |  Read Secret refs for    |
    |    auth credentials/token|
    +---------+---------------+
              |
              v
    +--- Create/Update Argo Sensor --------------------------+
    |  apiVersion: argoproj.io/v1alpha1                      |
    |  kind: Sensor                                          |
    |  metadata:                                             |
    |    name: at-{agenttrigger.name}                        |
    |    ownerReferences: [AgentTrigger]                     |
    |    labels:                                             |
    |      flokoa.ai/trigger: {name}                         |
    |  spec:                                                 |
    |    eventBusName: {spec.eventBus.name || default}       |
    |    dependencies:                                       |
    |      - name: {eventSource.eventName}                   |
    |        eventSourceName: {eventSource.name}             |
    |        eventName: {eventSource.eventName}              |
    |        filters:                                        |
    |          data: {spec.filter.data}                      |
    |          exprs: {spec.filter.exprs}                    |
    |    triggers:                                           |
    |      - template:                                       |
    |          name: invoke-agent                            |
    |          http:                                         |
    |            url: http://flokoa-server.{system-ns}       |
    |              .svc.cluster.local/api/v1alpha1/           |
    |              namespaces/{namespace}/                    |
    |              agenttriggers/{name}/invoke                |
    |            method: POST                                |
    |            headers:                                    |
    |              Content-Type: application/json            |
    |            payload:                                    |
    |              - src:                                    |
    |                  dependencyName: {eventName}           |
    |                  dataKey: body                         |
    |                dest: data                              |
    |              - src:                                    |
    |                  dependencyName: {eventName}           |
    |                  contextKey: id                        |
    |                dest: eventId                           |
    |              - src:                                    |
    |                  dependencyName: {eventName}           |
    |                  contextKey: type                      |
    |                dest: eventType                         |
    |              - src:                                    |
    |                  dependencyName: {eventName}           |
    |                  contextKey: source                    |
    |                dest: eventSource                       |
    |              - src:                                    |
    |                  dependencyName: {eventName}           |
    |                  contextKey: time                      |
    |                dest: eventTime                         |
    |          retryStrategy:                                |
    |            steps: 3                                    |
    |            duration: 5s                                |
    |            factor: 2                                   |
    |            jitter: 0.2                                 |
    +---------+----------------------------------------------+
              |
              v
    +--- Store trigger config --+
    |  ConfigMap:                |
    |  agenttrigger-{name}-config|
    |  - agent endpoint URL      |
    |  - push notification config|
    |  - session key JSONPath    |
    |  - limits                  |
    |  - task metadata           |
    +---------+----------------+
              |
              v
    +--- Update Status ----+
    |  Set conditions       |
    |  Set phase            |
    |  Set agentEndpoint    |
    |  Set sensorName       |
    +----------------------+
```

### 4.3 Child resource: Argo Events Sensor

The controller creates exactly one Argo Events Sensor per AgentTrigger. The Sensor is owned by the AgentTrigger (garbage collected on delete). The Sensor's HTTP trigger always targets flokoa-server, with a path that encodes the trigger identity:

```
POST /api/v1alpha1/namespaces/{namespace}/agenttriggers/{agenttrigger-name}/invoke
```

The Sensor's payload template extracts the event body and context attributes (id, type, source, time) and maps them into a structured JSON payload. This lets flokoa-server look up the full trigger configuration (agent endpoint, push config, limits, session key) when it receives the event.

Unlike Knative's Broker-based delivery (which handles retries at the infrastructure level), retry behavior in Argo Events is per-Sensor. The controller configures a default retry strategy (3 retries, exponential backoff) on every Sensor it creates. This ensures transient flokoa-server failures don't drop events.

### 4.4 Trigger configuration delivery

The controller writes the resolved trigger configuration as a ConfigMap in the same namespace, named `agenttrigger-{name}-config`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: agenttrigger-payment-failed-handler-config
  ownerReferences:
    - apiVersion: agent.flokoa.ai/v1alpha1
      kind: AgentTrigger
      name: payment-failed-handler
data:
  config.json: |
    {
      "agentEndpoint": "http://billing-agent.default.svc.cluster.local:8080",
      "sessionKeyFrom": "$.data.object.customer",
      "pushNotificationConfig": {
        "url": "http://flokoa-server.flokoa-system.svc.cluster.local/api/v1alpha1/namespaces/default/agents/escalation-agent/push",
        "authentication": {
          "schemes": ["Bearer"],
          "credentials": "<resolved from Secret>"
        },
        "token": "<resolved from Secret>"
      },
      "limits": {
        "maxInvocationsPerHour": 100,
        "maxConcurrentTasks": 10,
        "tokenBudgetPerEvent": 5000,
        "tokenBudgetPerHour": 500000
      },
      "metadata": {
        "trigger": "payment-failed-handler",
        "namespace": "default"
      }
    }
```

flokoa-server watches these ConfigMaps (or loads them on-demand from the path-based routing) to handle incoming events.

### 4.5 Watches and re-reconciliation

The controller watches cross-resource dependencies and re-reconciles as needed:

```go
func (r *AgentTriggerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.AgentTrigger{}).
		Watches(&agentv1alpha1.Agent{},
			handler.EnqueueRequestsFromMapFunc(r.findTriggersForAgent)).
		Named("agenttrigger").
		Complete(r)
}
```

The controller re-reconciles an AgentTrigger when:

* The AgentTrigger itself changes
* The referenced Agent changes (status, endpoint, capabilities)
* The referenced Argo Events EventSource changes (event definitions)
* The EventBus changes (health)
* The child Argo Events Sensor changes (readiness, deletion)
* Referenced Secrets change (auth credentials rotation)

## 5. Data Plane: flokoa-server Event Handler

> **Implementation Note**: The processing pipeline below describes A2A v1 HTTP+JSON binding semantics. The initial implementation uses `a2a-go v0.3.6` with JSON-RPC transport. In practice:
> - `POST {endpoint}/message:send` is implemented as `a2aclient.SendMessage(ctx, params)`
> - `SendMessageRequest`/`SendMessageResponse` map to `a2a.MessageSendParams`/`a2a.SendMessageResult`
> - `ROLE_USER` maps to `a2a.MessageRoleUser` (string value `"user"`)
> - Push notification config is sent inline via `MessageSendConfig.PushConfig`, not via a separate `CreateTaskPushNotificationConfig` call
> - Agent endpoints are tried with both `{url}` and `{url}/a2a` suffixes for compatibility
>
> The v1 terminology is retained for protocol clarity. See `plugins/a2a/plugin/plugin.go` for the current a2a-go usage patterns.

### 5.1 Endpoint

flokoa-server exposes:

```
POST /api/v1alpha1/namespaces/{namespace}/agenttriggers/{name}/invoke
Content-Type: application/json
```

The payload is a JSON object constructed by the Sensor's payload template, containing:

```json
{
  "data": { /* original event body */ },
  "eventId": "abc-123",
  "eventType": "com.stripe.payment_intent.payment_failed",
  "eventSource": "stripe-webhooks",
  "eventTime": "2026-03-01T10:00:00Z"
}
```

### 5.2 Processing pipeline

```
1. Parse event payload from Sensor HTTP trigger
2. Extract trigger identity from URL path
3. Load trigger config (ConfigMap or in-memory cache)
4. -- RATE CHECK --
   a. Check maxInvocationsPerHour (sliding window counter)
   b. Check maxConcurrentTasks (in-flight gauge)
   c. Check tokenBudgetPerHour (rolling sum)
   d. If any limit exceeded:
      - If deadLetterSink configured: forward event there
      - Increment dropped/deadLettered counter
      - Return 429 Too Many Requests
5. -- SESSION RESOLUTION --
   a. If sessionKeyFrom is set:
      - Evaluate JSONPath against event data
      - Hash the extracted value to a deterministic contextId
      - Look up existing A2A context in session store
   b. Else: generate new contextId (UUID)
6. -- A2A MESSAGE CONSTRUCTION --
   Build A2A SendMessageRequest (HTTP+JSON binding):

    POST {agentEndpoint}/message:send HTTP/1.1
    Content-Type: application/a2a+json
    A2A-Version: 1.0

    {
     "message": {
       "role": "ROLE_USER",
       "messageId": "<uuid>",
       "contextId": "<resolved-context-id or omit for new>",
       "taskId": "<previous-task-id or omit for new>",
       "parts": [
         {
           "data": <event.data>,
           "mediaType": "application/json",
           "metadata": {
             "event-type": "<eventType>",
             "event-source": "<eventSource>",
             "event-id": "<eventId>",
             "event-time": "<eventTime>"
           }
         }
       ]
     },
     "configuration": {
       "acceptedOutputModes": ["application/json", "text/plain"],
       "pushNotificationConfig": <from trigger config>,
       "blocking": false
     },
     "metadata": <trigger.task.metadata merged with trigger identity>
    }

    a. If contextId resolved from session: set message.contextId
    b. If previous taskId exists for this context: set message.taskId
    c. If tokenBudgetPerEvent set: include in request metadata as
       "flokoa.token_budget" for the agent to honor

    Response is a SendMessageResponse containing either:
    - A Task object (agent accepted async work) -> record task.id
    - A Message object (agent responded directly) -> no task tracking needed
7. -- TRACE CONTEXT --
   a. Create span: "agenttrigger.invoke {trigger-name}"
   b. Inject traceparent into A2A HTTP request headers
   c. Set span attributes:
      - flokoa.trigger.name
      - flokoa.trigger.namespace
      - flokoa.agent.name
      - flokoa.event.type
      - flokoa.event.source
      - flokoa.event.id
      - flokoa.session.context_id
8. -- SEND TO AGENT --
   POST {agentEndpoint}/message:send with A2A SendMessageRequest
   Content-Type: application/a2a+json
   a. On 2xx with Task in response: agent accepted the task
      - Record task.id from response (server-generated)
      - Push notification config was included inline via
        configuration.pushNotificationConfig -- agent handles setup
      - If sessionKeyFrom: persist contextId <-> task.id mapping
      - Increment delivered counter
      - Return 200 to Sensor (event consumed)
   b. On 2xx with Message in response: agent responded directly
      - No task tracking needed (trivial interaction)
      - Increment delivered counter
      - Return 200 to Sensor (event consumed)
   c. On 4xx: agent rejected (bad payload, unsupported)
      - Route to dead letter sink if configured
      - Increment dropped counter
      - Return 200 (don't retry -- the event is invalid)
   d. On 5xx or timeout: agent unavailable
      - Return 500 to Sensor (triggers retry per retryStrategy)
   NOTE: Push notification receivers (webhooks, chained agents)
   SHOULD call GetTask with the task.id from the notification to
   retrieve the full Task object including artifacts, rather than
   relying solely on the push notification payload. The push payload
   is a lightweight signal; GetTask is the source of truth.
9. -- METRICS --
   Emit Prometheus metrics per trigger (see S6.3)
```

### 5.3 Event to A2A mapping

The event payload is sent as an A2A DataPart with its data field set to the event body, preserving the original structure. Event metadata is included as metadata on the Part. There is no template engine or payload transformation in the trigger -- the agent receives structured data and extracts what it needs. The agent is the smart part.

| Event field | A2A location |
|---|---|
| `data` (event body) | `parts[0].data` (DataPart with data content) |
| `eventType` | `parts[0].metadata.event-type` |
| `eventSource` | `parts[0].metadata.event-source` |
| `eventId` | `parts[0].metadata.event-id` |
| `eventTime` | `parts[0].metadata.event-time` |
| `mediaType` | `parts[0].mediaType = "application/json"` |
| trace context | HTTP header on A2A request (not in payload) |

### 5.4 Push notification gateway

When an AgentTrigger's `pushNotification.agentRef` targets another flokoa Agent, flokoa-server acts as a push notification gateway. The endpoint:

```
POST /api/v1alpha1/namespaces/{namespace}/agents/{agent-name}/push
```

receives push notifications from the sending agent and forwards them to the target agent's A2A endpoint. This gateway path provides:

* Logging of all push notification deliveries between agents
* Prometheus metrics on the result path (`flokoa_push_forwarded_total`, `flokoa_push_forward_duration_seconds`)
* Trace context propagation from the push notification to the target agent
* Decoupling of agent-to-agent networking (agents don't need to know each other's internal URLs)

## 6. Observability Stitching

### 6.1 Trace propagation

The goal is a single distributed trace spanning: Event Source -> EventBus -> Sensor -> flokoa-server -> Agent -> Push Destination.

Trace context utilities from `internal/telemetry/telemetry.go` provide `Tracer()`, `ExtractTraceparent()`, and `ContextFromTraceparent()`. Traceparent is propagated via both HTTP headers (OTel-instrumented transport) and A2A message metadata (fallback for non-HTTP paths).

```
[EventSource]                    [Sensor]
  span: eventsource.receive        span: sensor.trigger
  traceparent: abc-123-...    -->  traceparent: abc-123-...
                                        |
                                        v
                              [flokoa-server]
                                span: agenttrigger.invoke
                                parent: abc-123-...
                                attributes:
                                  flokoa.trigger.name
                                  flokoa.agent.name
                                  flokoa.event.type
                                  flokoa.event.id
                                        |
                                        v
                              [Agent Pod]
                                span: agent.process
                                parent: (from traceparent header)
                                        |
                                        v
                              [flokoa-server gateway]
                                span: push.forward
                                parent: (from agent's outbound headers)
                                        |
                                        v
                              [Target Agent Pod]
                                span: agent.process
                                parent: (from gateway headers)
```

Argo Events propagates trace context through the EventBus. When the Sensor fires its HTTP trigger, the outbound request includes trace headers if the Sensor pod has OTel instrumentation configured. flokoa-server extracts the traceparent (if present), creates a child span, and forwards it as an HTTP header on the A2A request. The agent SDK (pydantic-ai, ADK, etc.) picks it up via standard OTel HTTP instrumentation.

If the Sensor doesn't propagate trace context (depends on EventBus backend), flokoa-server starts a new trace rooted at the `agenttrigger.invoke` span. The event-id is always available in span attributes for correlation.

### 6.2 Span naming convention

| Component | Span name | Key attributes |
|---|---|---|
| flokoa-server | `agenttrigger.invoke {name}` | `flokoa.trigger.name`, `flokoa.agent.name`, `flokoa.event.type`, `flokoa.event.source`, `flokoa.event.id`, `flokoa.session.context_id` |
| flokoa-server (on limit drop) | `agenttrigger.drop {name}` | `flokoa.trigger.name`, `flokoa.drop.reason` (rate_limit, cost_limit, concurrent_limit) |
| flokoa-server (dead letter) | `agenttrigger.deadletter {name}` | `flokoa.trigger.name`, `flokoa.deadletter.reason` |
| flokoa-server (push gateway) | `push.forward {target-agent}` | `flokoa.push.source_agent`, `flokoa.push.target_agent`, `flokoa.push.task_id` |

### 6.3 Prometheus metrics

All metrics are labeled with `trigger_name`, `trigger_namespace`, `agent_name`.

| Metric | Type | Description |
|---|---|---|
| `flokoa_trigger_events_total` | Counter | Total events received, by status label: `delivered`, `dropped`, `dead_lettered`, `error` |
| `flokoa_trigger_invocation_duration_seconds` | Histogram | Time from event receipt to A2A response (not agent completion) |
| `flokoa_trigger_active_tasks` | Gauge | Currently in-flight A2A tasks for this trigger |
| `flokoa_trigger_tokens_consumed_total` | Counter | Aggregate tokens consumed (reported by agent via push notification or task status) |
| `flokoa_trigger_rate_limit_hits_total` | Counter | Events dropped due to rate limits, by `limit_type`: `invocations_per_hour`, `concurrent_tasks`, `token_budget_per_event`, `token_budget_per_hour` |
| `flokoa_push_forwarded_total` | Counter | Push notifications forwarded via gateway, by `source_agent`, `target_agent` |
| `flokoa_push_forward_duration_seconds` | Histogram | Time to forward push notification to target agent |

### 6.4 W3C Baggage propagation

flokoa-server injects baggage headers with routing metadata for downstream observability backends:

```
baggage: flokoa.trigger=payment-failed-handler,flokoa.agent=billing-agent,flokoa.namespace=default
```

This enables ObservabilityPipeline CRDs (future) to route traces to different backends based on agent or trigger identity.

## 7. Examples

### 7.1 Stripe payment failure -> billing agent -> webhook callback

```yaml
# 1. EventBus (user-managed, one per namespace)
apiVersion: argoproj.io/v1alpha1
kind: EventBus
metadata:
  name: default
spec:
  jetstream:
    version: "2.10.10"
---
# 2. EventSource -- receives Stripe webhooks
apiVersion: argoproj.io/v1alpha1
kind: EventSource
metadata:
  name: stripe-webhooks
spec:
  webhook:
    payment-failed:
      port: "12000"
      endpoint: /stripe/payment-failed
      method: POST
---
# 3. AgentTrigger (flokoa-managed)
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTrigger
metadata:
  name: payment-failed-handler
spec:
  eventSource:
    name: stripe-webhooks
    eventName: payment-failed
  filter:
    data:
      - path: body.type
        type: string
        value:
          - "payment_intent.payment_failed"
  agent:
    name: billing-agent
  task:
    sessionKeyFrom: "$.data.object.customer"
    metadata:
      source: stripe
      priority: high
  pushNotification:
    url: "https://api.myapp.com/agent-results"
    authentication:
      schemes:
        - "Bearer"
      credentialsRef:
        name: callback-auth-secret
        key: token
  limits:
    maxInvocationsPerHour: 200
    maxConcurrentTasks: 20
    tokenBudgetPerEvent: 8000
    deadLetterSink:
      uri: "https://api.myapp.com/dead-letters"
```

### 7.2 SQS message queue -> triage agent -> escalation agent (agent chaining)

```yaml
# EventSource -- consumes from SQS
apiVersion: argoproj.io/v1alpha1
kind: EventSource
metadata:
  name: support-tickets
spec:
  sqs:
    ticket-queue:
      queue: "https://sqs.eu-west-1.amazonaws.com/123456789/support-tickets"
      region: "eu-west-1"
      waitTimeSeconds: 20
      accessKey:
        name: aws-credentials
        key: access-key
      secretKey:
        name: aws-credentials
        key: secret-key
---
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTrigger
metadata:
  name: ticket-triage
spec:
  eventSource:
    name: support-tickets
    eventName: ticket-queue
  agent:
    name: triage-agent
  task:
    sessionKeyFrom: "$.ticket_id"
  pushNotification:
    # Push results to escalation-agent via flokoa-server gateway.
    # flokoa-server receives the notification at
    # /api/v1alpha1/namespaces/default/agents/escalation-agent/push,
    # logs it, records metrics, and forwards to the escalation agent's
    # A2A endpoint.
    agentRef:
      name: escalation-agent
  limits:
    maxInvocationsPerHour: 500
    tokenBudgetPerHour: 1000000
```

### 7.3 Cron -> daily summary agent (fire-and-forget)

```yaml
# EventSource -- calendar-based cron
apiVersion: argoproj.io/v1alpha1
kind: EventSource
metadata:
  name: scheduled
spec:
  calendar:
    daily-morning:
      schedule: "0 8 * * *"
      timezone: "America/New_York"
      metadata:
        type: daily_summary
        scope: overnight
---
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTrigger
metadata:
  name: daily-summary
spec:
  eventSource:
    name: scheduled
    eventName: daily-morning
  agent:
    name: summary-agent
  # No pushNotification -- agent handles delivery internally
  # (e.g., via a Slack tool or email tool)
  limits:
    maxInvocationsPerHour: 2
    tokenBudgetPerEvent: 50000
```

### 7.4 Kafka topic -> fraud detection agent

```yaml
# EventSource -- Kafka consumer
apiVersion: argoproj.io/v1alpha1
kind: EventSource
metadata:
  name: transaction-stream
spec:
  kafka:
    high-value-transactions:
      url: "kafka-broker.kafka.svc.cluster.local:9092"
      topic: "transactions"
      consumerGroup:
        groupName: "flokoa-fraud-detection"
      tls:
        caCertSecret:
          name: kafka-tls
          key: ca.crt
---
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTrigger
metadata:
  name: fraud-detector
spec:
  eventSource:
    name: transaction-stream
    eventName: high-value-transactions
  filter:
    data:
      - path: body.amount
        type: float
        value: ["10000"]
        comparator: ">="
    exprs:
      - expr: "currency == 'usd' || currency == 'eur'"
        fields:
          currency: body.currency
  agent:
    name: fraud-agent
  task:
    sessionKeyFrom: "$.account_id"
    metadata:
      source: kafka
      pipeline: fraud-detection
  pushNotification:
    url: "https://api.myapp.com/fraud-alerts"
    authentication:
      schemes:
        - "Bearer"
      credentialsRef:
        name: fraud-api-secret
        key: api-key
  limits:
    maxInvocationsPerHour: 1000
    maxConcurrentTasks: 50
    tokenBudgetPerHour: 2000000
    deadLetterSink:
      uri: "https://api.myapp.com/dead-letters"
```

### 7.5 GitHub events -> code review agent

```yaml
# EventSource -- GitHub webhooks
apiVersion: argoproj.io/v1alpha1
kind: EventSource
metadata:
  name: github-events
spec:
  github:
    pull-requests:
      repositories:
        - owner: myorg
          names:
            - backend-api
            - frontend-app
      webhook:
        endpoint: /github
        port: "13000"
        method: POST
      events:
        - "pull_request"
      apiToken:
        name: github-token
        key: token
      webhookSecret:
        name: github-webhook-secret
        key: secret
---
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTrigger
metadata:
  name: pr-review
spec:
  eventSource:
    name: github-events
    eventName: pull-requests
  filter:
    data:
      - path: body.action
        type: string
        value:
          - "opened"
          - "synchronize"
  agent:
    name: code-review-agent
  task:
    sessionKeyFrom: "$.pull_request.number"
    metadata:
      source: github
  limits:
    maxInvocationsPerHour: 100
    maxConcurrentTasks: 10
    tokenBudgetPerEvent: 20000
```

## 8. Implementation Plan

### 8.1 New files

| Path | Purpose |
|---|---|
| `operator/api/v1alpha1/agenttrigger_types.go` | CRD type definitions |
| `operator/internal/webhook/v1alpha1/agenttrigger_webhook.go` | Admission webhook (validation) |
| `operator/internal/controller/agenttrigger_controller.go` | Controller shell: fetch, finalizer, delegate to app service, error classification, status persistence |
| `operator/internal/controller/agenttrigger_conditions.go` | Condition type/reason re-exports from domain layer |
| `operator/internal/domain/trigger/validate.go` | Pure validation functions (no I/O) |
| `operator/internal/domain/trigger/status.go` | Condition types, reasons, `SetCondition()` helpers |
| `operator/internal/domain/trigger/labels.go` | Label generation for child resources |
| `operator/internal/infra/repo/agenttrigger.go` | Repository interface + implementation for AgentTrigger reads |
| `operator/internal/app/trigger/reconcile.go` | App service with `Deps` struct, `Service` type, reconcile orchestration |
| `operator/internal/server/agenttrigger_service.go` | gRPC service implementation (CRUD) |
| `operator/internal/server/trigger_handler.go` | flokoa-server event -> A2A handler |
| `operator/internal/server/trigger_limiter.go` | Rate/cost limit enforcement (in-memory sliding window) |
| `operator/internal/server/trigger_session.go` | Session key extraction + context resolution |
| `operator/internal/server/push_gateway.go` | Push notification gateway for agentRef targets |
| `operator/server/proto/flokoa/agent/v1alpha1/agenttrigger_service.proto` | gRPC service definition (CRUD) |
| `operator/server/proto/flokoa/agent/v1alpha1/agenttrigger.proto` | Proto message types |
| `operator/config/crd/bases/agent.flokoa.ai_agenttriggers.yaml` | Generated CRD manifest |
| `operator/config/rbac/agenttrigger_admin_role.yaml` | RBAC admin role |
| `operator/config/rbac/agenttrigger_editor_role.yaml` | RBAC editor role |
| `operator/config/rbac/agenttrigger_viewer_role.yaml` | RBAC viewer role |
| `operator/config/samples/agent_v1alpha1_agenttrigger.yaml` | Sample CRs |
| `sdk/python/flokoa-types/src/flokoa_types/agenttrigger.py` | Generated Pydantic models |

### 8.2 Modified files

| Path | Change |
|---|---|
| `operator/api/v1alpha1/groupversion_info.go` | Register AgentTrigger type in `init()` |
| `operator/cmd/main.go` | Register AgentTrigger controller, webhook, and gRPC service |
| `operator/internal/server/server.go` | Add `agentTriggerService` parameter, register service + gateway, mount invoke and push HTTP routes |
| `operator/internal/infra/repo/interfaces.go` | Add `AgentTriggerReader`/`AgentTriggerWriter` interfaces |
| `operator/internal/infra/repo/fakes/fakes.go` | Add test fakes for new interfaces |
| `operator/go.mod` | Add Argo Events API types as dependency |
| `operator/charts/flokoa/templates/` | CRD template, RBAC |
| `CLAUDE.md` | Add AgentTrigger to CRD table |
| `docs/architecture.md` | Add event trigger architecture section |

### 8.3 Dependencies

| Dependency | Purpose | Import scope |
|---|---|---|
| `github.com/argoproj/argo-events/pkg/apis/sensor/v1alpha1` | Argo Sensor API types | Types only (no controller runtime) -- **NEW** |
| `github.com/argoproj/argo-events/pkg/apis/eventsource/v1alpha1` | Argo EventSource API types | Types only -- **NEW** |
| `github.com/argoproj/argo-events/pkg/apis/eventbus/v1alpha1` | Argo EventBus API types | Types only -- **NEW** |
| `github.com/PaesslerAG/jsonpath` (or similar) | JSONPath evaluation for sessionKeyFrom | flokoa-server only -- **NEW** |
| `github.com/a2aproject/a2a-go` | A2A protocol client | Already present at v0.3.6 |
| `github.com/argoproj/argo-workflows/v3` | Argo Workflows types | Already present at v3.7.9 |

### 8.4 Execution order

Follows the layered architecture -- types first, then domain, then infra, then app, then controller:

1. **Types** -- `agenttrigger_types.go`, register in `groupversion_info.go`
2. **Generate** -- `make manifests generate`
3. **Domain** -- `domain/trigger/validate.go`, `status.go`, `labels.go`
4. **Infra** -- `repo/agenttrigger.go`, update `interfaces.go`, add fakes
5. **App** -- `app/trigger/reconcile.go` (orchestrate sub-reconcilers)
6. **Controller** -- `controller/agenttrigger_controller.go` + conditions + watchers
7. **Webhook** -- `internal/webhook/v1alpha1/agenttrigger_webhook.go`
8. **Registration** -- `cmd/main.go` (controller + webhook)
9. **Data plane** -- `trigger_handler.go` + `trigger_limiter.go` + `trigger_session.go`
10. **Push gateway** -- `push_gateway.go` (forward push notifications for agentRef targets)
11. **Server routes** -- mount handler and gateway in `server.go`
12. **Tests** -- unit tests for each layer (domain, infra, app, controller, handler, limiter, session, push gateway)
13. **Proto + gRPC** -- `agenttrigger.proto` + `agenttrigger_service.proto` + `make buf-generate` + service implementation
14. **Python types** -- `make generate-python-models`
15. **Helm + RBAC** -- chart templates, RBAC manifests
16. **Samples + docs** -- example CRs, architecture doc update, CLAUDE.md CRD table

## 9. Resolved Design Decisions

1. **Session store backend** -- The session key -> contextId mapping will use pluggable session stores (see AgentService CRD in the feature roadmap). flokoa-server itself is a task store for monitoring and GetTask queries, not the session persistence layer. The session store is configured per-agent via AgentService, with Postgres as the default managed backend.

2. **Token budget enforcement** -- `tokenBudgetPerEvent` is optional. When set, flokoa-server injects it into A2A task metadata as `flokoa.token_budget`. The agent runtime SHOULD honor it -- enforcement is the agent's responsibility, not the platform's. This is advisory by design: the agent knows its own token accounting better than the gateway.

3. **Argo Events as hard dependency** -- Argo Events must be installed for AgentTrigger to function. The controller checks for the existence of Argo Events CRDs at startup. If they are not present, the controller logs a clear error and sets all AgentTrigger resources to Failed phase with a `SensorReady=False` condition and a message indicating Argo Events is required. No graceful degradation.

4. **Push notification to agentRef** -- All push notifications targeting another flokoa Agent route through flokoa-server's push gateway (`/api/v1alpha1/namespaces/{namespace}/agents/{agent-name}/push`). flokoa-server is a fully functional agent gateway: it receives the notification, logs it, records metrics, propagates trace context, and forwards to the target agent's A2A endpoint. Agents never need to know each other's internal Service URLs.

5. **Multi-part events** -- Structured data via DataPart only for v1. Binary payloads are deferred to a future version. The agent receives structured JSON data in a DataPart with `mediaType: "application/json"`; if the event references binary content (e.g., an S3 key), the agent can fetch it independently.

6. **A2A protocol version** -- This RFC targets A2A v1 HTTP+JSON binding. The initial implementation will use `a2a-go v0.3.6` (JSON-RPC transport). The a2a-go library's `SendMessage()` API maps directly to the v1 semantics described here. Push notification config is sent inline via `MessageSendConfig.PushConfig` (not via a separate `CreateTaskPushNotificationConfig` call). Agent endpoints are tried with both `{url}` and `{url}/a2a` suffixes for compatibility with different agent deployments.

7. **Layered controller architecture** -- The AgentTrigger controller follows the established layered pattern: controller shell -> app service -> domain -> infra. This matches the Agent controller (not the simpler Instruction controller pattern) because the trigger has complex cross-resource dependencies and needs testable business logic separation.

8. **Auth types divergence** -- `TriggerPushAuth` uses `Schemes []string` (plural, matching A2A `PushAuthInfo`) with `CredentialsRef *corev1.SecretKeySelector` (secret reference). This intentionally diverges from the existing `PushNotificationAuth` (which uses inline `Credentials string`) because operator-managed CRDs should not store credentials directly. AgentWorkflow's inline credentials are acceptable for ephemeral, workflow-scoped push configs.
