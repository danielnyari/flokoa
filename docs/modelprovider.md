# ModelProvider Resource

The `ModelProvider` resource configures connection settings for Large Language Model (LLM) providers like OpenAI, Anthropic, Google, and AWS Bedrock.

## Overview

A ModelProvider:
- Stores API credentials and connection configuration
- Can be shared across multiple Model resources
- Supports multiple LLM providers
- Handles authentication and endpoint configuration

## Supported Providers

- **OpenAI** - OpenAI API and compatible endpoints
- **Anthropic** - Claude models via Anthropic API
- **Google** - Gemini models via Google AI or Vertex AI
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
  
  # Exactly one provider configuration:
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
    # Optional: override default endpoint
    baseURL: "https://api.openai.com/v1"
    
    # Optional: organization ID
    organizationID: "org-xxx"
    
    # Optional: request timeout
    timeoutSeconds: 60
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
    
    # Optional: request timeout
    timeoutSeconds: 60
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
  
  google:
    timeoutSeconds: 60
```

For Vertex AI (requires service account):

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
    
    timeoutSeconds: 60
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
    
    # Optional: inference profile ARN
    inferenceProfileARN: "arn:aws:bedrock:us-east-1:123456789012:inference-profile/xxx"
```

For Bedrock, AWS credentials are typically provided via:
- IAM roles for service accounts (IRSA) - recommended
- Environment variables in the operator pod
- EC2 instance profiles

## Advanced Configuration

### Custom Endpoints

Use custom or self-hosted LLM endpoints:

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
    timeoutSeconds: 120
  
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
    # Option 1: Skip verification (not recommended for production)
    insecureSkipVerify: false
    
    # Option 2: Provide custom CA certificate
    caSecretRef:
      name: custom-ca-cert
      key: ca.crt
    
    # Include system CAs in addition to custom CA
    useSystemCAs: true
```

**Create CA certificate secret:**
```bash
kubectl create secret generic custom-ca-cert \
  --from-file=ca.crt=/path/to/ca.crt
```

## Examples

### Shared Provider for Multiple Models

```yaml
# Provider configuration
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: openai-shared
  namespace: shared-resources
spec:
  apiKeySecretRef:
    name: openai-credentials
    key: api-key
  openai:
    organizationID: "org-xxx"
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
# OpenAI Provider
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
# Anthropic Provider
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
# Google Provider
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
    timeoutSeconds: 60
  
  defaultHeaders:
    api-version: "2024-02-01"
```

### Development vs Production

Development (less strict):
```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: dev-provider
  namespace: development
spec:
  apiKeySecretRef:
    name: dev-credentials
    key: api-key
  
  openai:
    timeoutSeconds: 30
```

Production (with monitoring and failover):
```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: prod-provider
  namespace: production
  labels:
    environment: production
    monitoring: enabled
spec:
  apiKeySecretRef:
    name: prod-credentials
    key: api-key
  
  openai:
    organizationID: "org-prod"
    timeoutSeconds: 120
  
  defaultHeaders:
    X-Environment: "production"
    X-Request-Source: "flokoa"
```

## Status Fields

The operator maintains status information:

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

# The operator will detect the change automatically
```

### Using Providers Across Namespaces

```bash
# Create provider in shared namespace
kubectl apply -f provider.yaml -n shared-resources

# Reference from different namespace
kubectl apply -f model.yaml -n my-app
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

### Example: Restricting Secret Access

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: agent-sa
  namespace: my-namespace
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: secret-reader
  namespace: my-namespace
rules:
- apiGroups: [""]
  resources: ["secrets"]
  resourceNames: ["openai-credentials"]  # Only specific secrets
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: agent-secret-reader
  namespace: my-namespace
subjects:
- kind: ServiceAccount
  name: agent-sa
roleRef:
  kind: Role
  name: secret-reader
  apiGroup: rbac.authorization.k8s.io
```

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
- Invalid provider configuration

### Authentication Errors

```bash
# Verify secret exists and has correct key
kubectl get secret openai-credentials -o jsonpath='{.data.api-key}' | base64 -d

# Check provider configuration
kubectl get modelprovider openai-provider -o yaml
```

### Connection Timeouts

- Increase `timeoutSeconds` if requests are timing out
- Check network policies allowing egress to provider APIs
- Verify DNS resolution works for provider endpoints
- Check firewall rules and proxy configurations

### Testing Provider Connection

Create a test Model and Agent to verify connectivity:

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
