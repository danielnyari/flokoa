# AgentTool Resource

The `AgentTool` resource defines external tools that AI agents can use to interact with APIs, services, and data sources.

## Overview

An AgentTool:
- Defines a callable tool with input/output schemas
- Can call external HTTP APIs or internal Kubernetes services
- Provides a description for the LLM to understand when to use it
- Can be shared across multiple agents
- Supports various authentication methods

## Basic Structure

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: my-tool
spec:
  type: http-api
  description: "Description of what this tool does"
  
  httpApi:
    url: "https://api.example.com/endpoint"
    method: GET
  
  inputSchema:
    type: object
    properties:
      param: 
        type: string
```

## Tool Types

Currently supported:
- **http-api**: Call HTTP/REST APIs

## HTTP API Tools

### External API

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: weather-api
spec:
  type: http-api
  description: "Get current weather for a location"
  
  httpApi:
    url: "https://api.weather.com/v1/current"
    method: GET
    timeoutSeconds: 30
    
    headers:
      Accept: "application/json"
      User-Agent: "Flokoa-Agent/1.0"
  
  inputSchema:
    type: object
    properties:
      location:
        type: string
        description: "City name or coordinates"
      units:
        type: string
        enum: ["metric", "imperial"]
        default: "metric"
    required:
      - location
  
  outputSchema:
    type: object
    properties:
      temperature:
        type: number
      conditions:
        type: string
      humidity:
        type: number
```

### Internal Kubernetes Service

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: inventory-check
spec:
  type: http-api
  description: "Check product inventory levels"
  
  httpApi:
    # Reference internal service instead of external URL
    serviceRef:
      name: inventory-service
      namespace: backend
      port: 8080
    
    path: "/api/v1/inventory"
    method: GET
    timeoutSeconds: 10
  
  inputSchema:
    type: object
    properties:
      productId:
        type: string
    required:
      - productId
```

### POST Request with JSON Body

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: create-ticket
spec:
  type: http-api
  description: "Create a support ticket"
  
  httpApi:
    url: "https://api.helpdesk.com/v1/tickets"
    method: POST
    
    headers:
      Content-Type: "application/json"
  
  # For POST/PUT/PATCH, inputSchema becomes the request body
  inputSchema:
    type: object
    properties:
      title:
        type: string
        description: "Ticket title"
      description:
        type: string
        description: "Detailed description"
      priority:
        type: string
        enum: ["low", "medium", "high", "urgent"]
      category:
        type: string
    required:
      - title
      - description
  
  outputSchema:
    type: object
    properties:
      ticketId:
        type: string
      status:
        type: string
      createdAt:
        type: string
        format: date-time
```

## Input and Output Schemas

### Schema Behavior by HTTP Method

**GET requests**: Input schema properties become query parameters
```yaml
httpApi:
  method: GET
inputSchema:
  properties:
    search: {type: string}
    limit: {type: integer}
# Results in: GET /api?search=value&limit=10
```

**POST/PUT/PATCH requests**: Input schema becomes JSON request body
```yaml
httpApi:
  method: POST
inputSchema:
  properties:
    name: {type: string}
    email: {type: string}
# Results in: POST /api with JSON body: {"name": "...", "email": "..."}
```

### Complex Schemas

```yaml
inputSchema:
  type: object
  properties:
    # String with validation
    email:
      type: string
      format: email
      description: "User email address"
    
    # Number with range
    quantity:
      type: integer
      minimum: 1
      maximum: 100
      default: 1
    
    # Enum/choices
    status:
      type: string
      enum: ["active", "inactive", "pending"]
    
    # Nested object
    address:
      type: object
      properties:
        street: {type: string}
        city: {type: string}
        zipCode: {type: string}
    
    # Array
    tags:
      type: array
      items:
        type: string
      minItems: 1
      maxItems: 10
  
  required:
    - email
    - quantity
```

## OpenAPI Integration

Instead of manually defining schemas, reference an OpenAPI specification:

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: github-api
spec:
  type: http-api
  description: "Search GitHub repositories"
  
  httpApi:
    url: "https://api.github.com"
    method: GET
  
  # Reference OpenAPI spec instead of inline schemas
  openApiSchemaRef:
    url: "https://api.github.com/openapi.json"
```

Or store the OpenAPI spec in a ConfigMap:

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
      ...
    }
---
# Reference in AgentTool
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: my-tool
spec:
  type: http-api
  description: "Call my API"
  
  httpApi:
    url: "https://api.example.com"
    method: GET
  
  openApiSchemaRef:
    configMapRef:
      name: api-specs
      key: openapi.json
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
        name: create-ticket
    - toolRef:
        name: search-kb
        namespace: shared-tools  # Cross-namespace reference
  
  runtime:
    type: standard
    spec:
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
    # Inline tool definition
    - name: web-search
      inline:
        type: http-api
        description: "Search the web"
        httpApi:
          url: "https://api.search.com/v1/search"
          method: GET
        inputSchema:
          type: object
          properties:
            query: {type: string}
            limit: {type: integer, default: 10}
          required: ["query"]
  
  runtime:
    type: standard
    spec:
      container:
        name: agent
        image: ghcr.io/example/agent:v1.0.0
        ports:
        - containerPort: 8080
```

## Examples

### Database Query Tool

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: database-query
spec:
  type: http-api
  description: "Execute read-only database queries"
  
  httpApi:
    serviceRef:
      name: query-service
      namespace: data-layer
      port: 5000
    path: "/query"
    method: POST
    timeoutSeconds: 60
  
  inputSchema:
    type: object
    properties:
      sql:
        type: string
        description: "SELECT query to execute"
      maxRows:
        type: integer
        default: 100
        maximum: 1000
    required: ["sql"]
  
  outputSchema:
    type: object
    properties:
      rows:
        type: array
        items:
          type: object
      rowCount:
        type: integer
```

### Email Sender Tool

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: send-email
spec:
  type: http-api
  description: "Send email notifications"
  
  httpApi:
    url: "https://api.sendgrid.com/v3/mail/send"
    method: POST
    
    headers:
      Content-Type: "application/json"
  
  inputSchema:
    type: object
    properties:
      to:
        type: string
        format: email
        description: "Recipient email address"
      subject:
        type: string
        description: "Email subject"
      body:
        type: string
        description: "Email body (plain text or HTML)"
      isHtml:
        type: boolean
        default: false
    required: ["to", "subject", "body"]
  
  outputSchema:
    type: object
    properties:
      messageId:
        type: string
      status:
        type: string
```

### Multi-Step Order Processing

```yaml
# Check inventory tool
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: check-inventory
spec:
  type: http-api
  description: "Check if product is in stock"
  httpApi:
    serviceRef:
      name: inventory-service
      port: 8080
    path: "/check"
    method: GET
  inputSchema:
    type: object
    properties:
      productId: {type: string}
      quantity: {type: integer}
    required: ["productId", "quantity"]
---
# Create order tool
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: create-order
spec:
  type: http-api
  description: "Create a new customer order"
  httpApi:
    serviceRef:
      name: order-service
      port: 8080
    path: "/orders"
    method: POST
  inputSchema:
    type: object
    properties:
      customerId: {type: string}
      items:
        type: array
        items:
          type: object
          properties:
            productId: {type: string}
            quantity: {type: integer}
    required: ["customerId", "items"]
---
# Agent using both tools
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: order-agent
spec:
  tools:
    - toolRef:
        name: check-inventory
    - toolRef:
        name: create-order
  
  runtime:
    type: standard
    spec:
      container:
        name: agent
        image: ghcr.io/example/order-agent:v1.0.0
        ports:
        - containerPort: 8080
```

### Tool with Multiple HTTP Methods

Create separate tools for different operations on the same resource:

```yaml
# GET tool
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: get-user
spec:
  type: http-api
  description: "Get user information"
  httpApi:
    url: "https://api.example.com/users"
    method: GET
  inputSchema:
    properties:
      userId: {type: string}
    required: ["userId"]
---
# POST tool
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: create-user
spec:
  type: http-api
  description: "Create a new user"
  httpApi:
    url: "https://api.example.com/users"
    method: POST
  inputSchema:
    properties:
      name: {type: string}
      email: {type: string}
    required: ["name", "email"]
---
# PUT tool
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: update-user
spec:
  type: http-api
  description: "Update existing user"
  httpApi:
    url: "https://api.example.com/users"
    method: PUT
  inputSchema:
    properties:
      userId: {type: string}
      name: {type: string}
      email: {type: string}
    required: ["userId"]
```

## Status Fields

```yaml
status:
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2026-01-15T10:30:00Z"
      reason: ConfigurationValid
      message: "Tool is configured and ready"
  
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

### Testing Tools

Test a tool independently before giving it to an agent:

```bash
# Port-forward to test internal service
kubectl port-forward svc/inventory-service 8080:8080

# Test with curl
curl -X GET "http://localhost:8080/api/v1/inventory?productId=ABC123"
```

### Updating Tools

```bash
# Update tool timeout
kubectl patch agenttool weather-api --type='json' \
  -p='[{"op": "replace", "path": "/spec/httpApi/timeoutSeconds", "value": 60}]'
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
2. **Validate inputs** - Use JSON schema constraints (required, min/max, format)
3. **Set appropriate timeouts** - Balance responsiveness with reliability
4. **Use internal services** when possible - Faster and more secure than external APIs
5. **Share common tools** - Create reusable tools in a shared namespace
6. **Document output schemas** - Help the agent understand and parse responses
7. **Handle errors gracefully** - Consider timeouts and failure cases
8. **Secure API access** - Use secrets for authentication
9. **Test tools independently** - Verify they work before adding to agents
10. **Version your tools** - Use labels/annotations to track versions

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
  type: http-api
  description: "Call authenticated API"
  httpApi:
    url: "https://api.example.com/data"
    method: GET
    headers:
      # Note: Header values from secrets should be injected by the agent runtime
      # This is a placeholder - actual implementation may vary
      Authorization: "Bearer ${API_KEY}"
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
  httpApi:
    timeoutSeconds: 120  # Increase from default 30s
```

### Schema Validation Errors

- Verify JSON schema syntax
- Test schemas with sample data
- Check required fields match API expectations
- Ensure enum values are correct

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
