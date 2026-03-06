---
icon: lucide/rocket
---

# Flokoa Documentation

Welcome to the Flokoa documentation. Flokoa is a Kubernetes-native platform for deploying and managing AI agents.

## Getting Started

New to Flokoa? Start here:

1. **[Getting Started Guide](getting-started.md)** - Installation, quick start, and core concepts
2. **[Architecture Overview](architecture.md)** - How Flokoa components work together
3. **[Python SDK](python-sdk.md)** - Build and run agents locally with the CLI and SDK

## Resource Documentation

Learn about each Custom Resource Definition (CRD):

- **[Agent](agent.md)** - Deploy and configure AI agents
- **[ModelProvider](modelprovider.md)** - Connect to LLM providers (OpenAI, Anthropic, Google, Bedrock)
- **[Model](model.md)** - Configure LLM models with parameters
- **[AgentTool](agenttool.md)** - Give agents access to external APIs and services
- **[Instruction](instruction.md)** - Manage and share system prompts
- **[AgentWorkflow](agentworkflow.md)** - Orchestrate multi-agent workflows (compiled to Argo Workflows)

## Examples

Browse example configurations in the [`examples/`](examples/) directory:

### Agent Examples
- [`agent/minimal-agent.yaml`](examples/agent/minimal-agent.yaml) - Minimal configuration
- [`agent/basic-agent.yaml`](examples/agent/basic-agent.yaml) - Production-ready baseline
- [`agent/advanced-agent.yaml`](examples/agent/advanced-agent.yaml) - Advanced features

### ModelProvider Examples
- [`modelprovider/openai.yaml`](examples/modelprovider/openai.yaml) - OpenAI configuration
- [`modelprovider/anthropic.yaml`](examples/modelprovider/anthropic.yaml) - Anthropic/Claude configuration
- [`modelprovider/google.yaml`](examples/modelprovider/google.yaml) - Google Gemini configuration
- [`modelprovider/bedrock.yaml`](examples/modelprovider/bedrock.yaml) - AWS Bedrock configuration
- [`modelprovider/azure-openai.yaml`](examples/modelprovider/azure-openai.yaml) - Azure OpenAI
- [`modelprovider/custom.yaml`](examples/modelprovider/custom.yaml) - Custom/self-hosted LLM

### Model Examples
- [`model/gpt-4o.yaml`](examples/model/gpt-4o.yaml) - GPT-4 Omni
- [`model/gpt-4o-mini.yaml`](examples/model/gpt-4o-mini.yaml) - GPT-4 Omni Mini
- [`model/o1.yaml`](examples/model/o1.yaml) - OpenAI o1 reasoning model
- [`model/claude-sonnet-4.yaml`](examples/model/claude-sonnet-4.yaml) - Claude Sonnet 4
- [`model/claude-thinking.yaml`](examples/model/claude-thinking.yaml) - Claude with extended thinking
- [`model/gemini-2-flash.yaml`](examples/model/gemini-2-flash.yaml) - Gemini 2.0 Flash
- [`model/gpt-4o-code.yaml`](examples/model/gpt-4o-code.yaml) - Optimized for code generation
- [`model/gpt-4o-creative.yaml`](examples/model/gpt-4o-creative.yaml) - Optimized for creative writing

### AgentTool Examples
- [`agenttool/weather-api.yaml`](examples/agenttool/weather-api.yaml) - Simple GET request
- [`agenttool/create-ticket.yaml`](examples/agenttool/create-ticket.yaml) - POST request to create resources
- [`agenttool/inventory-check.yaml`](examples/agenttool/inventory-check.yaml) - Internal Kubernetes service
- [`agenttool/database-query.yaml`](examples/agenttool/database-query.yaml) - Database queries via service
- [`agenttool/github-api.yaml`](examples/agenttool/github-api.yaml) - Using OpenAPI specification
- [`agenttool/send-email.yaml`](examples/agenttool/send-email.yaml) - Send email notifications
- [`agenttool/create-order.yaml`](examples/agenttool/create-order.yaml) - Complex nested schemas
- [`agenttool/search-kb.yaml`](examples/agenttool/search-kb.yaml) - Search knowledge base

### Instruction Examples
- [`instruction/basic-instruction.yaml`](examples/instruction/basic-instruction.yaml) - Simple system prompt
- [`instruction/shared-instruction.yaml`](examples/instruction/shared-instruction.yaml) - Shared across agents

### AgentWorkflow Examples
- [`agentworkflow/simple-workflow.yaml`](examples/agentworkflow/simple-workflow.yaml) - Simple sequential workflow
- [`agentworkflow/multi-agent-workflow.yaml`](examples/agentworkflow/multi-agent-workflow.yaml) - Multi-agent DAG workflow

### Complete Examples
- [`complete-example.yaml`](examples/complete-example.yaml) - End-to-end customer service agent with all resources

## Quick Reference

### Create an Agent

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: my-agent
spec:
  runtime:
    type: standard
    standard:
      container:
        name: agent
        image: your-agent:v1.0.0
        ports:
        - containerPort: 8080
```

### Reference a Model

```yaml
spec:
  model:
    name: gpt-4o-model
    namespace: shared-models  # optional
```

### Add an Instruction

```yaml
spec:
  instruction:
    template: |
      You are a helpful customer service agent.
      Always be polite and professional.
```

### Add Tools

```yaml
spec:
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
```

## Common Patterns

### Development Setup

```yaml
# 1. Create provider with dev credentials
# 2. Create model with development settings
# 3. Deploy agent with single replica
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

### Production Setup

```yaml
# 1. Use production provider credentials
# 2. Configure model with production parameters
# 3. Deploy agent with high availability
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
        livenessProbe: {...}
        readinessProbe: {...}
      affinity:
        podAntiAffinity: {...}
```

### Shared Resources

```yaml
# Create in shared namespace
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: openai-provider
  namespace: shared-resources
---
# Reference from any namespace
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: my-model
  namespace: my-app
spec:
  providerRef:
    name: openai-provider
    namespace: shared-resources
```

## Operations

### View Resources

```bash
# List all resources
kubectl get agents
kubectl get models
kubectl get modelproviders
kubectl get agenttools
kubectl get instructions
kubectl get agentworkflows

# Get detailed information
kubectl describe agent my-agent
kubectl get agent my-agent -o yaml

# Watch for changes
kubectl get agents -w
```

### Update Resources

```bash
# Edit directly
kubectl edit agent my-agent

# Patch specific fields
kubectl patch agent my-agent --type='json' \
  -p='[{"op": "replace", "path": "/spec/runtime/standard/replicas", "value": 5}]'

# Apply updated manifest
kubectl apply -f agent.yaml
```

### Debug

```bash
# View logs
kubectl logs -l flokoa.ai/agent=my-agent
kubectl logs -l flokoa.ai/agent=my-agent -f --all-containers

# Execute commands
kubectl exec -it <pod-name> -- /bin/sh

# Check events
kubectl get events --sort-by='.lastTimestamp'

# Check agent status
kubectl get agent my-agent -o jsonpath='{.status.phase}'
```

## Python SDK

Run agents locally without a Kubernetes cluster:

```bash
# Install the SDK
pip install flokoa[pydantic-ai]

# Run an agent
flokoa run -m my_module:my_agent --framework pydantic-ai
```

See the [Python SDK documentation](python-sdk.md) for full details.

## Troubleshooting

### Agent not starting

1. Check pod status: `kubectl get pods -l flokoa.ai/agent=<name>`
2. View pod events: `kubectl describe pod <pod-name>`
3. Check logs: `kubectl logs <pod-name>`
4. Verify image exists and is accessible
5. Check resource requests/limits

### Model not found

1. Verify model exists: `kubectl get model <name>`
2. Check namespace (use `-n` flag or specify in spec)
3. Verify ModelProvider exists and is ready
4. Check secret with API key exists

### Tool not working

1. Test endpoint manually (port-forward if internal service)
2. Verify schema matches API expectations
3. Check authentication/API keys
4. Review tool timeout settings
5. Check agent logs for tool errors

### Connection timeouts

1. Increase timeout values in spec
2. Check network policies
3. Verify DNS resolution
4. Check firewall rules

## Best Practices

1. **Start simple** - Use minimal examples first, add complexity as needed
2. **Resource limits** - Always set CPU/memory limits
3. **Health checks** - Configure liveness and readiness probes
4. **Security** - Use secrets, RBAC, security contexts
5. **High availability** - Use multiple replicas with anti-affinity
6. **Monitoring** - Integrate metrics and logging
7. **Version control** - Keep manifests in Git
8. **Testing** - Test in development before production
9. **Documentation** - Document custom configurations
10. **Cost awareness** - Monitor LLM usage and costs

## Additional Resources

- [GitHub Repository](https://github.com/danielnyari/flokoa)
- [Quick Reference](quick-reference.md)

## Contributing

Found an issue or want to improve the documentation? Please open an issue or pull request on GitHub.

## License

Flokoa is licensed under the Apache License 2.0.
