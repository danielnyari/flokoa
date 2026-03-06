# Flokoa Example Configurations

This directory contains example Custom Resources (CRs) for the Flokoa operator.

## Agent Examples

### [agent/minimal-agent.yaml](agent/minimal-agent.yaml)
The absolute minimum configuration required to deploy an agent.

**Use when:** You want to quickly test or deploy a simple agent.

### [agent/basic-agent.yaml](agent/basic-agent.yaml)
A practical production-ready configuration with replicas, resources, and health checks.

**Use when:** You need a solid baseline for most production deployments.

### [agent/advanced-agent.yaml](agent/advanced-agent.yaml)
A comprehensive configuration showcasing all features: service accounts, volumes, security contexts, scheduling.

**Use when:** You need fine-grained control over scheduling, security, and observability.

## ModelProvider Examples

- [modelprovider/openai.yaml](modelprovider/openai.yaml) - OpenAI
- [modelprovider/anthropic.yaml](modelprovider/anthropic.yaml) - Anthropic
- [modelprovider/google.yaml](modelprovider/google.yaml) - Google Gemini / Vertex AI
- [modelprovider/bedrock.yaml](modelprovider/bedrock.yaml) - AWS Bedrock
- [modelprovider/azure-openai.yaml](modelprovider/azure-openai.yaml) - Azure OpenAI
- [modelprovider/custom.yaml](modelprovider/custom.yaml) - Custom/self-hosted

## Model Examples

- [model/gpt-4o.yaml](model/gpt-4o.yaml) - GPT-4 Omni
- [model/gpt-4o-mini.yaml](model/gpt-4o-mini.yaml) - GPT-4 Omni Mini
- [model/o1.yaml](model/o1.yaml) - OpenAI o1 reasoning model
- [model/claude-sonnet-4.yaml](model/claude-sonnet-4.yaml) - Claude Sonnet 4
- [model/claude-thinking.yaml](model/claude-thinking.yaml) - Claude with extended thinking
- [model/gemini-2-flash.yaml](model/gemini-2-flash.yaml) - Gemini 2.0 Flash
- [model/gpt-4o-code.yaml](model/gpt-4o-code.yaml) - Optimized for code generation
- [model/gpt-4o-creative.yaml](model/gpt-4o-creative.yaml) - Optimized for creative writing

## AgentTool Examples

- [agenttool/weather-api.yaml](agenttool/weather-api.yaml) - External API
- [agenttool/create-ticket.yaml](agenttool/create-ticket.yaml) - POST request
- [agenttool/inventory-check.yaml](agenttool/inventory-check.yaml) - Internal service
- [agenttool/database-query.yaml](agenttool/database-query.yaml) - Database queries
- [agenttool/github-api.yaml](agenttool/github-api.yaml) - OpenAPI specification
- [agenttool/send-email.yaml](agenttool/send-email.yaml) - Email notifications
- [agenttool/create-order.yaml](agenttool/create-order.yaml) - Complex schemas
- [agenttool/search-kb.yaml](agenttool/search-kb.yaml) - Knowledge base search

## Instruction Examples

- [instruction/basic-instruction.yaml](instruction/basic-instruction.yaml) - Simple system prompt
- [instruction/shared-instruction.yaml](instruction/shared-instruction.yaml) - Shared across agents

## AgentWorkflow Examples

- [agentworkflow/simple-workflow.yaml](agentworkflow/simple-workflow.yaml) - Simple sequential workflow
- [agentworkflow/multi-agent-workflow.yaml](agentworkflow/multi-agent-workflow.yaml) - Multi-agent DAG workflow

## Complete Examples

- [complete-example.yaml](complete-example.yaml) - End-to-end customer service agent with all resources

## Applying Examples

```bash
# Apply directly
kubectl apply -f docs/examples/agent/basic-agent.yaml

# Apply all examples in a directory
kubectl apply -f docs/examples/modelprovider/
```

## Field Reference

### Agent Required Fields
- `spec.card` - A2A agent metadata (name, description, version, skills)
- `spec.runtime.type` - Runtime backend type (`standard` or `template`)
- `spec.runtime.standard.container` - Container spec (for standard runtime)

### Agent Common Optional Fields
- `spec.framework` - Framework type (pydantic-ai, langchain, crewai, marvin, autogen, a2a)
- `spec.model` - Reference to a Model resource
- `spec.instruction` - System prompt (inline template or instructionRef)
- `spec.tools` - Tool references or inline definitions
- `spec.runtime.standard.replicas` - Number of pod replicas (default: 1)
- `spec.runtime.standard.volumes` - Pod volumes
- `spec.runtime.standard.serviceAccountName` - Service account for RBAC
- `spec.runtime.standard.securityContext` - Pod security context
- `spec.runtime.standard.nodeSelector` - Node selection constraints
- `spec.runtime.standard.tolerations` - Pod tolerations
- `spec.runtime.standard.affinity` - Pod affinity rules

## Runtime Types

- **`standard`** - Deploys agents using standard Kubernetes Deployments and Services. The user provides their own container image.
- **`template`** - Uses a generic runtime image managed by the operator. The agent's behavior is defined via `spec.instruction` and `spec.runtime.template.config.outputSchema`.

## Tips

1. **Start Simple**: Begin with `agent/minimal-agent.yaml` and add fields as needed
2. **Set Resource Limits**: Always define CPU/memory limits to prevent resource contention
3. **Use Health Checks**: Liveness and readiness probes ensure reliable deployments
4. **Security First**: Use `securityContext` to run containers as non-root
5. **High Availability**: Use `replicas > 1` with anti-affinity rules for production
6. **Share Instructions**: Use Instruction CRDs for reusable system prompts
