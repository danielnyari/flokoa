# Flokoa Architecture

This document provides an overview of how Flokoa components interact and the overall system
architecture. The normative operator↔runner interface is the
[runtime contract](reference/runtime-contract.md); the operating principles are in
[core beliefs](design-docs/core-beliefs.md).

## System Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│                          Kubernetes Cluster                           │
│                                                                        │
│  ┌──────────────────────────────────────────────────────────────┐    │
│  │             Flokoa Operator (Control Plane)                   │    │
│  │  • Watches CRDs (Agent, Model, ModelProvider, Instruction,    │    │
│  │    AgentTool, AgentTrigger, AgentWorkflow)                     │    │
│  │  • COMPILES each Agent (refs + inline fragment) into one       │    │
│  │    resolved pydantic-ai AgentSpec; validates it (SpecValid)    │    │
│  │  • Injects platform capabilities (telemetry, …)               │    │
│  │  • Emits the <agent>-agent-spec ConfigMap + Deployment +       │    │
│  │    Services; reconciles status (url, specHash, conditions)     │    │
│  └──────────────────────────────────────────────────────────────┘    │
│            │ compiles + creates                                        │
│            ▼                                                           │
│  ┌──────────────────────────────────────────────────────────────┐    │
│  │                   Agent Runtime Resources                     │    │
│  │  ┌────────────────────────────────────────────────────────┐  │    │
│  │  │  <agent>-agent-spec ConfigMap                          │  │    │
│  │  │  • compiled AgentSpec (secrets stay as ${secret:…})    │  │    │
│  │  │  • agent card                                          │  │    │
│  │  └────────────────────────────────────────────────────────┘  │    │
│  │  ┌────────────────────────────────────────────────────────┐  │    │
│  │  │  Deployment → generic runner pods (flokoa-runner)      │  │    │
│  │  │  • hydrate the spec, resolve ${secret:…} from          │  │    │
│  │  │    FLOKOA_SECRET_* env, install capabilities           │  │    │
│  │  │  • Agent.from_spec() → serve A2A on :8080              │  │    │
│  │  └────────────────────────────────────────────────────────┘  │    │
│  │  ┌────────────────────────────────────────────────────────┐  │    │
│  │  │  Services: {agent} (published, :80, status.url) +      │  │    │
│  │  │            {agent}-runtime (internal workload)         │  │    │
│  │  └────────────────────────────────────────────────────────┘  │    │
│  └──────────────────────────────────────────────────────────────┘    │
│            │ references (resolved at compile time)                     │
│            ▼                                                           │
│  ┌──────────────────────────────────────────────────────────────┐    │
│  │  Configuration CRs                                            │    │
│  │  • Model + ModelProvider (model id, settings, provider env)   │    │
│  │  • Instruction (system-prompt → ConfigMap)                    │    │
│  │  • AgentTool (declarative MCP endpoint)                       │    │
│  │  • Capability (packaged wheelhouses — P0b)                    │    │
│  │  • Secrets (projected into the runner, never into the spec)   │    │
│  └──────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────┘
             │ calls
             ▼
   ┌──────────────────────────────────────────────┐
   │  External Services                            │
   │  • OpenAI / Anthropic / Google / Bedrock      │
   │  • External MCP servers (via AgentTools)      │
   └──────────────────────────────────────────────┘
```

## Core Components

### 1. Flokoa Operator

The Kubernetes operator is a **compiler**: it turns declarative CRs into a running agent.

**Responsibilities:**
- Watches Agent, Model, ModelProvider, Instruction, AgentTool, AgentTrigger, and AgentWorkflow resources
- Compiles each Agent — its referenced Model/Instruction/AgentTool CRs plus an optional inline
  AgentSpec fragment — into one resolved pydantic-ai AgentSpec (composition root → compiler)
- Injects platform capabilities (telemetry today; session persistence and budget guardrail are
  reserved for P1) into every compiled spec
- Validates the compiled spec against the runner's pinned AgentSpec JSON Schema, surfacing a
  `SpecValid` condition and the resolved-spec hash (`status.specHash`)
- Writes the `<agent>-agent-spec` ConfigMap and creates/updates the Deployment (generic runner)
  and the published + internal Services
- Reports status: phase, `status.url`, `status.specHash`, `status.injectedCapabilities`, conditions

**Controller Reconciliation Loop:**
```
User creates/updates Agent CR
         ↓
Operator detects change → validates the spec (admission webhook)
         ↓
Resolves Model (+ ModelProvider), Instructions, and AgentTools
         ↓
Merges refs + inline fragment → one resolved AgentSpec
  (referenced CRs in declared order; inline scalars win; list fields append)
         ↓
Injects platform capabilities (telemetry, …)
         ↓
Validates the compiled spec vs the pinned AgentSpec JSON Schema → SpecValid
         ↓
Writes the <agent>-agent-spec ConfigMap
         ↓
Creates/Updates the Deployment (generic runner) + published Service
         ↓
Updates Agent status (phase, url, specHash, conditions)
         ↓
Continues monitoring (Secret/Model/Instruction/Tool changes recompile)
```

When compilation fails, `SpecValid=False` is set and **no Deployment update happens** — the last
good generation keeps running.

### 2. Custom Resource Definitions (CRDs)

Seven CRDs under `agent.flokoa.ai/v1alpha1` today (Capability arrives in P0b):

#### Agent
The **composition root**: an inline AgentSpec fragment plus `modelRef`, `instructionRefs`, `tools`,
`secretRefs`, `card` (A2A metadata), and `runtime` (image/runnerVersion/isolation/resources),
compiled into one resolved AgentSpec.

**Key Interactions:**
- **References** → Model, Instruction, AgentTool (composed by the compiler)
- **Creates** → `<agent>-agent-spec` ConfigMap (compiled AgentSpec + card)
- **Creates** → Deployment (generic runner pods)
- **Creates** → published `{agent}` Service (behind `status.url`) and internal `{agent}-runtime` Service

#### Model & ModelProvider
**Model** is a named, shareable model config (identifier + typed `settings` + `providerRef`) that
compiles to AgentSpec `model`/`model_settings`. **ModelProvider** is the connection behind a Model:
it projects provider-native env vars and the pydantic-ai model prefix into the runner. See
[model.md](model.md) and [modelprovider.md](modelprovider.md).

#### Instruction
A versioned, shareable system-prompt block (`content`). The controller writes it to a ConfigMap and
the compiler appends it, in declared order, into the AgentSpec `instructions`. See [instruction.md](instruction.md).

#### AgentTool
A **declarative MCP endpoint** (`url` OR `serviceRef`+`path`, `transport`, `headers`/`headerSecrets`,
`toolPrefix`, `allowedTools`, `timeoutSeconds`) that compiles to an MCP capability entry. The
`openapi`/`http-api` type is retired and rejected by admission. See [agenttool.md](agenttool.md).

**Key Interactions:**
- **May reference** → in-cluster Service (`serviceRef`+`path`)
- **May reference** → Secret (`headerSecrets`)
- **Referenced by** → Agent

#### AgentTrigger
Event-driven invocation built on **Argo Events**: references an EventSource/EventBus and compiles to
a Sensor that POSTs matching events to flokoa-server's invoke endpoint, with filters, rate/budget
limits, session-key extraction, and A2A push-notification delivery. See [agenttrigger.md](agenttrigger.md).

#### AgentWorkflow
**Frozen**, template-only: static A2A composition between deployed Agents, compiled to Argo
`WorkflowTemplate`s that call agents via the [A2A executor plugin](argo/executor-plugins.md). The
Argo Workflows execution path was removed; the `agentTask` task type is rejected by admission.

#### Capability (P0b — not yet shipped)
A versioned, digest-pinned OCI wheelhouse + JSON Schema config + `requires` tuple. Admission will
validate the schema, the `requires` check, and dependency conflicts.

## Resource Relationships

```
┌─────────────────┐
│  ModelProvider  │ ◄─────┐
│  • API key ref  │       │ references
│  • Endpoint     │       │
└─────────────────┘       │
                    ┌─────┴─────┐
                    │   Model   │  • identifier
                    │           │  • settings
                    └─────┬─────┘
                          │ modelRef
┌─────────────┐           │           ┌──────────────┐
│ Instruction │──────┐    │      ┌───►│ <agent>-spec │  compiled AgentSpec
│ • content   │      │    │      │    │   ConfigMap  │  + agent card
└─────────────┘      ▼    ▼      │    └──────┬───────┘
                  ┌────────────────┐         │ consumed by
┌─────────────┐   │     Agent      │─────────┤
│  AgentTool  │◄──│ (composition   │         ▼
│ • url /     │   │     root →     │   ┌──────────────┐
│   serviceRef│   │   compiled)    │──►│  Deployment  │ generic runner pods
│ • → MCP     │   └────────┬───────┘   └──────────────┘
└─────────────┘            │ creates
                           ▼
              ┌────────────────────────────┐
              │ Services                   │
              │ • {agent} (published, :80) │ ← status.url (virtual endpoint)
              │ • {agent}-runtime (internal)│ ← never addressed directly
              └────────────────────────────┘
```

## Agent Lifecycle

### 1. Agent Creation

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: my-agent
spec:
  card:
    name: my-agent
    description: "Demo agent"
    version: "1.0.0"
    skills:
      - {id: demo, name: Demo, description: "Demo", tags: [demo]}
  modelRef:
    name: gpt-4o-model
  tools:
    - name: weather-tool
```

**What happens:**
1. User applies the Agent manifest
2. The admission webhook validates the composition statically
3. The compiler resolves the Model (+ ModelProvider), Instructions, and AgentTools, merges them
   with the inline fragment into one resolved pydantic-ai AgentSpec, injects platform capabilities,
   and validates it against the runner's pinned AgentSpec JSON Schema (`SpecValid` condition;
   `status.specHash`)
4. The operator writes the `<agent>-agent-spec` ConfigMap (compiled spec + agent card)
5. The operator creates the Deployment (generic runner image) and the published Service behind
   `status.url`, plus the internal `{agent}-runtime` workload Service
6. The runner pod hydrates the spec — resolving `${secret:…}` placeholders from `FLOKOA_SECRET_*`
   env, installing referenced capabilities — constructs the agent via `Agent.from_spec`, and serves A2A
7. Containers become ready; the operator updates Agent status to `Running` and publishes `status.url`

### 2. Agent Update

When you update an Agent (or a referenced Model/Instruction/AgentTool/Secret changes):
1. The operator recompiles the AgentSpec and rewrites the ConfigMap
2. The Deployment rolls out with the new spec
3. Kubernetes performs a rolling update of the runner pods
4. The agent remains available during the update (if replicas > 1)

### 3. Agent Scaling

```bash
kubectl patch agent my-agent --type='json' \
  -p='[{"op": "replace", "path": "/spec/runtime/replicas", "value": 5}]'
```

### 4. Agent Deletion

```bash
kubectl delete agent my-agent
```

Owned resources (ConfigMap, Deployment, Services) are garbage-collected via owner references.

## Runtime

The default runtime is the **generic runner** (`flokoa-runner`) on a Kubernetes Deployment: the
operator-built image that hydrates the compiled-spec ConfigMap, installs referenced capabilities,
and serves A2A. The only axes are:

- **Isolation tier** (`spec.runtime.isolation`): `shared` (today — pooled runner pods, many
  sessions) and `session` (one sandbox per A2A context, **P1, not yet shipped**).
- **Runner version** (`spec.runtime.runnerVersion`): each release pins one runner version; this
  per-Agent override is the escape hatch.
- **Custom image** (`spec.runtime.image`): bring your own container — the escape hatch when the
  generic runner isn't enough.

There is no menu of pluggable "backends"; building a custom image is the exception, not a co-equal
mode. The operator manages the Deployment, Service, health probes, and rolling updates.

## Tool Integration

### How Agents Use Tools

AgentTools are **MCP endpoints**. The runner connects to the MCP server compiled from each AgentTool
and exposes its tools to the agent:

```
Agent (runner)
  ↓ connects to the MCP endpoint compiled from the AgentTool
  │   (serviceRef+path or url, with transport + header auth)
  ↓
MCP server  →  advertises tools (filtered by allowedTools / toolPrefix)
  ↓
pydantic-ai calls the tools over the MCP protocol
  ↓
Results return to the model
```

### Internal vs External Tools

MCP is the only AgentTool type (the `openapi`/`http-api` type is retired and rejected by admission).

**Internal (serviceRef):**
```yaml
spec:
  type: mcp
  serviceRef:
    name: inventory-service
    namespace: backend
    port: 8080
  path: /mcp
```
- Targets an in-cluster MCP service; stays in-cluster, no egress required

**External (url):**
```yaml
spec:
  type: mcp
  url: "https://mcp.external.com/mcp"
```
- Targets an external MCP server; requires egress and usually authentication (via `headerSecrets`)

## Model Resolution

When an Agent references a Model, the compiler resolves it — it does **not** read secrets:

```
Agent spec.modelRef.name = "gpt-4o-model"
         ↓
Compiler finds the Model CR (model identifier + settings)
         ↓
Model spec.providerRef → ModelProvider CR
         ↓
Compiler emits into the AgentSpec:
  • model = "<prefix>:<identifier>"   (e.g. openai:gpt-4o)
  • model_settings from the Model's typed settings (+ extra)
  • a ${secret:…} placeholder for the API key
         ↓
Operator projects the API-key Secret as a FLOKOA_SECRET_* env var
  (valueFrom.secretKeyRef) plus provider env (base URL, region, …)
         ↓
The runner resolves the placeholder at hydration time
```

Rotating a Model — or its provider's Secret — recompiles every referencing Agent.

## Namespace Organization

### Single Namespace (Simple)

```
namespace: default
  • agents
  • models
  • modelproviders
  • agenttools
```

Good for: Small deployments, development

### Multi-Namespace (Organized)

```
namespace: shared-resources
  • modelproviders (OpenAI, Anthropic, etc.)

namespace: shared-models
  • models (GPT-4, Claude, etc.)

namespace: shared-tools
  • agenttools (common integrations)

namespace: app-1
  • agents (specific to this app)
  • models (app-specific configs)
  • agenttools (app-specific tools)
```

Good for: Multi-team, multi-app deployments

### Environment Isolation

```
namespace: dev          • All development resources
namespace: staging      • Staging resources (may share models/providers with prod)
namespace: production    • Production agents, models, providers
```

Good for: Environment separation, security

## Security Architecture

### Secrets Management

Secret **values never leave the kubelet**. The compiler emits `${secret:NAME}` placeholders into
the compiled spec; the operator projects the referenced Secrets as `FLOKOA_SECRET_*` env vars
(`valueFrom.secretKeyRef`); the **runner** resolves the placeholders at hydration time. No secret
value is ever written into the compiled-spec ConfigMap, a CR, a log, or any compiled artifact.

```
Kubernetes Secret
   ↓ valueFrom.secretKeyRef (projected by the operator)
FLOKOA_SECRET_* env on the runner pod
   ↓ resolved at hydration
${secret:NAME} placeholders in the compiled spec
```

### RBAC Recommendations

**For Users:**
```yaml
kind: Role
rules:
- apiGroups: ["agent.flokoa.ai"]
  resources: ["agents"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]
- apiGroups: ["agent.flokoa.ai"]
  resources: ["agenttools"]
  verbs: ["get", "list"]
```

**For Agents:**
```yaml
# Agent service account - minimal permissions
kind: Role
rules:
- apiGroups: [""]
  resources: ["secrets"]
  resourceNames: ["specific-secret-name"]
  verbs: ["get"]
```

**For Operator:**
```yaml
kind: ClusterRole
rules:
- apiGroups: ["agent.flokoa.ai"]
  resources: ["*"]
  verbs: ["*"]
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["*"]
- apiGroups: [""]
  resources: ["services", "secrets", "configmaps"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
```

## Networking

### Service Discovery

Each Agent gets two Services. `status.url` is a **flokoa-owned virtual endpoint** — the only address
callers should use:

```
Agent: my-agent
  ↓
status.url → published Service: my-agent.default.svc.cluster.local:80
  ↓ targets
internal workload Service: my-agent-runtime  (port 8080 — never address directly)
  ↓
runner pods: my-agent-xxxx, my-agent-yyyy
```

Routing the published endpoint through a flokoa-owned identity is what lets the P1 session-routing
gateway be inserted later without breaking any caller.

### Ingress/Load Balancer

To expose an agent externally, target the **published** `{agent}` Service (never `{agent}-runtime`):

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: agent-ingress
spec:
  rules:
  - host: agents.example.com
    http:
      paths:
      - path: /my-agent
        pathType: Prefix
        backend:
          service:
            name: my-agent
            port:
              number: 80
```

### Network Policies

Restrict agent communication:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: agent-netpol
spec:
  podSelector:
    matchLabels:
      flokoa.ai/agent: my-agent
  policyTypes:
  - Ingress
  - Egress
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: UDP
      port: 53
```

## Observability

### Metrics & Tracing (platform-injected)

Telemetry is a **platform-injected capability** (`flokoa.platform/telemetry`) — the operator wires
OpenTelemetry into every agent with no user configuration, setting `OTEL_EXPORTER_OTLP_ENDPOINT`,
`OTEL_SERVICE_NAME`, and `OTEL_RESOURCE_ATTRIBUTES` (per-agent identity, GenAI semantic conventions,
and token usage). `status.injectedCapabilities` lists what was injected. You do not hand-configure
OTel env on the Agent.

### Logging

```bash
kubectl logs -l flokoa.ai/agent=my-agent
```

### Status Conditions

The operator maintains status on each Agent:

```yaml
status:
  phase: Running
  url: http://my-agent.default.svc.cluster.local:80/   # published virtual endpoint
  specHash: "sha256:…"                                  # resolved-spec hash (drift detection)
  injectedCapabilities: ["flokoa.platform/telemetry"]
  conditions:
  - type: SpecValid          # compiled spec validated against the pinned JSON Schema
    status: "True"
    reason: Compiled
  - type: Ready
    status: "True"
    reason: DeploymentAvailable
```

## Best Practices

1. **Separation of Concerns**: Keep providers, models, and agents in appropriate namespaces
2. **Reuse Resources**: Share ModelProviders, Models, and Instructions across agents
3. **Security First**: Use Secret refs + `${secret:…}` placeholders, RBAC, and network policies
4. **Resource Limits**: Always set CPU/memory limits
5. **Call the published endpoint**: Use `status.url` / the `{agent}` Service, never `{agent}-runtime`
6. **High Availability**: Use multiple replicas with anti-affinity
7. **Watch SpecValid**: A `SpecValid=False` condition means the new spec didn't compile — the last good generation keeps running
8. **Version Control**: Keep manifests in Git
9. **Environment Isolation**: Use namespaces for dev/staging/prod
10. **Cost Management**: Monitor LLM usage and set appropriate limits
