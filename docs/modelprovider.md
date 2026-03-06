# ModelProvider Resource

The `ModelProvider` resource configures connection settings for Large Language Model (LLM) providers like OpenAI, Anthropic, Google, and AWS Bedrock.

## Overview

A ModelProvider:
- Stores API credentials and connection configuration
- Can be shared across multiple Model resources
- Supports multiple LLM providers
- Handles authentication and endpoint configuration
- The provider type is inferred from which configuration block is set

## Supported Providers

- **OpenAI** - OpenAI API and compatible endpoints
- **Anthropic** - Claude models via Anthropic API
- **Google** - Gemini models via Google AI API or Vertex AI
- **Bedrock** - AWS Bedrock inference

## Basic Structure

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: my-provider
spec:
  apiKeySecretRef:
    name: api-credentials
    key: api-key

  # Exactly one provider configuration block must be set.
  # The provider type is inferred from which block is present.
  openai: {}
  # OR anthropic: {}
  # OR google: {}
  # OR bedrock: {}
```

## Provider Configurations

### OpenAI

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: openai-provider
spec:
  apiKeySecretRef:
    name: openai-credentials
    key: api-key

  openai:
    # Optional: override default endpoint (for Azure OpenAI, self-hosted, etc.)
    baseURL: "https://api.openai.com/v1"
```

**Create the secret:**
```bash
kubectl create secret generic openai-credentials \
  --from-literal=api-key=sk-proj-xxx
```

### Anthropic

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: anthropic-provider
spec:
  apiKeySecretRef:
    name: anthropic-credentials
    key: api-key

  anthropic:
    # Optional: override default endpoint
    baseURL: "https://api.anthropic.com"
```

**Create the secret:**
```bash
kubectl create secret generic anthropic-credentials \
  --from-literal=api-key=sk-ant-xxx
```

### Google (Gemini)

For Google AI API (simpler, API key based):

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: google-provider
spec:
  apiKeySecretRef:
    name: google-credentials
    key: api-key

  google: {}
```

For Vertex AI (requires service account and project):

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: vertex-ai-provider
spec:
  google:
    project: "my-gcp-project"
    location: "us-central1"

    serviceAccountKeySecretRef:
      name: gcp-sa-credentials
      key: service-account.json
```

**Create the service account secret:**
```bash
kubectl create secret generic gcp-sa-credentials \
  --from-file=service-account.json=/path/to/key.json
```

### AWS Bedrock

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: bedrock-provider
spec:
  bedrock:
    region: "us-east-1"
```

For Bedrock, AWS credentials are typically provided via:
- IAM roles for service accounts (IRSA) - recommended
- Environment variables in the operator pod
- EC2 instance profiles

## Advanced Configuration

### Custom Endpoints

Use custom or self-hosted LLM endpoints with OpenAI-compatible APIs:

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: custom-openai
spec:
  apiKeySecretRef:
    name: custom-api-credentials
    key: api-key

  openai:
    baseURL: "https://my-custom-llm.example.com/v1"

  # Custom headers for authentication or routing
  defaultHeaders:
    X-Custom-Header: "value"
    X-Tenant-ID: "tenant-123"
```

### TLS Configuration

For custom endpoints with self-signed certificates:

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: secure-provider
spec:
  apiKeySecretRef:
    name: api-credentials
    key: api-key

  openai:
    baseURL: "https://internal-llm.company.local/v1"

  tls:
    # Skip verification (not recommended for production)
    insecureSkipVerify: false

    # Provide custom CA certificate
    caSecretRef:
      name: custom-ca-cert
      key: ca.crt

    # Include system CAs in addition to custom CA (default: true)
    useSystemCAs: true
```

**Create CA certificate secret:**
```bash
kubectl create secret generic custom-ca-cert \
  --from-file=ca.crt=/path/to/ca.crt
```

### Default Headers

Add custom headers to all requests made through this provider:

```yaml
spec:
  defaultHeaders:
    X-Environment: "production"
    X-Request-Source: "flokoa"
    api-version: "2024-02-01"  # Useful for Azure OpenAI
```

## Spec Fields Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `apiKeySecretRef` | SecretKeySelector | No | Reference to secret containing the API key |
| `openai` | Object | No* | OpenAI-specific config (baseURL) |
| `anthropic` | Object | No* | Anthropic-specific config (baseURL) |
| `google` | Object | No* | Google-specific config (project, location, serviceAccountKeySecretRef) |
| `bedrock` | Object | No* | Bedrock-specific config (region) |
| `tls` | Object | No | TLS configuration for custom endpoints |
| `defaultHeaders` | map[string]string | No | Headers to include in all requests |

\* Exactly one provider block must be set.

## Examples

### Shared Provider for Multiple Models

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: openai-shared
  namespace: shared-resources
spec:
  apiKeySecretRef:
    name: openai-credentials
    key: api-key
  openai: {}
---
# Multiple models using the same provider
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: gpt-4o
  namespace: my-namespace
spec:
  model: "gpt-4o"
  providerRef:
    name: openai-shared
    namespace: shared-resources
---
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: gpt-4o-mini
  namespace: my-namespace
spec:
  model: "gpt-4o-mini"
  providerRef:
    name: openai-shared
    namespace: shared-resources
```

### Multi-Provider Setup

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: openai
spec:
  apiKeySecretRef:
    name: openai-credentials
    key: api-key
  openai: {}
---
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: anthropic
spec:
  apiKeySecretRef:
    name: anthropic-credentials
    key: api-key
  anthropic: {}
---
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: google
spec:
  apiKeySecretRef:
    name: google-credentials
    key: api-key
  google: {}
```

### Azure OpenAI

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: azure-openai
spec:
  apiKeySecretRef:
    name: azure-credentials
    key: api-key

  openai:
    baseURL: "https://your-resource.openai.azure.com/openai/deployments/your-deployment"

  defaultHeaders:
    api-version: "2024-02-01"
```

## Status Fields

```yaml
status:
  provider: openai  # Resolved provider type
  ready: true

  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2026-01-15T10:30:00Z"
      reason: SecretFound
      message: "Provider is configured and ready"

  observedGeneration: 1
  secretHash: "abc123..."  # For detecting secret changes
```

## Operations

### Viewing Providers

```bash
# List all providers
kubectl get modelproviders

# Get detailed information
kubectl describe modelprovider openai-provider

# Check provider status
kubectl get modelprovider openai-provider -o jsonpath='{.status.ready}'
```

### Updating API Keys

```bash
# Update the secret
kubectl create secret generic openai-credentials \
  --from-literal=api-key=sk-proj-new-key \
  --dry-run=client -o yaml | kubectl apply -f -

# The operator will detect the change automatically via secretHash
```

### Using Providers Across Namespaces

```bash
# Create provider in shared namespace
kubectl apply -f provider.yaml -n shared-resources

# Reference from different namespace in Model spec
# providerRef.namespace: shared-resources
```

## Security Best Practices

1. **Never commit secrets** to version control - use Kubernetes secrets
2. **Use RBAC** to restrict access to secrets containing API keys
3. **Rotate keys regularly** and update secrets
4. **Use namespace isolation** for different environments
5. **Enable TLS verification** for custom endpoints
6. **Audit secret access** using Kubernetes audit logs
7. **Consider external secret management** (e.g., Vault, AWS Secrets Manager)
8. **Use service accounts** (IRSA/Workload Identity) when possible instead of API keys

## Troubleshooting

### Provider Not Ready

Check the provider status:
```bash
kubectl describe modelprovider <name>
```

Common issues:
- Secret not found or wrong name/key
- Invalid API key in secret
- Network connectivity issues to provider API
- No provider configuration block set

### Authentication Errors

```bash
# Verify secret exists and has correct key
kubectl get secret openai-credentials -o jsonpath='{.data.api-key}' | base64 -d

# Check provider configuration
kubectl get modelprovider openai-provider -o yaml
```

### Connection Timeouts

- Check network policies allowing egress to provider APIs
- Verify DNS resolution works for provider endpoints
- Check firewall rules and proxy configurations
- Set `timeOut` on the Model resource parameters if needed

### Testing Provider Connection

Create a test Model to verify connectivity:

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: test-model
spec:
  model: "gpt-4o-mini"  # Use a small/cheap model for testing
  providerRef:
    name: openai-provider
```

```bash
# Check if model becomes ready
kubectl get model test-model -w
```
