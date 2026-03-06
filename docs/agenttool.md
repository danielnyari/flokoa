# AgentTool Resource

The `AgentTool` resource defines external tools that AI agents can use to interact with APIs and services, backed by OpenAPI specifications.

## Overview

An AgentTool:
- Defines a callable toolset backed by an OpenAPI specification
- Can call external APIs (via URL) or internal Kubernetes services (via ServiceRef)
- Provides a description for the LLM to understand when to use it
- Can be shared across multiple agents
- Requires a mandatory OpenAPI schema source

## Basic Structure

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: my-tool
spec:
  type: openapi
  description: "Description of what this tool does"

  openApi:
    url: "https://api.example.com"
    openApiSchema:
      endpointPath: "/openapi.json"
```

## Tool Types

Currently supported:
- **openapi**: Tools backed by an OpenAPI specification

## OpenAPI Schema Sources

The OpenAPI schema is **mandatory** and defines the available operations, input parameters, and output schemas. It can be sourced from one of three locations:

### 1. Endpoint Path (`endpointPath`)

Fetch the OpenAPI spec from a path on the target service/URL itself:

```yaml
spec:
  type: openapi
  description: "Weather API"
  openApi:
    url: "https://api.weather.com/v1"
    openApiSchema:
      endpointPath: "/docs/openapi.json"
```

### 2. Inline Value (`value`)

Embed the OpenAPI spec directly in the tool definition:

```yaml
spec:
  type: openapi
  description: "Simple calculator API"
  openApi:
    url: "https://api.example.com"
    openApiSchema:
      value:
        openapi: "3.0.0"
        info:
          title: Calculator API
          version: "1.0"
        paths:
          /calculate:
            post:
              summary: Perform calculation
              requestBody:
                content:
                  application/json:
                    schema:
                      type: object
                      properties:
                        operation:
                          type: string
                          enum: ["add", "subtract", "multiply", "divide"]
                        a:
                          type: number
                        b:
                          type: number
              responses:
                "200":
                  description: Result
                  content:
                    application/json:
                      schema:
                        type: object
                        properties:
                          result:
                            type: number
```

### 3. ConfigMap Reference (`valueFrom`)

Reference an OpenAPI spec stored in a ConfigMap:

```yaml
# Create ConfigMap with OpenAPI spec
apiVersion: v1
kind: ConfigMap
metadata:
  name: api-specs
data:
  openapi.json: |
    {
      "openapi": "3.0.0",
      "info": {"title": "My API", "version": "1.0"},
      "paths": { ... }
    }
---
# Reference in AgentTool
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: my-tool
spec:
  type: openapi
  description: "Call my API"
  openApi:
    url: "https://api.example.com"
    openApiSchema:
      valueFrom:
        name: api-specs
        key: openapi.json
```

## Target Configuration

### External API (URL)

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: weather-api
spec:
  type: openapi
  description: "Get current weather for a location"

  openApi:
    url: "https://api.weather.com/v1"
    timeoutSeconds: 30
    headers:
      Accept: "application/json"
      User-Agent: "Flokoa-Agent/1.0"
    openApiSchema:
      endpointPath: "/openapi.json"
```

### Internal Kubernetes Service (ServiceRef)

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: inventory-check
spec:
  type: openapi
  description: "Check product inventory levels"

  openApi:
    serviceRef:
      name: inventory-service
      namespace: backend
      port: 8080
    timeoutSeconds: 10
    openApiSchema:
      endpointPath: "/api/openapi.json"
```

## Using Tools in Agents

### Reference Tool by Name

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: customer-service-agent
spec:
  tools:
    - toolRef:
        name: weather-api
    - toolRef:
        name: inventory-check
        namespace: shared-tools  # Cross-namespace reference

  runtime:
    type: standard
    standard:
      container:
        name: agent
        image: ghcr.io/example/agent:v1.0.0
        ports:
        - containerPort: 8080
```

### Inline Tool Definition

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: search-agent
spec:
  tools:
    - name: petstore
      template:
        type: openapi
        description: "Petstore API"
        openApi:
          url: "https://petstore3.swagger.io"
          openApiSchema:
            endpointPath: "/api/v3/openapi.json"

  runtime:
    type: standard
    standard:
      container:
        name: agent
        image: ghcr.io/example/agent:v1.0.0
        ports:
        - containerPort: 8080
```

## Examples

### Database Query Service

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: database-query
spec:
  type: openapi
  description: "Execute read-only database queries via query service"

  openApi:
    serviceRef:
      name: query-service
      namespace: data-layer
      port: 5000
    timeoutSeconds: 60
    openApiSchema:
      endpointPath: "/docs/openapi.json"
```

### OpenAPI Spec from ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: email-api-spec
data:
  openapi.yaml: |
    openapi: "3.0.0"
    info:
      title: Email API
      version: "1.0"
    paths:
      /send:
        post:
          summary: Send email notification
          requestBody:
            content:
              application/json:
                schema:
                  type: object
                  properties:
                    to:
                      type: string
                      format: email
                    subject:
                      type: string
                    body:
                      type: string
                  required: [to, subject, body]
          responses:
            "200":
              description: Email sent
---
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: send-email
spec:
  type: openapi
  description: "Send email notifications"

  openApi:
    url: "https://api.sendgrid.com/v3"
    headers:
      Content-Type: "application/json"
    openApiSchema:
      valueFrom:
        name: email-api-spec
        key: openapi.yaml
```

### Multiple Tools Sharing a Service

```yaml
# A single OpenAPI tool covers all operations
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: user-management
spec:
  type: openapi
  description: "User management API - create, read, update users"
  openApi:
    url: "https://api.example.com"
    openApiSchema:
      endpointPath: "/openapi.json"
---
# Agent using the tool
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: admin-agent
spec:
  tools:
    - toolRef:
        name: user-management

  runtime:
    type: standard
    standard:
      container:
        name: agent
        image: ghcr.io/example/admin-agent:v1.0.0
        ports:
        - containerPort: 8080
```

## Status Fields

```yaml
status:
  conditions:
    - type: Validated
      status: "True"
      lastTransitionTime: "2026-01-15T10:30:00Z"
      reason: ValidationSuccess
      message: "Spec is valid"
    - type: Stored
      status: "True"
      lastTransitionTime: "2026-01-15T10:30:00Z"
      reason: StorageSuccess
      message: "Spec stored in ConfigMap"

  observedGeneration: 1
```

## Operations

### Viewing Tools

```bash
# List all tools
kubectl get agenttools

# Get detailed information
kubectl describe agenttool weather-api

# List tools used by an agent
kubectl get agent my-agent -o jsonpath='{.spec.tools[*].toolRef.name}'
```

### Updating Tools

```bash
# Update tool timeout
kubectl patch agenttool weather-api --type='json' \
  -p='[{"op": "replace", "path": "/spec/openApi/timeoutSeconds", "value": 60}]'
```

### Sharing Tools

```bash
# Create tool in shared namespace
kubectl apply -f tool.yaml -n shared-tools

# Reference from any agent
# spec.tools[].toolRef.namespace: shared-tools
```

## Best Practices

1. **Write clear descriptions** - The LLM uses this to decide when to use the tool
2. **Use `endpointPath`** when the service serves its own OpenAPI spec - simplest setup
3. **Use `valueFrom`** for static OpenAPI specs that don't change with the service
4. **Use `value`** for small, simple APIs where inline definition is clearest
5. **Set appropriate timeouts** - Balance responsiveness with reliability
6. **Use internal services** when possible - Faster and more secure than external APIs
7. **Share common tools** - Create reusable tools in a shared namespace
8. **Secure API access** - Use secrets for authentication
9. **Test tools independently** - Verify they work before adding to agents

## Security Considerations

### API Credentials

Never hardcode credentials in tool definitions. Use Kubernetes secrets:

```yaml
# Create secret
apiVersion: v1
kind: Secret
metadata:
  name: api-credentials
stringData:
  api-key: "your-api-key-here"
---
# Reference in tool headers
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: secure-tool
spec:
  type: openapi
  description: "Call authenticated API"
  openApi:
    url: "https://api.example.com"
    headers:
      # Note: Header values from secrets should be injected by the agent runtime
      Authorization: "Bearer ${API_KEY}"
    openApiSchema:
      endpointPath: "/openapi.json"
```

### Network Policies

Restrict agent network access:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: agent-egress
spec:
  podSelector:
    matchLabels:
      flokoa.ai/agent: my-agent
  policyTypes:
    - Egress
  egress:
    # Allow DNS
    - to:
      - namespaceSelector:
          matchLabels:
            name: kube-system
      ports:
      - protocol: UDP
        port: 53
    # Allow specific services only
    - to:
      - podSelector:
          matchLabels:
            app: inventory-service
      ports:
      - protocol: TCP
        port: 8080
```

### Rate Limiting

Protect external APIs:
- Set reasonable `timeoutSeconds`
- Implement rate limiting in your agent code
- Monitor tool usage
- Consider API quotas and costs

## Troubleshooting

### Tool Not Working

```bash
# Check tool status
kubectl describe agenttool <name>

# Test endpoint manually
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl -v https://api.example.com/endpoint

# Check agent logs for tool errors
kubectl logs -l flokoa.ai/agent=<agent-name> | grep "tool"
```

### Timeout Issues

```yaml
# Increase timeout
spec:
  openApi:
    timeoutSeconds: 120  # Increase from default 30s
```

### OpenAPI Schema Issues

- Verify the OpenAPI spec is valid (use online validators)
- For `endpointPath`: ensure the path returns a valid OpenAPI spec
- For `valueFrom`: verify the ConfigMap exists and the key contains a valid spec
- For `value`: check YAML/JSON syntax of the inline spec

### Service Resolution Issues

For internal services:
```bash
# Verify service exists
kubectl get svc inventory-service -n backend

# Check DNS resolution from agent pod
kubectl exec -it <agent-pod> -- nslookup inventory-service.backend.svc.cluster.local

# Test service connectivity
kubectl exec -it <agent-pod> -- curl http://inventory-service.backend:8080/health
```

### Authentication Failures

- Verify API keys/credentials are correct
- Check header format matches API requirements
- Ensure secrets exist and are readable by agents
- Review API provider documentation for auth requirements
