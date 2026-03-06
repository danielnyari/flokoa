# Flokoa Architecture

This document provides an overview of how Flokoa components interact and the overall system architecture.

## System Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│                       Kubernetes Cluster                             │
│                                                                      │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │              Flokoa Operator (Control Plane)                  │  │
│  │                                                                │  │
│  │  • Watches 6 CRDs (Agent, Model, ModelProvider,               │  │
│  │    AgentTool, Instruction, AgentWorkflow)                      │  │
│  │  • Reconciles desired state → Deployments, Services           │  │
│  │  • Compiles AgentWorkflows → Argo WorkflowTemplates           │  │
│  │  • gRPC API server with OIDC auth                             │  │
│  │  • OpenTelemetry observability                                │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                          │                                           │
│            creates/manages│                                          │
│                          ▼                                           │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │                  Agent Runtime Resources                      │  │
│  │                                                                │  │
│  │  ┌─────────────────────┐     ┌──────────────────────────┐    │  │
│  │  │  Agent Deployment   │     │  Agent Service           │    │  │
│  │  │  (Pods)             │     │                          │    │  │
│  │  │  • AI agent code    │◄───►│  • Stable endpoint       │    │  │
│  │  │  • A2A endpoints    │     │  • Load balancing        │    │  │
│  │  └─────────────────────┘     └──────────────────────────┘    │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                          │                                           │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │               Argo Workflows Integration                      │  │
│  │                                                                │  │
│  │  • AgentWorkflow → Argo WorkflowTemplate compilation          │  │
│  │  • A2A Executor Plugin (sidecar, port 4355)                   │  │
│  │  • DAG-based multi-agent orchestration                        │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                          │                                           │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │                Configuration Resources                        │  │
│  │                                                                │  │
│  │  • ModelProvider (LLM connection config)                      │  │
│  │  • Model (LLM parameters)                                    │  │
│  │  • AgentTool (External API integrations)                      │  │
│  │  • Instruction (System prompts → ConfigMaps)                  │  │
│  │  • Secrets (API keys, credentials)                            │  │
│  └───────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────┘
                          │
                          │ calls
                          ▼
        ┌──────────────────────────────────────────┐
        │        External Services                  │
        │                                           │
        │  • OpenAI API                             │
        │  • Anthropic API                          │
        │  • Google Gemini API / Vertex AI          │
        │  • AWS Bedrock                            │
        │  • External HTTP APIs (via AgentTools)    │
        └──────────────────────────────────────────┘
```

## Core Components

### 1. Flokoa Operator

The Kubernetes operator that manages the lifecycle of AI agents:

**Responsibilities:**
- Watches for changes to all six CRDs
- Creates and manages Kubernetes resources (Deployments, Services, ConfigMaps)
- Compiles AgentWorkflows into Argo WorkflowTemplates
- Handles updates, scaling, and deletion of agents
- Reports status and health information

**Architecture Layers:**
- **API layer** (`api/v1alpha1/`) - CRD type definitions with kubebuilder markers
- **Controller layer** (`internal/controller/`) - Kubernetes reconciliation loops and provider implementations
- **Domain layer** (`internal/app/agent/`) - Business logic for agent reconciliation (tools, instructions, models)
- **Infrastructure layer** (`internal/infra/`) - Kubernetes resource builders and repository pattern
- **Server layer** (`internal/server/`) - gRPC API with OIDC auth and grpc-gateway REST proxy

**Controller Reconciliation Loop:**
```
User creates/updates Agent CR
         ↓
Operator detects change
         ↓
Validates Agent spec
         ↓
Resolves Model and ModelProvider references
         ↓
Resolves AgentTool references
         ↓
Resolves Instruction (inline or ref)
         ↓
Creates/Updates Deployment
         ↓
Creates/Updates Service
         ↓
Updates Agent status
         ↓
Continues monitoring
```

### 2. gRPC API Server

The operator includes a gRPC server for programmatic access:
- CRUD operations for all CRD resources
- OIDC authentication via go-oidc
- REST API via grpc-gateway
- Protobuf definitions managed with buf

### 3. Custom Resource Definitions (CRDs)

The operator manages six CRDs under `agent.flokoa.ai/v1alpha1`:

#### Agent
The main resource representing a deployed AI agent.

**Key Interactions:**
- **References** → Model (for LLM access)
- **References** → AgentTool (for external capabilities)
- **References/Creates** → Instruction (system prompt)
- **Creates** → Deployment (pod runtime)
- **Creates** → Service (network endpoint)

#### ModelProvider
Connection configuration for LLM providers.

**Key Interactions:**
- **References** → Secret (API keys)
- **Referenced by** → Model (provider selection)

#### Model
Specific LLM model with parameters.

**Key Interactions:**
- **References** → ModelProvider (connection config)
- **Referenced by** → Agent (model selection)

#### AgentTool
External tool/API integration backed by OpenAPI specifications.

**Key Interactions:**
- **May reference** → Service (internal Kubernetes service)
- **May reference** → ConfigMap (OpenAPI specs)
- **Referenced by** → Agent (tool usage)

#### Instruction
System prompt management.

**Key Interactions:**
- **Creates** → ConfigMap (instruction text)
- **Referenced by** → Agent (system prompt)
- **Can be created by** → Agent (inline template)

#### AgentWorkflow
Multi-agent workflow definitions.

**Key Interactions:**
- **References** → Agent (task targets via A2A)
- **Creates** → Argo WorkflowTemplate (compiled workflow)
- **Supports** → DAG dependencies, conditions, retries

## Resource Relationships

```
┌─────────────────┐
│  ModelProvider  │ ◄─────┐
│                 │       │
│  • API Key      │       │ references
│  • Endpoint     │       │
└─────────────────┘       │
                          │
                    ┌─────┴─────┐
                    │   Model   │
                    │           │
                    │  • Name   │
                    │  • Params │
                    └─────┬─────┘
                          │
                          │ references
                          │
┌──────────────┐         ▼
│ Instruction  │   ┌───────────────────┐     ┌──────────────┐
│              │◄──│      Agent        │────►│  Deployment  │
│ • Content    │   │                   │     │  • Pods      │
│ → ConfigMap  │   │  • Card (A2A)     │     │  • Replicas  │
└──────────────┘   │  • Runtime        │     └──────────────┘
                   │  • Model ref      │
┌──────────────┐   │  • Instruction    │     ┌──────────────┐
│  AgentTool   │◄──│  • Tools          │────►│   Service    │
│              │   └────────┬──────────┘     │  • Endpoint  │
│ • OpenAPI    │            │                └──────────────┘
│ • URL/SvcRef │            │
└──────────────┘            │ referenced by
                            ▼
                   ┌───────────────────┐     ┌──────────────────┐
                   │  AgentWorkflow    │────►│ Argo Workflow    │
                   │                   │     │ Template         │
                   │  • Tasks (DAG)    │     │ (compiled)       │
                   │  • Params         │     └──────────────────┘
                   │  • Conditions     │
                   └───────────────────┘
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
    name: "My Agent"
    description: "Example agent"
    version: "1.0.0"
    skills:
      - id: "main"
        name: "Main"
        description: "Main capability"
        tags: ["general"]
  model:
    name: gpt-4o-model
  instruction:
    template: "You are a helpful assistant."
  tools:
    - toolRef:
        name: weather-tool
  runtime:
    type: standard
    standard:
      container:
        image: my-agent:v1.0.0
```

**What happens:**
1. User applies Agent manifest
2. Operator validates the spec
3. Operator resolves model reference → checks if Model exists and is ready
4. Operator resolves tool references → checks if AgentTools exist
5. Operator processes instruction (creates Instruction CR if inline template)
6. Operator creates Deployment with agent container
7. Operator creates Service to expose agent
8. Operator updates Agent status to "Pending"
9. Pods start, containers pull image
10. Containers become ready
11. Operator updates Agent status to "Running"
12. Agent URL is available in status

### 2. Agent Update

When you update an Agent:
1. Operator detects spec change
2. Updates underlying Deployment
3. Kubernetes performs rolling update of pods
4. New pods start with updated configuration
5. Old pods terminate after new ones are ready
6. Agent remains available during update (if replicas > 1)

### 3. Agent Scaling

```bash
kubectl patch agent my-agent --type='json' \
  -p='[{"op": "replace", "path": "/spec/runtime/standard/replicas", "value": 5}]'
```

### 4. Agent Deletion

```bash
kubectl delete agent my-agent
```

**What happens:**
1. Agent marked for deletion
2. Operator removes finalizers
3. Deployment is deleted
4. Pods are terminated gracefully
5. Service is deleted
6. Child Instruction CR (if any) is deleted
7. Agent resource is removed

## Runtime Backends

### Standard Runtime

The default runtime backend using Kubernetes Deployments:

**Components Created:**
- **Deployment**: Manages agent pod replicas
- **Service**: ClusterIP service exposing agent
- **ConfigMap** (optional): For instruction and configuration data

**Features:**
- Pod replica management
- Rolling updates
- Health checks
- Resource limits
- Volume mounts
- Affinity/anti-affinity
- Node selection
- Security contexts

### Template Runtime

Operator-managed runtime using a generic image:

**Components Created:**
- **Deployment**: With `flokoa-managed-agent` image
- **Service**: ClusterIP service
- **ConfigMap**: Mounted configuration (model, tools, instruction, output schema)
- **Secret**: Mounted API credentials

**Features:**
- No custom container image needed
- Agent behavior defined via Instruction and output schema
- Uses pydantic-ai framework internally
- Supports all standard scheduling options

## AgentWorkflow Compilation

AgentWorkflow CRDs are compiled into Argo Workflow resources:

```
AgentWorkflow spec.tasks
         ↓
Compiler (agentworkflow_compiler.go)
         ↓
Translates to Argo DAG template
         ↓
Task types mapped:
  • agent → A2A executor plugin template
  • agentTask → Container template (flokoa-managed-task)
  • container → Container template
  • http → HTTP template
  • switch → Argo condition/when
         ↓
Creates Argo WorkflowTemplate CR
         ↓
Status.workflowTemplateName updated
```

The A2A executor plugin runs as a sidecar in workflow pods, calling agents via the A2A protocol.

## Tool Integration

### How Agents Use Tools

```
Agent Pod
  ↓
  │ needs to call API
  ↓
Reads AgentTool spec
  ↓
  │ knows: URL, method, schema
  ↓
Constructs HTTP request
  ↓
  │ with LLM-generated params
  ↓
Calls external API
  ↓
Receives response
  ↓
Returns to LLM
```

### Internal vs External Tools

**Internal (serviceRef):**
```yaml
openApi:
  serviceRef:
    name: inventory-service
    namespace: backend
    port: 8080
  openApiSchema:
    endpointPath: "/openapi.json"
```

**External (url):**
```yaml
openApi:
  url: "https://api.external.com"
  openApiSchema:
    endpointPath: "/openapi.json"
```

## Model Resolution

When an Agent references a Model:

```
Agent spec.model.name = "gpt-4o-model"
         ↓
Operator finds Model CR
         ↓
Model spec.providerRef.name = "openai-provider"
         ↓
Operator finds ModelProvider CR
         ↓
ModelProvider spec.apiKeySecretRef
         ↓
Operator reads Secret
         ↓
Agent container gets:
  • Model name (gpt-4o)
  • Model parameters
  • Provider endpoint
  • API key
```

## Namespace Organization

### Single Namespace (Simple)

```
namespace: default
  • agents, models, modelproviders,
    agenttools, instructions, agentworkflows
```

Good for: Small deployments, development

### Multi-Namespace (Organized)

```
namespace: shared-resources
  • modelproviders, instructions (shared prompts)

namespace: shared-models
  • models

namespace: shared-tools
  • agenttools

namespace: app-1
  • agents, agentworkflows
  • app-specific models, tools, instructions

namespace: app-2
  • agents, agentworkflows
```

Good for: Multi-team, multi-app deployments

## Security Architecture

### Secrets Management

```
API Keys → Kubernetes Secrets
              ↓
         Referenced by:
              ↓
    ┌────────┴────────┐
    ↓                 ↓
ModelProvider    AgentTool
    ↓                 ↓
Referenced by:   Used by:
    ↓                 ↓
  Model            Agent
    ↓
Referenced by:
    ↓
  Agent
```

### RBAC

The operator provides pre-built roles:
- **Admin** - Full CRUD on all Flokoa CRDs
- **Editor** - Create/update/delete
- **Viewer** - Read-only access

## Observability

### Metrics

The operator exposes Prometheus metrics. Agents can expose custom metrics:
```yaml
spec:
  runtime:
    standard:
      container:
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics
```

### Logging

```bash
kubectl logs -l flokoa.ai/agent=my-agent
```

### Tracing

Agents and the operator integrate with OpenTelemetry:
```yaml
env:
- name: OTEL_ENDPOINT
  value: "http://otel-collector:4317"
```

### Status Conditions

Operator maintains conditions on all resources:
```yaml
status:
  conditions:
  - type: Ready
    status: "True"
    reason: DeploymentAvailable
```

## Best Practices

1. **Separation of Concerns**: Keep providers, models, and agents in appropriate namespaces
2. **Reuse Resources**: Share ModelProviders, Instructions, and common tools across agents
3. **Security First**: Use secrets, RBAC, network policies
4. **Resource Limits**: Always set CPU/memory limits
5. **Health Checks**: Configure liveness and readiness probes
6. **High Availability**: Use multiple replicas with anti-affinity
7. **Monitoring**: Integrate with observability stack
8. **Version Control**: Keep manifests in Git
9. **Environment Isolation**: Use namespaces for dev/staging/prod
10. **Cost Management**: Monitor LLM usage and set appropriate limits
