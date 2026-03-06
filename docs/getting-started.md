# Getting Started with Flokoa

Flokoa is a Kubernetes-native platform for deploying and managing AI agents. It provides Custom Resource Definitions (CRDs) that allow you to declaratively define and deploy AI agents in your Kubernetes cluster.

## Overview

Flokoa consists of several key components:

- **Agent**: The main resource representing an AI agent deployment
- **ModelProvider**: Configuration for connecting to LLM providers (OpenAI, Anthropic, Google, AWS Bedrock)
- **Model**: Definition of a specific LLM model with its parameters
- **AgentTool**: External tools that agents can use to interact with APIs and services
- **Instruction**: System prompt management, shareable across agents
- **AgentWorkflow**: Multi-agent workflows compiled to Argo Workflows

Additionally, the **Python SDK** lets you build and run agents locally with a CLI.

## Quick Start

### Prerequisites

- A Kubernetes cluster (1.25+)
- kubectl configured to access your cluster
- The Flokoa operator installed in your cluster

### Install the Operator

```bash
# Install CRDs and deploy the operator via Helm
helm install flokoa oci://ghcr.io/danielnyari/flokoa/charts/flokoa \
  --namespace flokoa-system --create-namespace

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
    name: "My First Agent"
    description: "A simple example agent"
    version: "1.0.0"
    skills:
      - id: "hello"
        name: "Hello"
        description: "Responds to greetings"
        tags: ["demo"]
  runtime:
    type: standard
    standard:
      container:
        name: agent
        image: ghcr.io/example/simple-agent:latest
        ports:
        - containerPort: 8080
          name: http
```

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
- **template** - Uses a generic runtime image managed by the operator. The agent's behavior is defined entirely in the CR via instructions and output schema. No custom container image needed.

### Framework Declaration

You can declare which AI framework your agent uses for observability:

- `pydantic-ai` - Pydantic AI framework
- `langchain` - LangChain framework
- `crewai` - CrewAI framework
- `marvin` - Marvin AI framework
- `autogen` - Microsoft AutoGen
- `a2a` - Agent-to-Agent protocol

### A2A Protocol

Flokoa uses the A2A (Agent-to-Agent) protocol across the platform:

- Agents expose A2A-compatible HTTP endpoints
- The `card` field on Agent CRs defines A2A metadata (skills, capabilities)
- The Argo Workflows executor plugin communicates with agents via A2A
- The Python SDK serves agents via A2A endpoints using FastAPI

### Instructions

System prompts can be managed as first-class resources:

```yaml
# Inline instruction in Agent spec
spec:
  instruction:
    template: "You are a helpful customer service agent."

# Or reference a shared Instruction resource
spec:
  instruction:
    instructionRef:
      name: shared-prompt
      namespace: shared-resources
```

### AgentWorkflows

Orchestrate multiple agents with declarative workflows:

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentWorkflow
metadata:
  name: research-pipeline
spec:
  tasks:
    - name: research
      agent:
        name: researcher-agent
        text: "Research the topic: {{params.topic}}"
    - name: summarize
      agent:
        name: writer-agent
        text: "Summarize this research: {{tasks.research.output}}"
      dependsOn: [research]
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
  model:
    name: gpt-4o-model
    namespace: shared-models  # Optional, defaults to agent's namespace
```

## Python SDK

Run agents locally without a Kubernetes cluster:

```bash
# Install the SDK
pip install flokoa[pydantic-ai]

# Run an agent
flokoa run -m my_module:my_agent --framework pydantic-ai --port 8000
```

See the [Python SDK documentation](python-sdk.md) for details.

## Next Steps

- Learn about [Agents](agent.md) - How to deploy and configure AI agents
- Learn about [ModelProviders](modelprovider.md) - How to connect to LLM providers
- Learn about [Models](model.md) - How to configure LLM models
- Learn about [AgentTools](agenttool.md) - How to give agents access to external APIs
- Learn about [Instructions](instruction.md) - How to manage system prompts
- Learn about [AgentWorkflows](agentworkflow.md) - How to orchestrate multi-agent pipelines
- Learn about the [Python SDK](python-sdk.md) - How to build and run agents locally

## Common Patterns

### Development vs Production

For development:
```yaml
spec:
  runtime:
    standard:
      replicas: 1
      container:
        resources:
          requests:
            cpu: "100m"
            memory: "128Mi"
```

For production:
```yaml
spec:
  runtime:
    standard:
      replicas: 3
      container:
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
- Check image pull secrets if using private registries

**Agent pods crashing**
- Check pod logs: `kubectl logs <pod-name>`
- Verify environment variables and secrets are configured correctly
- Check resource limits aren't too restrictive

**Service not accessible**
- Verify the agent is in Running phase
- Check service configuration: `kubectl get svc -l flokoa.ai/agent=<agent-name>`
- Check network policies and ingress configuration
