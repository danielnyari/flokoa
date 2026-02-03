# Prompt Examples

This directory contains examples of using the Prompt CRD in Flokoa.

## Overview

The Prompt CRD allows you to manage AI prompts separately from your agents, supporting:

- **Inline prompts**: Define prompt content directly in the manifest
- **Langfuse integration**: Fetch prompts from Langfuse with version control
- **Langsmith integration**: Fetch prompts from Langsmith with commit-based versioning
- **Automatic sync**: Periodically refresh prompts from external sources
- **Agent integration**: Reference prompts from Agent resources

## Examples

### 1. Inline Prompt

The simplest way to define a prompt with content directly in the manifest.

```bash
kubectl apply -f inline-prompt.yaml
```

See: [inline-prompt.yaml](./inline-prompt.yaml)

### 2. Langfuse Prompt

Fetch prompts from Langfuse with automatic syncing.

```bash
# First, create the credentials secret
kubectl create secret generic langfuse-project-credentials \
  --from-literal=publicKey=pk-lf-xxx \
  --from-literal=secretKey=sk-lf-xxx

# Then apply the prompt
kubectl apply -f langfuse-prompt.yaml
```

See: [langfuse-prompt.yaml](./langfuse-prompt.yaml)

**Features:**
- Fetches prompt from Langfuse by name and version
- Supports version pinning or "latest"
- Optional periodic sync (e.g., every 5 minutes)

### 3. Langsmith Prompt

Fetch prompts from Langsmith using commit hashes.

```bash
# First, create the credentials secret
kubectl create secret generic langsmith-credentials \
  --from-literal=apiKey=lsv2_pt_xxx

# Then apply the prompt
kubectl apply -f langsmith-prompt.yaml
```

See: [langsmith-prompt.yaml](./langsmith-prompt.yaml)

**Features:**
- Fetches prompt from Langsmith by name and commit
- Supports commit hash or "latest"
- Optional periodic sync (e.g., every 1 hour)

### 4. Agent with Prompts

Shows how to reference prompts from an Agent resource.

```bash
kubectl apply -f agent-with-prompts.yaml
```

See: [agent-with-prompts.yaml](./agent-with-prompts.yaml)

**Features:**
- Reference multiple prompts by name
- Assign aliases (e.g., "system", "guidelines")
- Prompts are resolved and made available to the agent

## Checking Prompt Status

After creating a prompt, you can check its status:

```bash
# List all prompts
kubectl get prompts

# Get detailed status
kubectl describe prompt customer-support-system

# View resolved content
kubectl get prompt customer-support-system -o jsonpath='{.status.resolvedContent}'
```

The status includes:
- `resolvedContent`: The current prompt content
- `resolvedAt`: Timestamp of last resolution
- `sourceVersion`: Version/commit from the source
- `checksum`: SHA256 checksum of the content
- `conditions`: Ready status and any errors

## Source Configuration

### Inline Source

```yaml
source:
  inline:
    content: "Your prompt content here"
```

### Langfuse Source

```yaml
source:
  langfuse:
    endpoint: https://cloud.langfuse.com  # optional
    credentialsSecretRef:
      name: langfuse-credentials
    promptName: my-prompt
    version: "3"  # or "latest"
```

**Secret Format:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: langfuse-credentials
type: Opaque
stringData:
  publicKey: "pk-lf-xxx"
  secretKey: "sk-lf-xxx"
```

### Langsmith Source

```yaml
source:
  langsmith:
    credentialsSecretRef:
      name: langsmith-credentials
    promptName: my-prompt
    commitHash: "abc123"  # or "latest"
```

**Secret Format:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: langsmith-credentials
type: Opaque
stringData:
  apiKey: "lsv2_pt_xxx"
```

## Sync Configuration

For non-inline sources, you can configure automatic syncing:

```yaml
sync:
  interval: 5m  # Supported units: s, m, h
```

Examples:
- `30s` - Every 30 seconds
- `5m` - Every 5 minutes  
- `1h` - Every 1 hour
- `24h` - Every 24 hours

## Best Practices

1. **Version Control**: Use specific versions in production, "latest" in development
2. **Sync Intervals**: Balance freshness vs. API rate limits
   - Development: 1-5 minutes
   - Production: 15-60 minutes
3. **Secret Management**: Use tools like Sealed Secrets or External Secrets Operator for credentials
4. **Namespaces**: Keep prompts in the same namespace as the agents that use them
5. **Prompt Naming**: Use descriptive names that indicate purpose (e.g., `system-prompt`, `safety-guidelines`)

## Troubleshooting

### Prompt not resolving

```bash
# Check conditions
kubectl describe prompt <name>

# Look for error messages in the Ready condition
```

Common issues:
- Invalid credentials in secret
- Network connectivity to external service
- Invalid prompt name or version

### Sync not working

Check that:
- Sync interval is properly formatted (e.g., "5m" not "5 minutes")
- Prompt source is not inline (inline prompts don't sync)
- Controller logs for sync errors: `kubectl logs -n flokoa-system deployment/flokoa-controller-manager`

## API Reference

For the full API specification, see the [Prompt CRD documentation](../../operator/api/v1alpha1/prompt_types.go).
