# Flokoa Quick Reference

A quick reference guide for common tasks and patterns in Flokoa.

## Resource Overview

| Resource | Purpose | Required? | Example Use |
|----------|---------|-----------|-------------|
| **Agent** | Deploy AI agent | Yes | Customer service bot, code assistant |
| **ModelProvider** | Connect to LLM | Optional* | OpenAI, Anthropic, Google credentials |
| **Model** | Configure LLM | Optional* | GPT-4o with specific parameters |
| **AgentTool** | External APIs | Optional | Weather API, database queries |

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
  -p='[{"op": "replace", "path": "/spec/runtime/spec/replicas", "value": 5}]'

# Scale to zero
kubectl patch agent my-agent --type='json' \
  -p='[{"op": "replace", "path": "/spec/runtime/spec/replicas", "value": 0}]'
```

### Updates

```bash
# Update image
kubectl set image agent/my-agent agent=new-image:v2.0.0

# Patch configuration
kubectl patch agent my-agent --type='json' \
  -p='[{"op": "replace", "path": "/spec/runtime/spec/container/env/0/value", "value": "new-value"}]'

# Edit directly
kubectl edit agent my-agent
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
    name: simple
    description: "Answers questions"
    version: "0.1.0"
    skills:
      - {id: assistant, name: Assistant, description: "Assistant", tags: [assistant]}
  spec:
    model: openai:gpt-5-mini
    instructions: ["You are a helpful assistant."]
```

### Agent with Model

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: ai-agent
spec:
  card:
    name: ai-agent
    description: "Shared-model agent"
    version: "0.1.0"
    skills:
      - {id: assistant, name: Assistant, description: "Assistant", tags: [assistant]}
  modelRef:
    name: gpt-4o-model
  instructionRefs:
    - name: assistant-policy
```

### Agent with Tools

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: tool-agent
spec:
  card:
    name: tool-agent
    description: "Agent with MCP tools"
    version: "0.1.0"
    skills:
      - {id: tools, name: Tools, description: "Uses tools", tags: [tools]}
  modelRef:
    name: gpt-4o-model
  tools:
    - name: weather-api        # AgentTool: declarative MCP endpoint
  spec:
    capabilities:
      - name: WebSearch        # native pydantic-ai capability
```

### Production Agent

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: prod-agent
spec:
  card:
    name: prod-agent
    description: "Production agent"
    version: "1.0.0"
    skills:
      - {id: assistant, name: Assistant, description: "Assistant", tags: [assistant]}
  modelRef:
    name: gpt-4o-model
  tools:
    - name: api-tool
  runtime:
    replicas: 3
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

```bash
kubectl create secret generic anthropic-creds \
  --from-literal=api-key=sk-ant-xxx
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
  settings:
    temperature: "0.7"
    maxTokens: 4096
```

### Deterministic (Code, Math)

```yaml
settings:
  temperature: "0.2"
  maxTokens: 8192
  topP: "0.1"
```

### Creative (Writing)

```yaml
settings:
  temperature: "1.2"
  maxTokens: 8192
  presencePenalty: "0.6"
  frequencyPenalty: "0.3"
```

### Cost Optimized

```yaml
spec:
  model: "gpt-4o-mini"  # Use cheaper model
  settings:
    maxTokens: 2048     # Limit output
    extra:
      openai_prompt_cache_key: "my-cache"  # Enable caching
```

## Tool Patterns

### External MCP Server

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: external-api
spec:
  type: mcp
  description: "Call the external MCP server"
  url: "https://mcp.external.com/mcp"
```

### Internal Service

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: internal-service
spec:
  type: mcp
  description: "Call the internal MCP service"
  serviceRef:
    name: my-service
    namespace: backend
    port: 8080
  path: /mcp
```

### Secret-Backed Auth Header

```yaml
spec:
  type: mcp
  url: "https://mcp.external.com/mcp"
  headerSecrets:
    - name: Authorization
      secretRef:
        name: api-credentials
        key: token
  toolPrefix: ext
  allowedTools: [search]
```

## Namespace Patterns

### Single Namespace (Simple)

```yaml
# Everything in default namespace
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: openai
  namespace: default
---
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: gpt-4o
  namespace: default
spec:
  providerRef:
    name: openai  # Same namespace
---
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: my-agent
  namespace: default
spec:
  model:
    name: gpt-4o  # Same namespace
```

### Shared Resources

```yaml
# In shared-resources namespace
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: openai
  namespace: shared-resources
---
# In app namespace
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: gpt-4o
  namespace: my-app
spec:
  providerRef:
    name: openai
    namespace: shared-resources  # Cross-namespace ref
---
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: my-agent
  namespace: my-app
spec:
  model:
    name: gpt-4o  # Same namespace
```

## Troubleshooting Guide

| Problem | Check | Solution |
|---------|-------|----------|
| Agent pending | `kubectl describe agent <name>` | Check image, resources, secrets |
| Pods crashing | `kubectl logs <pod>` | Check env vars, health probes |
| Model not found | `kubectl get model <name>` | Verify model exists, check namespace |
| Provider not ready | `kubectl describe modelprovider <name>` | Check secret exists, valid API key |
| Tool not working | `kubectl logs -l flokoa.ai/agent=<name>` | Test endpoint, check schema |
| Connection timeout | Network policies, DNS | Increase timeout, check connectivity |
| High costs | Token usage | Lower maxTokens, use cheaper model |
| Slow responses | `kubectl top pods` | Scale up, optimize prompts |

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
- [ ] Test in staging first
- [ ] Monitor costs

### Security
- [ ] Never commit secrets
- [ ] Use Kubernetes secrets
- [ ] Apply RBAC policies
- [ ] Use network policies
- [ ] Run as non-root
- [ ] Read-only root filesystem
- [ ] Drop all capabilities
- [ ] Enable seccomp profile

## Quick Tips

1. **Test locally first**: Use `kubectl apply --dry-run=client` to validate syntax
2. **Use labels**: Organize resources with labels for easier management
3. **Version everything**: Tag images and track manifest versions in Git
4. **Start simple**: Begin with minimal config, add complexity as needed
5. **Monitor costs**: Track LLM token usage and set budgets
6. **Cache when possible**: Use provider caching features
7. **Scale gradually**: Start with 1 replica, scale up as needed
8. **Document configs**: Add annotations explaining non-obvious settings
9. **Test tools independently**: Verify tools work before giving to agents
10. **Review logs regularly**: Catch issues early

## Resource Limits Guide

### Small Agent (Development)
```yaml
resources:
  requests:
    cpu: "100m"
    memory: "128Mi"
  limits:
    cpu: "500m"
    memory: "512Mi"
```

### Medium Agent (Production)
```yaml
resources:
  requests:
    cpu: "500m"
    memory: "512Mi"
  limits:
    cpu: "2000m"
    memory: "2Gi"
```

### Large Agent (High Load)
```yaml
resources:
  requests:
    cpu: "1000m"
    memory: "1Gi"
  limits:
    cpu: "4000m"
    memory: "4Gi"
```

## Further Reading

- [Getting Started](getting-started.md)
- [Agent Documentation](agent.md)
- [Model Documentation](model.md)
- [ModelProvider Documentation](modelprovider.md)
- [AgentTool Documentation](agenttool.md)
- [Architecture Overview](architecture.md)
- [Examples](examples/)
