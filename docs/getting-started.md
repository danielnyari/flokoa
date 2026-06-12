# Getting Started with Flokoa

Flokoa is a Kubernetes-native platform for deploying and managing AI agents. It provides Custom Resource Definitions (CRDs) that allow you to declaratively define and deploy AI agents in your Kubernetes cluster.

## Overview

Flokoa consists of several key components:

- **Agent**: the composition root — references Model/Instruction/AgentTool CRs plus an optional inline AgentSpec fragment, which the controller compiles into one resolved pydantic-ai AgentSpec
- **Model**: a named, shareable model configuration (identifier + typed settings) → AgentSpec `model`/`model_settings`
- **ModelProvider**: the provider connection behind a Model (OpenAI, Anthropic, Google, AWS Bedrock)
- **AgentTool**: a declarative MCP endpoint (`url` or `serviceRef`+`path`) that compiles to an MCP capability
- **Instruction**: versioned, shareable system-prompt blocks
- **AgentTrigger**: event-driven agent invocation via Argo Events
- **AgentWorkflow**: frozen, template-only A2A composition between deployed agents

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

`status.url` is the agent's published (flokoa-owned) endpoint — always invoke via this URL, never
the internal `<agent>-runtime` Service. Watch the `SpecValid` condition (compile-time schema
validation) and `status.specHash` (drift detection) alongside the phase; a failed compile shows
`SpecValid=False` and the last good generation keeps running.

### The generic runner

Most agents need no container image. The controller compiles the Agent's referenced CRs and inline
fragment into one resolved pydantic-ai AgentSpec, validates it against the runner's pinned AgentSpec
JSON Schema, and delivers it as the `<agent>-agent-spec` ConfigMap. The **generic runner** image
hydrates that spec at startup (installing any referenced capabilities), builds the agent via
`Agent.from_spec()`, and serves it over A2A. Building a custom image via `spec.runtime.image` is the
escape hatch — see the [runtime contract](reference/runtime-contract.md).

### Framework

flokoa targets **pydantic-ai** exclusively — it is the only framework and is not configurable.
Agents compile to a pydantic-ai AgentSpec; there is no framework selector.

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
```

> Health probes for the generic runner are operator-managed — there is no per-Agent probe override.
> Pod-level scheduling (node selectors, tolerations, affinity) is set via `spec.runtime`'s inlined
> deployment overrides.

### Secrets Management

Store sensitive data in Kubernetes secrets:

```bash
# Create a secret for API keys
kubectl create secret generic agent-secrets \
  --from-literal=api-key=your-api-key-here
```

Reference it from the agent via `secretRefs` and a `${secret:NAME}` placeholder:
```yaml
spec:
  secretRefs:
    api-key:
      name: agent-secrets
      key: api-key
  # ...then use the value elsewhere in the spec as ${secret:api-key}
```

The placeholder is resolved **in the runner at hydration time** (projected as `FLOKOA_SECRET_*`);
the secret value is never written into the compiled-spec ConfigMap.

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
- Check `kubectl describe agent <name>` for a `SpecValid=False` condition (e.g. an unknown `runnerVersion` with no embedded schema, or a fragment that fails the pinned AgentSpec schema) — when compilation fails, no Deployment update happens
- Verify resource requests can be satisfied by the cluster
- For custom-image agents (`spec.runtime.image`): check the image is accessible and add image pull secrets for private registries

**Agent pods crashing**
- Check pod logs: `kubectl logs <pod-name>`
- Verify environment variables and secrets are configured correctly
- Check resource limits aren't too restrictive

**Service not accessible**
- Verify the agent is in Running phase
- Read the published endpoint from `kubectl get agent <name> -o jsonpath='{.status.url}'` and call that; the `<agent>-runtime` workload Service is internal and not part of the public contract
- Check network policies and ingress configuration
