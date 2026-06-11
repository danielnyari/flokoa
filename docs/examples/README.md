# Agent CR Examples

This directory contains example Custom Resources (CRs) for the Flokoa Agent operator.

## Examples

### [minimal-agent.yaml](minimal-agent.yaml)
The absolute minimum: a card, an inline model, and instructions. No image,
no build — the operator compiles the spec and runs it on the generic runner.

**Use when:** You want to quickly test or deploy a simple agent.

### [basic-agent.yaml](basic-agent.yaml)
The composition shape: shared Model and Instruction resources, an MCP tool,
an inline fragment, and secret-backed placeholders. Rotate the Model CR and
every referencing agent recompiles and rolls.

**Use when:** You need the fleet-managed baseline for production deployments.

### [advanced-agent.yaml](advanced-agent.yaml)
The custom-image escape hatch plus scheduling overrides:
- `runtime.image` replacing the generic runner
- Structured output schema
- Node selectors and tolerations
- Security contexts (pod and container level)
- Image pull secrets
- Node scheduling (selectors, tolerations, affinity)
- Advanced health check configurations

**Use when:** You need fine-grained control over scheduling, security, and observability.

## Applying Examples

```bash
# Apply directly
kubectl apply -f docs/examples/basic-agent.yaml

# Or from the operator directory's samples
kubectl apply -f operator/config/samples/agent_v1alpha1_agent.yaml
```

## Viewing Agent Status

```bash
# List all agents
kubectl get agents

# Get detailed status
kubectl describe agent basic-agent

# View logs
kubectl logs -l flokoa.ai/agent=basic-agent
```

## Field Reference

### Required Fields
- `spec.card` - The published A2A agent card (name, description, version, skills)
- A model: either `spec.modelRef` (a Model resource) or `spec.spec.model`
  (an inline pydantic-ai model identifier like `openai:gpt-5-mini`)

### Composition Fields
- `spec.modelRef` - References a Model resource (inline `spec.spec.model` wins conflicts)
- `spec.instructionRefs` - Instruction resources, composed in order before inline instructions
- `spec.tools` - AgentTool resources (declarative MCP endpoints)
- `spec.spec` - Inline pydantic-ai AgentSpec fragment (instructions, modelSettings,
  outputSchema, native capabilities, extra passthrough)
- `spec.secretRefs` - Named secrets resolvable via `${secret:NAME}` placeholders

### Runtime Fields
- `spec.runtime.image` - Custom image escape hatch (default: the generic runner)
- `spec.runtime.runnerVersion` - Pins a runner release
- `spec.runtime.isolation` - Session isolation tier (`shared` today; `session` ships with the session router)
- `spec.runtime.replicas` - Number of pod replicas (default: 1)
- `spec.runtime.env` / `resources` / `serviceAccountName` / `securityContext` /
  `nodeSelector` / `tolerations` / `affinity` - Pod-level configuration

The compiled spec lands in the `<agent>-agent-spec` ConfigMap; `kubectl get
agent <name> -o jsonpath='{.status.specHash}'` shows the resolved-spec hash.
