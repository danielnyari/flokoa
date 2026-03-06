# Instruction Resource

The `Instruction` resource defines a system prompt that can be shared across multiple AI agents, stored as a Kubernetes ConfigMap.

## Overview

An Instruction:
- Represents a reusable system prompt for AI agents
- Contains the prompt text in a single `content` field
- Creates a ConfigMap containing the instruction text (managed by the controller)
- Can be referenced by multiple Agent CRDs via `spec.instruction.instructionRef`
- Can be created inline in an Agent via `spec.instruction.template` (the operator creates a child Instruction CR automatically)

## Basic Structure

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Instruction
metadata:
  name: my-instruction
spec:
  content: |
    You are a helpful assistant. Follow these guidelines:

    1. Be concise and accurate
    2. Ask clarifying questions when needed
    3. Provide examples when helpful
```

## Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `content` | string | Yes (MinLength=1) | The system prompt text that defines the agent's behavior |

The `content` field is the only spec field. It holds the full system prompt text and must be at least one character long.

## Status Fields

```yaml
status:
  configMapName: my-instruction-configmap

  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2026-01-15T10:30:00Z"
      reason: ConfigMapCreated
      message: "ConfigMap created successfully"

  observedGeneration: 1
```

| Field | Type | Description |
|-------|------|-------------|
| `configMapName` | string | Name of the ConfigMap containing the instruction text |
| `conditions` | []Condition | Standard Kubernetes conditions representing the Instruction's state |
| `observedGeneration` | int64 | The generation most recently observed by the controller |

### PrintColumns

When listing Instructions with `kubectl get instructions`, the following columns are displayed:

| Column | Source |
|--------|--------|
| ConfigMap | `.status.configMapName` |
| Age | `.metadata.creationTimestamp` |

## Using Instructions with Agents

There are two ways to provide instructions to an Agent: referencing an existing Instruction resource, or defining the prompt inline.

### Using instructionRef (Recommended for Shared Prompts)

Reference an existing Instruction resource by name. This is ideal when multiple agents share the same system prompt.

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Instruction
metadata:
  name: support-guidelines
spec:
  content: |
    You are a customer support agent. Be professional and empathetic.
---
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: support-agent
spec:
  instruction:
    instructionRef:
      name: support-guidelines
      namespace: shared-resources  # Optional, defaults to agent's namespace
  model:
    name: gpt-4o
  runtime:
    type: standard
    spec:
      container:
        name: agent
        image: ghcr.io/example/support-agent:latest
        ports:
        - containerPort: 8080
```

### Using template (Inline Prompts)

Define the instruction content directly in the Agent spec. The operator automatically creates a child Instruction CR from the template.

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: simple-agent
spec:
  instruction:
    template: |
      You are a helpful coding assistant. Provide clear,
      well-documented code examples.
  model:
    name: gpt-4o
  runtime:
    type: standard
    spec:
      container:
        name: agent
        image: ghcr.io/example/code-agent:latest
        ports:
        - containerPort: 8080
```

### Choosing Between template and instructionRef

- **`template`**: Use for single-agent prompts that are specific to one agent. Simpler to manage as everything is in one manifest.
- **`instructionRef`**: Use when the same prompt is shared across multiple agents, or when you want to manage prompts independently of agent lifecycle.

**Note**: You must specify exactly one of `template` or `instructionRef`. Specifying both or neither will be rejected by the webhook.

## Examples

### Simple Instruction

A minimal instruction with a short system prompt:

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Instruction
metadata:
  name: concise-assistant
spec:
  content: "You are a concise and helpful assistant."
```

### Shared Instruction Across Agents

Create one instruction and reference it from multiple agents:

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Instruction
metadata:
  name: shared-support-instructions
  namespace: shared-resources
spec:
  content: |
    You are a customer support agent. Follow these guidelines:

    1. Always greet the customer professionally
    2. Identify the issue category: billing, technical, account, or general
    3. Attempt to resolve the issue or escalate appropriately
    4. Always summarize the resolution or next steps

    Tone: Professional, empathetic, concise.
---
# Agent in team-a namespace
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: support-agent-a
  namespace: team-a
spec:
  instruction:
    instructionRef:
      name: shared-support-instructions
      namespace: shared-resources
  model:
    name: gpt-4o
  runtime:
    type: standard
    spec:
      container:
        name: agent
        image: ghcr.io/example/support-agent:latest
        ports:
        - containerPort: 8080
---
# Agent in team-b namespace
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: support-agent-b
  namespace: team-b
spec:
  instruction:
    instructionRef:
      name: shared-support-instructions
      namespace: shared-resources
  model:
    name: gpt-4o
  runtime:
    type: standard
    spec:
      container:
        name: agent
        image: ghcr.io/example/support-agent:latest
        ports:
        - containerPort: 8080
```

### Multi-Line Instruction with Detailed Guidelines

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Instruction
metadata:
  name: code-review-instructions
spec:
  content: |
    You are an expert code reviewer. Your role is to analyze code changes
    and provide actionable feedback.

    ## Review Criteria

    1. **Correctness**: Does the code do what it claims?
    2. **Security**: Are there any vulnerabilities?
    3. **Performance**: Are there obvious inefficiencies?
    4. **Readability**: Is the code clear and well-structured?
    5. **Testing**: Are edge cases covered?

    ## Output Format

    For each issue found, provide:
    - Severity: critical, warning, or suggestion
    - Location: file and line reference
    - Description: what the issue is
    - Recommendation: how to fix it

    ## Guidelines

    - Be constructive, not critical
    - Praise good patterns when you see them
    - Prioritize issues by severity
    - Keep feedback concise and actionable
```

## Operations

### Viewing Instructions

```bash
# List all instructions
kubectl get instructions

# List instructions across all namespaces
kubectl get instructions -A

# Get detailed information
kubectl describe instruction shared-support-instructions

# Check the generated ConfigMap name
kubectl get instruction shared-support-instructions -o jsonpath='{.status.configMapName}'

# View the ConfigMap contents
kubectl get configmap <configmap-name> -o yaml
```

### Updating Instructions

When you update the `content` field of an Instruction, the controller updates the backing ConfigMap. Agents referencing the Instruction will pick up the new prompt.

```bash
# Update content via patch
kubectl patch instruction shared-support-instructions --type='merge' \
  -p='{"spec": {"content": "Updated system prompt text."}}'

# Or edit interactively
kubectl edit instruction shared-support-instructions

# Replace from a file
kubectl apply -f instruction.yaml
```

### Deleting Instructions

```bash
# Delete an instruction
kubectl delete instruction shared-support-instructions

# Note: The backing ConfigMap is garbage-collected automatically.
# Agents referencing a deleted Instruction will report an error condition.
```

## Best Practices

1. **Use descriptive names** - Name instructions after their purpose (e.g., `code-review-instructions`, `support-guidelines`)
2. **Share common prompts** - Use `instructionRef` when multiple agents need the same behavior
3. **Use inline for simple cases** - For single-agent, short prompts, `template` keeps manifests self-contained
4. **Version control prompts** - Keep Instruction manifests in Git alongside agent definitions
5. **Use YAML multi-line syntax** - Use `|` for multi-line content to preserve formatting and readability
6. **Organize by namespace** - Place shared instructions in a dedicated namespace (e.g., `shared-resources`)
7. **Keep prompts focused** - Each Instruction should define a clear, single-purpose behavior
8. **Label instructions** - Use labels for organizational purposes (e.g., `team`, `domain`, `purpose`)
9. **Test prompt changes** - Create test agents to validate instruction updates before rolling out to production agents
10. **Document prompt intent** - Use annotations to explain why a prompt is written a certain way

## Troubleshooting

### Instruction Not Ready

```bash
# Check instruction status and conditions
kubectl describe instruction <name>

# Common issues:
# - Content field is empty (MinLength=1 validation)
# - Controller unable to create ConfigMap (RBAC issue)
```

### ConfigMap Not Created

If the `configMapName` status field is empty:

```bash
# Check the operator logs for errors
kubectl logs -n flokoa-system deployment/flokoa-operator -f

# Verify the instruction resource exists and has content
kubectl get instruction <name> -o yaml

# Check for RBAC issues
kubectl auth can-i create configmaps --as=system:serviceaccount:flokoa-system:flokoa-operator
```

### Agent Cannot Find Instruction

If an Agent referencing an Instruction shows errors:

```bash
# Verify the Instruction exists in the expected namespace
kubectl get instruction <name> -n <namespace>

# Check the Agent's instructionRef matches
kubectl get agent <agent-name> -o jsonpath='{.spec.instruction.instructionRef}'

# If cross-namespace, verify the namespace field is set
kubectl get agent <agent-name> -o yaml | grep -A3 instructionRef
```

### Webhook Rejection

If creating an Agent with instructions fails:

- Ensure exactly one of `template` or `instructionRef` is set, not both
- Verify the `content` field is not empty when creating an Instruction directly
- Check that the referenced Instruction exists (if using `instructionRef`)
