# AgentTrigger

`AgentTrigger` connects external events to flokoa Agents. When a matching event
arrives, the agent is invoked via the A2A protocol; results can be delivered
onward through A2A push notifications.

Event transport is built on **[Argo Events](https://argoproj.github.io/argo-events/)**:
an AgentTrigger references an existing Argo Events `EventSource` (webhook,
Kafka, SQS, cron, …) and the flokoa operator compiles it into an Argo Events
`Sensor` that forwards matching events to flokoa-server's invoke endpoint.

> The original design RFC (written against Knative Eventing, superseded by the
> Argo Events implementation) is preserved in git history as
> `docs/agenttrigger-rfc.md`.

## How it works

```
EventSource (Argo Events)          flokoa-server                     Agent
  webhook / kafka / sqs / …   ┌──────────────────────────┐
        │                     │ POST /api/v1alpha1/      │
        ▼                     │  namespaces/{ns}/        │   A2A SendMessage
  EventBus ──► Sensor ───────►│  agenttriggers/{name}/   ├──────────────────►
  (compiled by the operator)  │  invoke                  │
                              │  • rate/budget limits    │   A2A push notification
                              │  • session key extraction│◄──────────────────
                              └────────────┬─────────────┘
                                           │ push gateway (agent chaining)
                                           ▼
                                external webhook or another Agent
```

1. **Sensor compilation** — the AgentTrigger controller creates a child Argo
   Events `Sensor` (owned by the trigger, garbage-collected with it) that
   subscribes to the referenced `EventSource`/`EventBus` and applies the
   trigger's filters. The Sensor's name is surfaced in `status.sensorName`.
2. **Invoke endpoint** — matching events are POSTed to flokoa-server at
   `/api/v1alpha1/namespaces/{namespace}/agenttriggers/{name}/invoke`, which
   enforces the trigger's limits, extracts the session key, and sends an A2A
   message to the target Agent's endpoint (resolved from `status.url`, surfaced
   in `status.agentEndpoint`).
3. **Result delivery** — if `pushNotification` is configured, the A2A
   `PushNotificationConfig` attached to each request points either at an
   external HTTPS webhook or at flokoa-server's push gateway
   (`/api/v1alpha1/namespaces/{ns}/agents/{name}/push`), which proxies the
   notification to another Agent — enabling observable agent chaining:
   *event → Agent A → flokoa-server → Agent B*.

## Spec overview

| Field | Purpose |
|-------|---------|
| `eventSource` | Argo Events `EventSource` name + event name to consume (required) |
| `eventBus` | Non-default Argo Events `EventBus` (optional) |
| `filter.data[]` | JSONPath payload filters with Sensor data-filter semantics (AND across filters, OR within a filter's values; comparators `=`, `!=`, `>`, `<`, `>=`, `<=`) |
| `filter.exprs[]` | CEL expression filters for complex boolean logic |
| `agent` | The flokoa Agent to invoke (required) |
| `task.sessionKeyFrom` | JSONPath into the event payload; the extracted value becomes the A2A `contextId`, giving events from the same entity a shared conversation context |
| `task.metadata` | Static key/value pairs attached to every A2A task |
| `pushNotification` | Result destination: `agentRef` (via the push gateway) or external HTTPS `url`, with optional `authentication` (schemes + Secret-referenced credentials) and `tokenRef` |
| `limits` | `maxInvocationsPerHour`, `maxConcurrentTasks`, `tokenBudgetPerEvent`, `tokenBudgetPerHour`, and a `deadLetterSink` for dropped events |

### Limits and dead-lettering

Rate and budget limits are enforced in flokoa-server before the agent is
invoked. When a limit trips, the event is forwarded to
`limits.deadLetterSink.uri` (with `X-Flokoa-Drop-Reason`,
`X-Flokoa-Trigger-Name`, and `X-Flokoa-Trigger-Namespace` headers) or dropped
with a metric increment if no sink is configured.

## Example

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTrigger
metadata:
  name: payment-events
spec:
  eventSource:
    name: stripe-webhook
    eventName: payments
  filter:
    data:
      - path: body.type
        type: string
        value: ["payment_intent.payment_failed"]
  agent:
    name: billing-agent
  task:
    sessionKeyFrom: "$.data.object.customer"
    metadata:
      source: stripe
  pushNotification:
    url: https://ops.example.com/hooks/billing
    authentication:
      schemes: ["Bearer"]
      credentialsRef:
        name: ops-webhook-creds
        key: token
  limits:
    maxInvocationsPerHour: 500
    maxConcurrentTasks: 10
    deadLetterSink:
      uri: https://ops.example.com/hooks/dead-letter
```

## Status

| Field | Meaning |
|-------|---------|
| `status.phase` | `Pending` / `Running` / `Failed` |
| `status.sensorName` | Name of the compiled Argo Events Sensor |
| `status.agentEndpoint` | Resolved A2A endpoint of the target Agent |
| `status.invocations` | Best-effort counters (reset on controller restart; authoritative metrics come from Prometheus) |
| `status.conditions` | Latest observations from the controller |

## Prerequisites

- Argo Events installed in the cluster, with an `EventSource` (and `EventBus`,
  default name `default`) in the AgentTrigger's namespace.
- flokoa-server running with the invoke endpoint enabled (default in the Helm
  chart).
