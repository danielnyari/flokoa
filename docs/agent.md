# Agent Resource

The `Agent` resource is the core of Flokoa, representing a deployable AI agent in your Kubernetes cluster.

## Overview

An Agent defines:
- The container image running your AI agent code
- Runtime configuration (replicas, resources, health checks)
- A2A protocol metadata (card with skills and capabilities)
- Optional model configuration for LLM access
- Optional instruction (system prompt) for agent behavior
- Optional tools the agent can use
- Framework declaration for observability

## Basic Structure

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: my-agent
spec:
  card:
    name: "My Agent"
    description: "Description of what this agent does"
    version: "1.0.0"
    skills:
      - id: "main-skill"
        name: "Main Skill"
        description: "The primary capability"
        tags: ["general"]
  runtime:
    type: standard
    standard:
      container:
        name: agent
        image: your-agent-image:tag
        ports:
        - containerPort: 8080
```

## Spec Fields

### Card (A2A Agent Metadata)

The `card` field contains metadata about the agent following the A2A (Agent-to-Agent) protocol. All fields are required:

```yaml
spec:
  card:
    name: "Customer Support Agent"
    description: "Handles customer inquiries and support requests"
    version: "1.0.0"

    # Input/output modes (defaults to application/json)
    defaultInputModes:
      - "application/json"
    defaultOutputModes:
      - "application/json"

    # Capabilities (defaults to streaming: false)
    capabilities:
      streaming: true
      pushNotifications: false
      stateTransitionHistory: true
      extensions:
        - uri: "urn:example:extension"
          description: "Custom extension"
          required: false

    # Skills this agent can perform
    skills:
      - id: "answer-questions"
        name: "Answer Customer Questions"
        description: "Provide answers to common customer questions"
        tags:
          - "support"
          - "faq"
        examples:
          - "What are your business hours?"
          - "How do I reset my password?"
        inputModes:
          - "text"
        outputModes:
          - "application/json"
```

### Runtime Configuration

The `runtime` field defines how the agent is deployed. Two types are supported: `standard` and `template`.

#### Standard Runtime

Uses your own container image:

```yaml
spec:
  runtime:
    type: standard
    standard:
      replicas: 2

      container:
        name: agent
        image: ghcr.io/example/agent:v1.0.0

        ports:
        - containerPort: 8080
          name: http
          protocol: TCP

        env:
        - name: LOG_LEVEL
          value: "info"

        resources:
          requests:
            cpu: "100m"
            memory: "128Mi"
          limits:
            cpu: "500m"
            memory: "512Mi"

        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10

        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5

      # Pod-level scheduling (all optional)
      serviceAccountName: agent-sa
      imagePullSecrets:
        - name: ghcr-secret
      securityContext:
        fsGroup: 1000
        runAsNonRoot: true
      nodeSelector:
        workload-type: ai-agents
      tolerations:
        - key: "ai-workload"
          operator: "Equal"
          value: "true"
          effect: "NoSchedule"
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  flokoa.ai/agent: my-agent
              topologyKey: topology.kubernetes.io/zone

      # Volumes
      volumes:
      - name: config
        configMap:
          name: agent-config
```

#### Template Runtime

Uses a generic runtime image managed by the operator. The agent's behavior is defined via `instruction` and `config.outputSchema`:

```yaml
spec:
  instruction:
    template: "You are a helpful assistant that answers questions concisely."

  runtime:
    type: template
    template:
      replicas: 2
      config:
        outputSchema:
          name: "response"
          description: "Structured response format"
          jsonSchema:
            type: object
            properties:
              answer:
                type: string
              confidence:
                type: number
      env:
        - name: LOG_LEVEL
          value: "info"
      resources:
        requests:
          cpu: "100m"
          memory: "128Mi"
```

### Instruction

Define the system prompt for the agent. Can be inline or a reference:

#### Inline Template

```yaml
spec:
  instruction:
    template: |
      You are a customer service agent for Acme Corp.
      Always be polite and helpful.
      If you cannot resolve an issue, create a support ticket.
```

When using an inline template, the operator creates a child Instruction CR automatically.

#### Reference an Instruction Resource

```yaml
spec:
  instruction:
    instructionRef:
      name: shared-prompt
      namespace: shared-resources  # Optional
```

### Framework Declaration

Explicitly declare which AI framework your agent uses:

```yaml
spec:
  framework: pydantic-ai  # or: langchain, crewai, marvin, autogen, a2a
```

### Model Configuration

Reference a Model resource to give your agent access to an LLM:

```yaml
spec:
  model:
    name: gpt-4o-model
    namespace: shared-models  # Optional, defaults to agent's namespace
```

### Tools

Agents can use tools to interact with external services:

#### Inline Tool Definition

```yaml
spec:
  tools:
    - name: weather-api
      template:
        type: openapi
        description: "Get current weather for a location"
        openApi:
          url: "https://api.weather.com/v1"
          openApiSchema:
            endpointPath: "/openapi.json"
```

#### Tool Reference

```yaml
spec:
  tools:
    - toolRef:
        name: product-search-tool
        namespace: shared-tools  # Optional
```

## Status Fields

The operator updates these fields automatically:

```yaml
status:
  phase: Running  # Pending, Running, or Failed
  backend: standard
  url: http://my-agent.default.svc.cluster.local:8080
  replicas: 2
  availableReplicas: 2
  detectedFramework: pydantic-ai
  lastToolSync: "2026-01-15T10:30:00Z"
  observedGeneration: 1

  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2026-01-15T10:30:00Z"
      reason: DeploymentAvailable
      message: "Agent is running and available"
```

| Field | Description |
|-------|-------------|
| `phase` | Current lifecycle phase: Pending, Running, or Failed |
| `backend` | Runtime backend in use (standard or template) |
| `url` | Endpoint URL for invoking the agent |
| `replicas` | Current number of pod replicas |
| `availableReplicas` | Number of ready and available replicas |
| `detectedFramework` | AI framework detected from the container image |
| `lastToolSync` | Last time tools were synchronized to the agent |
| `observedGeneration` | Most recent generation observed by the controller |
| `conditions` | Standard Kubernetes conditions |

## Examples

### Minimal Agent

The absolute minimum configuration:

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: minimal-agent
spec:
  card:
    name: "Minimal Agent"
    description: "A minimal agent"
    version: "1.0.0"
    skills:
      - id: "default"
        name: "Default"
        description: "Default skill"
        tags: ["general"]
  runtime:
    type: standard
    standard:
      container:
        name: agent
        image: ghcr.io/example/agent:latest
        ports:
        - containerPort: 8080
          name: http
```

### Production Agent with High Availability

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: production-agent
spec:
  card:
    name: "Production Agent"
    description: "Production-grade AI agent"
    version: "2.0.0"
    capabilities:
      streaming: false
    skills:
      - id: "assist"
        name: "Assist"
        description: "General assistance"
        tags: ["production"]

  framework: pydantic-ai

  instruction:
    template: "You are a helpful production assistant."

  model:
    name: gpt-4o-model

  runtime:
    type: standard
    standard:
      replicas: 3

      container:
        name: agent
        image: ghcr.io/example/agent:v2.0.0

        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics

        env:
        - name: ENVIRONMENT
          value: "production"
        - name: API_KEY
          valueFrom:
            secretKeyRef:
              name: agent-secrets
              key: api-key

        resources:
          requests:
            cpu: "500m"
            memory: "512Mi"
          limits:
            cpu: "2000m"
            memory: "2Gi"

        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10

        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5

        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 1000
          capabilities:
            drop:
            - ALL

      securityContext:
        fsGroup: 1000
        runAsNonRoot: true
        seccompProfile:
          type: RuntimeDefault

      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  flokoa.ai/agent: production-agent
              topologyKey: topology.kubernetes.io/zone
```

### Agent with Multiple Tools

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: multi-tool-agent
spec:
  card:
    name: "Multi-Tool Agent"
    description: "Agent with multiple tools"
    version: "1.0.0"
    skills:
      - id: "query"
        name: "Query"
        description: "Query data and send notifications"
        tags: ["tools"]

  model:
    name: claude-sonnet-model

  tools:
    # Reference external tool
    - toolRef:
        name: weather-tool

    # Inline tool definition
    - name: database-query
      template:
        type: openapi
        description: "Query the product database"
        openApi:
          serviceRef:
            name: database-service
            namespace: backend
            port: 5432
          openApiSchema:
            endpointPath: "/openapi.json"

    # Cross-namespace tool
    - toolRef:
        name: email-sender
        namespace: shared-tools

  runtime:
    type: standard
    standard:
      container:
        name: agent
        image: ghcr.io/example/agent:v1.0.0
        ports:
        - containerPort: 8080
          name: http
```

### Template Runtime Agent

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: templated-agent
spec:
  card:
    name: "Q&A Agent"
    description: "Answers questions using a managed runtime"
    version: "1.0.0"
    skills:
      - id: "qa"
        name: "Q&A"
        description: "Answer questions"
        tags: ["qa"]

  model:
    name: gpt-4o-model

  instruction:
    template: |
      You are a Q&A assistant. Answer questions concisely and accurately.
      Always provide sources when available.

  runtime:
    type: template
    template:
      replicas: 2
      config:
        outputSchema:
          name: "qa-response"
          description: "Q&A response format"
          jsonSchema:
            type: object
            properties:
              answer:
                type: string
              sources:
                type: array
                items:
                  type: string
      resources:
        requests:
          cpu: "100m"
          memory: "128Mi"
```

## Operations

### Viewing Agents

```bash
# List all agents
kubectl get agents

# Get detailed information
kubectl describe agent my-agent

# Watch agent status
kubectl get agents -w
```

### Scaling Agents

```bash
# Scale up replicas
kubectl patch agent my-agent --type='json' \
  -p='[{"op": "replace", "path": "/spec/runtime/standard/replicas", "value": 5}]'
```

### Updating Agents

```bash
# Update the container image
kubectl patch agent my-agent --type='json' \
  -p='[{"op": "replace", "path": "/spec/runtime/standard/container/image", "value": "new-image:v2.0.0"}]'
```

### Debugging

```bash
# Get agent pods
kubectl get pods -l flokoa.ai/agent=my-agent

# View logs
kubectl logs -l flokoa.ai/agent=my-agent

# Follow logs from all replicas
kubectl logs -l flokoa.ai/agent=my-agent --all-containers=true -f

# Execute commands in agent pod
kubectl exec -it <pod-name> -- /bin/sh
```

## Best Practices

1. **Always set resource limits** to prevent agents from consuming excessive cluster resources
2. **Use health checks** (liveness and readiness probes) for reliable deployments
3. **Run multiple replicas** for high availability in production
4. **Use pod anti-affinity** to spread replicas across nodes/zones
5. **Declare the framework** explicitly for better observability
6. **Store secrets properly** using Kubernetes secrets, never in the Agent spec
7. **Use security contexts** to run containers as non-root with minimal privileges
8. **Version your images** with specific tags, avoid using `latest` in production
9. **Use Instructions** for system prompts instead of hardcoding in images
10. **Define A2A skills** in the card for agent discovery and interoperability

## Troubleshooting

### Agent Stuck in Pending

- Check if the container image exists and is accessible
- Verify sufficient cluster resources are available
- Check image pull secrets for private registries

### Agent Pods Crashing

- View pod logs: `kubectl logs <pod-name>`
- Check resource limits aren't too restrictive
- Verify environment variables and secrets are correct
- Check health probe configurations

### Performance Issues

- Monitor resource usage: `kubectl top pods -l flokoa.ai/agent=<name>`
- Adjust CPU/memory requests and limits
- Scale up replicas if needed
- Check for slow external API calls (tools)

### Networking Issues

- Verify service is created: `kubectl get svc -l flokoa.ai/agent=<name>`
- Check network policies aren't blocking traffic
- Verify ingress/load balancer configuration
- Test connectivity from within cluster
