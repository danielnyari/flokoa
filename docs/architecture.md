# Flokoa Architecture

This document provides an overview of how Flokoa components interact and the overall system architecture.

## System Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                        │
│                                                              │
│  ┌────────────────────────────────────────────────────┐    │
│  │           Flokoa Operator (Control Plane)          │    │
│  │                                                     │    │
│  │  • Watches CRDs (Agent, Model, ModelProvider, etc) │    │
│  │  • Reconciles desired state                        │    │
│  │  • Creates Deployments, Services, ConfigMaps       │    │
│  │  • Manages agent lifecycle                         │    │
│  └────────────────────────────────────────────────────┘    │
│                          │                                  │
│                          │ creates/manages                  │
│                          ▼                                  │
│  ┌────────────────────────────────────────────────────┐    │
│  │              Agent Runtime Resources                │    │
│  │                                                     │    │
│  │  ┌──────────────────────────────────────────────┐ │    │
│  │  │         Agent Deployment (Pods)              │ │    │
│  │  │                                              │ │    │
│  │  │  • AI Agent Container                        │ │    │
│  │  │  • Runs your agent code                      │ │    │
│  │  │  • Connects to LLM providers                 │ │    │
│  │  │  • Uses tools to call APIs                   │ │    │
│  │  └──────────────────────────────────────────────┘ │    │
│  │                                                     │    │
│  │  ┌──────────────────────────────────────────────┐ │    │
│  │  │         Agent Service (Load Balancer)        │ │    │
│  │  │                                              │ │    │
│  │  │  • Exposes agent pods                        │ │    │
│  │  │  • Provides stable endpoint                  │ │    │
│  │  └──────────────────────────────────────────────┘ │    │
│  └────────────────────────────────────────────────────┘    │
│                          │                                  │
│                          │ uses                             │
│                          ▼                                  │
│  ┌────────────────────────────────────────────────────┐    │
│  │            Configuration Resources                  │    │
│  │                                                     │    │
│  │  • ModelProvider (LLM connection config)           │    │
│  │  • Model (LLM parameters)                          │    │
│  │  • AgentTool (External API integrations)           │    │
│  │  • Secrets (API keys, credentials)                 │    │
│  │  • ConfigMaps (Configuration data)                 │    │
│  └────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
                          │
                          │ calls
                          ▼
        ┌──────────────────────────────────────────┐
        │        External Services                  │
        │                                           │
        │  • OpenAI API                             │
        │  • Anthropic API                          │
        │  • Google Gemini API                      │
        │  • AWS Bedrock                            │
        │  • External HTTP APIs (via AgentTools)    │
        └──────────────────────────────────────────┘
```

## Core Components

### 1. Flokoa Operator

The Kubernetes operator that manages the lifecycle of AI agents:

**Responsibilities:**
- Watches for changes to Agent, Model, ModelProvider, and AgentTool resources
- Creates and manages underlying Kubernetes resources (Deployments, Services)
- Handles updates, scaling, and deletion of agents
- Reports status and health information
- Detects AI frameworks used by agents

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
Creates/Updates Deployment
         ↓
Creates/Updates Service
         ↓
Updates Agent status
         ↓
Continues monitoring
```

### 2. Custom Resource Definitions (CRDs)

#### Agent
The main resource representing a deployed AI agent.

**Key Interactions:**
- **References** → Model (for LLM access)
- **References** → AgentTool (for external capabilities)
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
External tool/API integration.

**Key Interactions:**
- **May reference** → Service (internal Kubernetes service)
- **May reference** → ConfigMap (OpenAPI specs)
- **Referenced by** → Agent (tool usage)

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
                          ▼
┌─────────────────┐     ┌───────────────────┐     ┌──────────────┐
│   AgentTool     │ ◄───│      Agent        │────►│  Deployment  │
│                 │     │                   │     │              │
│  • Type         │     │  • Card (meta)    │     │  • Pods      │
│  • Description  │     │  • Runtime        │     │  • Replicas  │
│  • HTTP API     │     │  • Model ref      │     └──────────────┘
│  • Schema       │     │  • Tools          │
└─────────────────┘     └───────────────────┘     ┌──────────────┐
                                │                  │   Service    │
                                └─────────────────►│              │
                                                   │  • Endpoint  │
                                                   │  • Port      │
                                                   └──────────────┘
```

## Agent Lifecycle

### 1. Agent Creation

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: my-agent
spec:
  model:
    name: gpt-4o-model
  tools:
    - toolRef:
        name: weather-tool
  runtime:
    type: standard
    spec:
      container:
        image: my-agent:v1.0.0
```

**What happens:**
1. User applies Agent manifest
2. Operator validates the spec
3. Operator resolves model reference → checks if Model exists and is ready
4. Operator resolves tool references → checks if AgentTools exist
5. Operator creates Deployment with agent container
6. Operator creates Service to expose agent
7. Operator updates Agent status to "Pending"
8. Pods start, containers pull image
9. Containers become ready
10. Operator updates Agent status to "Running"
11. Agent URL is available in status

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
  -p='[{"op": "replace", "path": "/spec/runtime/spec/replicas", "value": 5}]'
```

**What happens:**
1. Agent spec updated with new replica count
2. Operator updates Deployment
3. Kubernetes scales pods up or down
4. Status reflects new replica counts

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
6. Agent resource is removed

## Runtime Backends

### Standard Runtime

The default runtime backend using Kubernetes Deployments:

**Components Created:**
- **Deployment**: Manages agent pod replicas
- **Service**: ClusterIP service exposing agent
- **ConfigMap** (optional): For configuration data
- **Secrets** (via references): For sensitive data

**Features:**
- Pod replica management
- Rolling updates
- Health checks
- Resource limits
- Volume mounts
- Affinity/anti-affinity
- Node selection
- Security contexts

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
Parses using output schema
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
- Calls Kubernetes service
- No external network required
- Faster, more secure
- No authentication typically needed

**External (url):**
```yaml
openApi:
  url: "https://api.external.com"
  openApiSchema:
    endpointPath: "/openapi.json"
```
- Calls external HTTP API
- Requires egress network access
- May need authentication
- Subject to external rate limits

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
  
namespace: app-2
  • agents
  • models
  • agenttools
```

Good for: Multi-team, multi-app deployments

### Environment Isolation

```
namespace: dev
  • All development resources
  
namespace: staging
  • Staging resources
  • May share models/providers with prod
  
namespace: production
  • Production agents
  • Production models
  • Production providers
```

Good for: Environment separation, security

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
    ↓                 ↓
Referenced by:   ┌───┴───┐
    ↓            ↓       ↓
  Agent        Reads    Executes
               at        at
             startup    runtime
```

### RBAC Recommendations

**For Users:**
```yaml
# Can create/manage agents
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
# Operator needs broad permissions
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

Agents are exposed via ClusterIP services:
```
Agent: my-agent
  ↓
Service: my-agent.default.svc.cluster.local:8080
  ↓
Pods: my-agent-xxxx, my-agent-yyyy
```

### Ingress/Load Balancer

To expose agents externally:

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
              number: 8080
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
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: ingress-nginx
  egress:
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          app: inventory-service
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: UDP
      port: 53
```

## Observability

### Metrics

Agents can expose Prometheus metrics:
```yaml
spec:
  runtime:
    spec:
      container:
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics
```

### Logging

Agent logs available via:
```bash
kubectl logs -l flokoa.ai/agent=my-agent
```

### Tracing

Agents can integrate with OpenTelemetry:
```yaml
env:
- name: OTEL_ENDPOINT
  value: "http://otel-collector:4317"
- name: OTEL_SERVICE_NAME
  value: "my-agent"
```

### Status Conditions

Operator maintains conditions on resources:
```yaml
status:
  conditions:
  - type: Ready
    status: "True"
    reason: DeploymentAvailable
  - type: ModelResolved
    status: "True"
    reason: ModelFound
```

## Best Practices

1. **Separation of Concerns**: Keep providers, models, and agents in appropriate namespaces
2. **Reuse Resources**: Share ModelProviders and common tools across agents
3. **Security First**: Use secrets, RBAC, network policies
4. **Resource Limits**: Always set CPU/memory limits
5. **Health Checks**: Configure liveness and readiness probes
6. **High Availability**: Use multiple replicas with anti-affinity
7. **Monitoring**: Integrate with observability stack
8. **Version Control**: Keep manifests in Git
9. **Environment Isolation**: Use namespaces for dev/staging/prod
10. **Cost Management**: Monitor LLM usage and set appropriate limits
