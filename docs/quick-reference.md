# Flokoa Quick Reference

A quick reference guide for common tasks and patterns in Flokoa.

## Resource Overview

| Resource | Purpose | Required? | Example Use |
|----------|---------|-----------|-------------|
| **Agent** | Deploy AI agent | Yes | Customer service bot, code assistant |
| **ModelProvider** | Connect to LLM | Optional* | OpenAI, Anthropic, Google credentials |
| **Model** | Configure LLM | Optional* | GPT-4o with specific parameters |
| **AgentTool** | External APIs | Optional | Weather API, database queries |
| **Instruction** | System prompts | Optional | Shared behavior definitions |
| **AgentWorkflow** | Multi-agent pipelines | Optional | Research → summarize → review |

\* Not required if your agent doesn't use LLMs

## Common Commands

### Resource Management

```bash
# Create resources
kubectl apply -f agent.yaml
kubectl apply -f directory/

# View resources
kubectl get agents
kubectl get models
kubectl get modelproviders
kubectl get agenttools
kubectl get instructions
kubectl get agentworkflows    # shortName: awf

# Detailed information
kubectl describe agent my-agent
kubectl get agent my-agent -o yaml

# Watch for changes
kubectl get agents -w

# Delete resources
kubectl delete agent my-agent
```

### Debugging

```bash
# Check agent status
kubectl get agent my-agent -o jsonpath='{.status.phase}'

# View logs
kubectl logs -l flokoa.ai/agent=my-agent
kubectl logs -l flokoa.ai/agent=my-agent -f --all-containers

# Get pods
kubectl get pods -l flokoa.ai/agent=my-agent

# Execute in pod
kubectl exec -it <pod-name> -- /bin/sh

# Check events
kubectl get events --field-selector involvedObject.name=my-agent

# Port forward for testing
kubectl port-forward svc/my-agent 8080:8080
```

### Scaling

```bash
# Scale replicas
kubectl patch agent my-agent --type='json' \
  -p='[{"op": "replace", "path": "/spec/runtime/standard/replicas", "value": 5}]'

# Scale to zero
kubectl patch agent my-agent --type='json' \
  -p='[{"op": "replace", "path": "/spec/runtime/standard/replicas", "value": 0}]'
```

## Resource Patterns

### Minimal Agent

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: simple
spec:
  card:
    name: "Simple Agent"
    description: "A simple agent"
    version: "1.0.0"
    skills:
      - id: "default"
        name: "Default"
        description: "Default skill"
        tags: ["general"]
  runtime:
    type: standard
    standard:
      container:
        name: agent
        image: my-agent:latest
        ports:
        - containerPort: 8080
```

### Agent with Model and Instruction

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: ai-agent
spec:
  card:
    name: "AI Agent"
    description: "An AI-powered agent"
    version: "1.0.0"
    skills:
      - id: "assist"
        name: "Assist"
        description: "General assistance"
        tags: ["ai"]
  model:
    name: gpt-4o-model
  instruction:
    template: "You are a helpful assistant."
  runtime:
    type: standard
    standard:
      container:
        name: agent
        image: my-agent:latest
        ports:
        - containerPort: 8080
```

### Agent with Tools

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: tool-agent
spec:
  card:
    name: "Tool Agent"
    description: "Agent with tools"
    version: "1.0.0"
    skills:
      - id: "search"
        name: "Search"
        description: "Search and retrieve data"
        tags: ["tools"]
  tools:
    - toolRef:
        name: weather-api
    - name: inline-tool
      template:
        type: openapi
        description: "Call the example API"
        openApi:
          url: "https://api.example.com"
          openApiSchema:
            endpointPath: "/openapi.json"
  runtime:
    type: standard
    standard:
      container:
        name: agent
        image: my-agent:latest
        ports:
        - containerPort: 8080
```

### Production Agent

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: prod-agent
spec:
  card:
    name: "Production Agent"
    description: "Production-grade agent"
    version: "1.0.0"
    skills:
      - id: "serve"
        name: "Serve"
        description: "Production service"
        tags: ["production"]
  model:
    name: gpt-4o-model
  instruction:
    instructionRef:
      name: prod-prompt
  tools:
    - toolRef:
        name: api-tool
  runtime:
    type: standard
    standard:
      replicas: 3
      container:
        name: agent
        image: my-agent:v1.0.0
        ports:
        - containerPort: 8080
        env:
        - name: ENVIRONMENT
          value: production
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
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  flokoa.ai/agent: prod-agent
              topologyKey: topology.kubernetes.io/zone
```

## Provider Configurations

### OpenAI

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: openai
spec:
  apiKeySecretRef:
    name: openai-creds
    key: api-key
  openai: {}
```

```bash
kubectl create secret generic openai-creds \
  --from-literal=api-key=sk-proj-xxx
```

### Anthropic

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: anthropic
spec:
  apiKeySecretRef:
    name: anthropic-creds
    key: api-key
  anthropic: {}
```

### Google (API Key)

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: google
spec:
  apiKeySecretRef:
    name: google-creds
    key: api-key
  google: {}
```

### Custom/Self-Hosted

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: custom
spec:
  apiKeySecretRef:
    name: custom-creds
    key: api-key
  openai:
    baseURL: "https://my-llm.example.com/v1"
```

## Model Configurations

### Balanced (Default)

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: gpt-4o-balanced
spec:
  model: "gpt-4o"
  providerRef:
    name: openai
  parameters:
    temperature: "0.7"
    maxTokens: 4096
```

### Deterministic (Code, Math)

```yaml
parameters:
  temperature: "0.2"
  maxTokens: 8192
  topP: "0.1"
```

### Creative (Writing)

```yaml
parameters:
  temperature: "1.2"
  maxTokens: 8192
  presencePenalty: "0.6"
  frequencyPenalty: "0.3"
```

### Cost Optimized

```yaml
spec:
  model: "gpt-4o-mini"
  parameters:
    maxTokens: 2048
    openai:
      promptCacheKey: "my-cache"
```

## Instruction Patterns

### Inline

```yaml
spec:
  instruction:
    template: |
      You are a helpful customer service agent.
      Always be polite and professional.
```

### Shared Reference

```yaml
# Instruction resource
apiVersion: agent.flokoa.ai/v1alpha1
kind: Instruction
metadata:
  name: shared-prompt
  namespace: shared-resources
spec:
  content: "You are a helpful assistant."
---
# Agent referencing it
spec:
  instruction:
    instructionRef:
      name: shared-prompt
      namespace: shared-resources
```

## Workflow Patterns

### Simple Sequential

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentWorkflow
metadata:
  name: pipeline
spec:
  params:
    - name: topic
      value: "AI safety"
  tasks:
    - name: research
      agent:
        name: researcher
        text: "Research: {{params.topic}}"
    - name: summarize
      agent:
        name: writer
        text: "Summarize: {{tasks.research.output}}"
      dependsOn: [research]
```

## Tool Patterns

### External API

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: external-api
spec:
  type: openapi
  description: "Call external API"
  openApi:
    url: "https://api.external.com"
    openApiSchema:
      endpointPath: "/openapi.json"
```

### Internal Service

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: internal-service
spec:
  type: openapi
  description: "Call internal service"
  openApi:
    serviceRef:
      name: my-service
      namespace: backend
      port: 8080
    openApiSchema:
      endpointPath: "/openapi.json"
```

## Python SDK

```bash
# Install
pip install flokoa[pydantic-ai]

# Run an agent locally
flokoa run -m my_module:my_agent --framework pydantic-ai --port 8000
```

## Troubleshooting Guide

| Problem | Check | Solution |
|---------|-------|----------|
| Agent pending | `kubectl describe agent <name>` | Check image, resources, secrets |
| Pods crashing | `kubectl logs <pod>` | Check env vars, health probes |
| Model not found | `kubectl get model <name>` | Verify model exists, check namespace |
| Provider not ready | `kubectl describe modelprovider <name>` | Check secret exists, valid API key |
| Tool not working | `kubectl logs -l flokoa.ai/agent=<name>` | Test endpoint, check schema |
| Connection timeout | Network policies, DNS | Check connectivity |
| Workflow not ready | `kubectl describe awf <name>` | Check task names, agent refs |
| High costs | Token usage | Lower maxTokens, use cheaper model |

## Best Practices Checklist

### Development
- [ ] Start with minimal config
- [ ] Use single replica
- [ ] Set low resource limits
- [ ] Use cheaper models (gpt-4o-mini)
- [ ] Enable verbose logging

### Production
- [ ] Use 3+ replicas
- [ ] Set resource limits
- [ ] Configure health checks
- [ ] Use pod anti-affinity
- [ ] Enable monitoring/metrics
- [ ] Use security contexts
- [ ] Store secrets properly
- [ ] Version images (no :latest)
- [ ] Use Instruction CRDs for prompts
- [ ] Test in staging first

### Security
- [ ] Never commit secrets
- [ ] Use Kubernetes secrets
- [ ] Apply RBAC policies
- [ ] Use network policies
- [ ] Run as non-root
- [ ] Read-only root filesystem
- [ ] Drop all capabilities
- [ ] Enable seccomp profile

## Further Reading

- [Getting Started](getting-started.md)
- [Agent Documentation](agent.md)
- [Model Documentation](model.md)
- [ModelProvider Documentation](modelprovider.md)
- [AgentTool Documentation](agenttool.md)
- [Instruction Documentation](instruction.md)
- [AgentWorkflow Documentation](agentworkflow.md)
- [Python SDK](python-sdk.md)
- [Architecture Overview](architecture.md)
- [Examples](examples/)
