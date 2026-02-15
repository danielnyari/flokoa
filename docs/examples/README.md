# Agent CR Examples

This directory contains example Custom Resources (CRs) for the Flokoa Agent operator.

## Examples

### [minimal-agent.yaml](minimal-agent.yaml)
The absolute minimum configuration required to deploy an agent. This example shows:
- Only required fields
- Default replica count (1)
- Single container with basic port configuration

**Use when:** You want to quickly test or deploy a simple agent without extra configuration.

### [basic-agent.yaml](basic-agent.yaml)
A practical production-ready configuration. This example includes:
- Framework declaration (`pydantic-ai`)
- Multiple replicas for high availability
- Environment variables
- Resource requests and limits
- Health checks (liveness and readiness probes)

**Use when:** You need a solid baseline for most production deployments.

### [advanced-agent.yaml](advanced-agent.yaml)
A comprehensive configuration showcasing all available features:
- Service account for RBAC
- Multiple container ports (HTTP + metrics)
- Secrets management
- Volume mounts (ConfigMap, EmptyDir)
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
- `spec.runtime.type` - Runtime backend type (currently only "standard" supported)
- `spec.runtime.spec.container.name` - Container name
- `spec.runtime.spec.container.image` - Container image

### Common Optional Fields
- `spec.framework` - Framework type (pydantic-ai, langchain, crewai, marvin, autogen, a2a, custom)
- `spec.runtime.spec.replicas` - Number of pod replicas (default: 1)
- `spec.runtime.spec.container.ports` - Container ports
- `spec.runtime.spec.container.env` - Environment variables
- `spec.runtime.spec.container.resources` - CPU/memory limits
- `spec.runtime.spec.container.volumeMounts` - Volume mount points
- `spec.runtime.spec.volumes` - Pod volumes
- `spec.runtime.spec.serviceAccountName` - Service account for RBAC
- `spec.runtime.spec.securityContext` - Pod security context
- `spec.runtime.spec.nodeSelector` - Node selection constraints
- `spec.runtime.spec.tolerations` - Pod tolerations
- `spec.runtime.spec.affinity` - Pod affinity rules

## Runtime Types

The `spec.runtime.type` field determines the backend used to run the agent:

- **`standard`** - Deploys agents using standard Kubernetes Deployments and Services. This is the default and currently the only supported runtime type.

Future runtime types may include:
- `knative` - Serverless deployment with Knative Serving (planned)
- `job` - One-time execution using Kubernetes Jobs (planned)

## Status Fields

The operator updates these status fields automatically:

- `status.phase` - Current phase (Pending, Running, Failed)
- `status.backend` - Backend implementation (core, knative)
- `status.url` - Service endpoint URL
- `status.replicas` - Current replica count
- `status.availableReplicas` - Ready replicas
- `status.conditions` - Standard Kubernetes conditions
- `status.observedGeneration` - Last observed spec generation

## Network Policies

See [`networkpolicy/`](networkpolicy/) for NetworkPolicy examples that secure agent pod traffic using a default-deny + allow-list strategy. Covers:

- Default deny all ingress/egress
- DNS resolution, LLM provider APIs, state backends, Kubernetes API server
- Ingress from the operator, server, and Argo Workflows

The Flokoa Helm chart also includes parameterized NetworkPolicy templates -- see `networkPolicy.agent` in `values.yaml`.

## Tips

1. **Start Simple**: Begin with `minimal-agent.yaml` and add fields as needed
2. **Set Resource Limits**: Always define CPU/memory limits to prevent resource contention
3. **Use Health Checks**: Liveness and readiness probes ensure reliable deployments
4. **Security First**: Use `securityContext` to run containers as non-root with read-only filesystems
5. **High Availability**: Use `replicas > 1` with anti-affinity rules for production
6. **Network Security**: Apply NetworkPolicies to restrict agent pod traffic in production
