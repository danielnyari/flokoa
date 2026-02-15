# Agent Resource

The `Agent` resource is the core of Flokoa, representing a deployable AI agent in your Kubernetes cluster.

## Overview

An Agent defines:
- The container image running your AI agent code
- Runtime configuration (replicas, resources, health checks)
- Optional model configuration for LLM access
- Optional tools the agent can use
- Framework declaration for observability

## Basic Structure

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: my-agent
spec:
  runtime:
    type: standard
    spec:
      container:
        name: agent
        image: your-agent-image:tag
```

## Spec Fields

### Card (Agent Metadata)

The `card` field contains metadata about the agent following the A2A (Agent-to-Agent) protocol:

```yaml
spec:
  card:
    name: "Customer Support Agent"
    description: "Handles customer inquiries and support requests"
    version: "1.0.0"
    
    # Input/output modes
    defaultInputModes:
      - "application/json"
    defaultOutputModes:
      - "application/json"
    
    # Capabilities
    capabilities:
      streaming: true
      pushNotifications: false
      stateTransitionHistory: true
    
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
```

### Runtime Configuration

The `runtime` field defines how the agent is deployed:

```yaml
spec:
  runtime:
    type: standard  # 'standard' or 'template'
    spec:
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
```

### Framework Declaration

Explicitly declare which AI framework your agent uses:

```yaml
spec:
  framework: pydantic-ai  # or: langchain, google-adk, crewai, marvin, autogen, a2a
```

Supported frameworks:
- `pydantic-ai` - Pydantic AI framework
- `langchain` - LangChain framework
- `google-adk` - Google Agent Development Kit
- `crewai` - CrewAI framework
- `marvin` - Marvin AI framework
- `autogen` - Microsoft AutoGen
- `a2a` - Agent-to-Agent protocol

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
  
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2026-01-15T10:30:00Z"
      reason: DeploymentAvailable
      message: "Agent is running and available"
```

## Examples

### Minimal Agent

The absolute minimum configuration:

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: minimal-agent
spec:
  runtime:
    type: standard
    spec:
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
  framework: pydantic-ai
  
  model:
    name: gpt-4o-model
  
  runtime:
    type: standard
    spec:
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
          timeoutSeconds: 5
          failureThreshold: 3
        
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
          timeoutSeconds: 3
          successThreshold: 1
          failureThreshold: 3
        
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 1000
          capabilities:
            drop:
            - ALL
      
      # High availability configuration
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
  framework: langchain
  
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
    
    # Another external tool
    - toolRef:
        name: email-sender
        namespace: shared-tools
  
  runtime:
    type: standard
    spec:
      container:
        name: agent
        image: ghcr.io/example/langchain-agent:v1.0.0
        ports:
        - containerPort: 8080
          name: http
```

### Agent with Volume Mounts

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: agent-with-volumes
spec:
  runtime:
    type: standard
    spec:
      container:
        name: agent
        image: ghcr.io/example/agent:v1.0.0
        
        ports:
        - containerPort: 8080
          name: http
        
        volumeMounts:
        - name: config
          mountPath: /etc/agent
          readOnly: true
        - name: cache
          mountPath: /var/cache/agent
        - name: models
          mountPath: /models
          readOnly: true
      
      volumes:
      - name: config
        configMap:
          name: agent-config
      - name: cache
        emptyDir:
          sizeLimit: 1Gi
      - name: models
        persistentVolumeClaim:
          claimName: model-cache-pvc
```

### Agent with Advanced Scheduling

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: scheduled-agent
spec:
  runtime:
    type: standard
    spec:
      replicas: 2
      
      container:
        name: agent
        image: ghcr.io/example/agent:v1.0.0
        ports:
        - containerPort: 8080
          name: http
      
      # Service account for RBAC
      serviceAccountName: agent-sa
      
      # Node selection
      nodeSelector:
        workload-type: ai-agents
        node.kubernetes.io/instance-type: m5.xlarge
      
      # Tolerations for tainted nodes
      tolerations:
      - key: "ai-workload"
        operator: "Equal"
        value: "true"
        effect: "NoSchedule"
      - key: "gpu"
        operator: "Exists"
        effect: "NoSchedule"
      
      # Pod affinity rules
      affinity:
        # Prefer nodes with GPUs
        nodeAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            preference:
              matchExpressions:
              - key: nvidia.com/gpu
                operator: Exists
        
        # Don't schedule multiple replicas on same node
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchLabels:
                flokoa.ai/agent: scheduled-agent
            topologyKey: kubernetes.io/hostname
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
  -p='[{"op": "replace", "path": "/spec/runtime/spec/replicas", "value": 5}]'
```

### Updating Agents

```bash
# Update the container image
kubectl patch agent my-agent --type='json' \
  -p='[{"op": "replace", "path": "/spec/runtime/spec/container/image", "value": "new-image:v2.0.0"}]'
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
9. **Monitor your agents** through metrics endpoints and logging
10. **Test with minimal config first** then add complexity as needed

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
