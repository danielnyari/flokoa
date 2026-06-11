# Getting Started with Flokoa

Flokoa is a Kubernetes-native platform for deploying and managing AI agents. It provides Custom Resource Definitions (CRDs) that allow you to declaratively define and deploy AI agents in your Kubernetes cluster.

## Overview

Flokoa consists of several key components:

- **Agent**: The main resource representing an AI agent deployment
- **ModelProvider**: Configuration for connecting to LLM providers (OpenAI, Anthropic, Google, AWS Bedrock)
- **Model**: Definition of a specific LLM model with its parameters
- **AgentTool**: External tools that agents can use to interact with APIs and services

## Quick Start

### Prerequisites

- A Kubernetes cluster (1.25+)
- kubectl configured to access your cluster
- The Flokoa operator installed in your cluster

### Install the Operator

```bash
# Apply the Flokoa operator manifests
kubectl apply -f https://github.com/danielnyari/flokoa/releases/latest/download/install.yaml

# Verify the operator is running
kubectl get pods -n flokoa-system
```

### Deploy Your First Agent

Create a minimal agent configuration:

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: my-first-agent
spec:
  card:
    name: my-first-agent
    description: "Answers questions"
    version: "0.1.0"
    skills:
      - id: assistant
        name: Assistant
        description: "General-purpose assistant"
        tags: [assistant]
  spec:
    model: openai:gpt-5-mini
    instructions:
      - "You are a helpful assistant."
```

No container image required: the operator compiles the spec and runs it on
the generic runner (the OpenAI API key comes from a referenced Model +
ModelProvider, or `runtime.env` for quick experiments).

Apply the configuration:

```bash
kubectl apply -f my-first-agent.yaml

# Check the agent status
kubectl get agents
kubectl describe agent my-first-agent
```

## Core Concepts

### Agent Lifecycle

1. **Pending**: Agent is being scheduled and pods are being created
2. **Running**: Agent pods are running and the service is available
3. **Failed**: Agent deployment failed (check status conditions for details)

### Runtime Backends

Flokoa supports two runtime backends:

- **standard** - Creates a Kubernetes Deployment using your own container image, along with a Service to expose it. Manages pod lifecycle, scaling, and health checks.
- **template** - Uses a generic runtime image managed by the operator. The agent's behavior is defined entirely in the CR via instructions and output schema.

### Framework

flokoa targets **pydantic-ai** exclusively. Declare it in your Agent spec
for observability:

```yaml
spec:
  framework: pydantic-ai
```

## Resource Organization

### Namespaces

All Flokoa resources are namespaced. You can organize your agents, models, and tools into different namespaces:

```bash
# Create a namespace for your agents
kubectl create namespace ai-agents

# Deploy resources to that namespace
kubectl apply -f agent.yaml -n ai-agents
```

### Resource References

Resources can reference other resources within the same namespace or across namespaces:

```yaml
spec:
  modelRef:
    name: gpt-4o-model
    namespace: shared-models  # Optional, defaults to agent's namespace
```

## Next Steps

- Learn about [Agents](agent.md) - How to deploy and configure AI agents
- Learn about [ModelProviders](modelprovider.md) - How to connect to LLM providers
- Learn about [Models](model.md) - How to configure LLM models
- Learn about [AgentTools](agenttool.md) - How to give agents access to external APIs

## Common Patterns

### Development vs Production

For development:
```yaml
spec:
  runtime:
    replicas: 1
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
```

For production:
```yaml
spec:
  runtime:
    replicas: 3
    resources:
      requests:
        cpu: "500m"
        memory: "512Mi"
      limits:
        cpu: "2000m"
        memory: "2Gi"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
```

### Secrets Management

Store sensitive data in Kubernetes secrets:

```bash
# Create a secret for API keys
kubectl create secret generic agent-secrets \
  --from-literal=api-key=your-api-key-here
```

Reference in your agent:
```yaml
env:
- name: API_KEY
  valueFrom:
    secretKeyRef:
      name: agent-secrets
      key: api-key
```

## Troubleshooting

### Check Agent Status

```bash
# View agent status
kubectl get agents
kubectl describe agent <agent-name>

# View agent pods
kubectl get pods -l flokoa.ai/agent=<agent-name>

# View agent logs
kubectl logs -l flokoa.ai/agent=<agent-name>
```

### Common Issues

**Agent stuck in Pending**
- Check if the container image is accessible
- Verify resource requests can be satisfied by the cluster
- Check for image pull secrets if using private registries

**Agent pods crashing**
- Check pod logs: `kubectl logs <pod-name>`
- Verify environment variables and secrets are configured correctly
- Check resource limits aren't too restrictive

**Service not accessible**
- Verify the agent is in Running phase
- Check service configuration: `kubectl get svc -l flokoa.ai/agent=<agent-name>`
- Check network policies and ingress configuration
