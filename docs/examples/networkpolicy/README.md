# NetworkPolicy Examples for Flokoa Agent Pods

These examples demonstrate how to secure network traffic to and from agent pods managed by the Flokoa operator using Kubernetes NetworkPolicies.

## Strategy: Default Deny + Allow List

The recommended approach is to start with a default-deny policy and then layer on specific allow rules for the traffic your agents need.

## Examples

### [default-deny.yaml](default-deny.yaml)
Denies all ingress and egress traffic to agent pods by default. Apply this first, then add specific allow policies.

**Use when:** You want to follow the principle of least privilege for all agent pods.

### [allow-egress-dns.yaml](allow-egress-dns.yaml)
Allows DNS resolution (port 53) to kube-dns/CoreDNS. Required for agents to resolve any external hostnames.

**Use when:** Always apply this alongside the default-deny policy.

### [allow-egress-llm-providers.yaml](allow-egress-llm-providers.yaml)
Allows HTTPS egress (port 443) for reaching LLM provider APIs (OpenAI, Anthropic, Google, etc.). Standard Kubernetes NetworkPolicy does not support hostname-based rules, so this allows all HTTPS egress by default. For tighter control, use CIDR blocks or a DNS-aware policy engine like Cilium.

**Use when:** Agents need to call external LLM APIs.

### [allow-egress-state-backends.yaml](allow-egress-state-backends.yaml)
Allows egress to in-cluster Redis (6379) and PostgreSQL (5432) for agents that use external state storage.

**Use when:** Agents are configured with `redis`, `postgres`, or `s3` state backends.

### [allow-egress-kube-apiserver.yaml](allow-egress-kube-apiserver.yaml)
Allows egress to the Kubernetes API server for agents that need service discovery or ConfigMap access.

**Use when:** Agents need to interact with the Kubernetes API.

### [allow-ingress-operator.yaml](allow-ingress-operator.yaml)
Allows ingress from the Flokoa controller for health check probes.

**Use when:** Always apply this so the operator can perform health checks.

### [allow-ingress-argo.yaml](allow-ingress-argo.yaml)
Allows ingress from the Argo Workflows namespace for workflow orchestration via the A2A executor plugin.

**Use when:** You use Argo Workflows to orchestrate agent tasks.

### [allow-ingress-server.yaml](allow-ingress-server.yaml)
Allows ingress from the Flokoa server for gRPC/HTTP API proxying.

**Use when:** Agents are accessed through the Flokoa API server.

## Quick Start

Apply the default deny and the essential allow policies:

```bash
# 1. Default deny all traffic
kubectl apply -f docs/examples/networkpolicy/default-deny.yaml

# 2. Allow DNS (required)
kubectl apply -f docs/examples/networkpolicy/allow-egress-dns.yaml

# 3. Allow operator health checks (required)
kubectl apply -f docs/examples/networkpolicy/allow-ingress-operator.yaml

# 4. Allow LLM API access
kubectl apply -f docs/examples/networkpolicy/allow-egress-llm-providers.yaml

# 5. Allow server access (if using the Flokoa API server)
kubectl apply -f docs/examples/networkpolicy/allow-ingress-server.yaml
```

## Helm Chart Integration

The Flokoa Helm chart includes parameterized NetworkPolicy templates. Enable them in your `values.yaml`:

```yaml
networkPolicy:
  agent:
    enabled: true
    ingress:
      operator: true
      server: true
      argo: true      # Only applied when argo.enabled is true
    egress:
      llmProviders:
        enabled: true
        cidrs: []      # Empty = allow all HTTPS; set CIDRs for tighter control
      kubeApiServer:
        enabled: false
        cidr: "10.96.0.1/32"
      stateBackends:
        enabled: false
```

## Notes

- These policies use `app.kubernetes.io/managed-by: flokoa-operator` to select agent pods, which matches all pods created by the Flokoa operator.
- Kubernetes NetworkPolicy does not support FQDN/hostname-based rules. For hostname filtering, use Cilium CiliumNetworkPolicy or a service mesh.
- Policies are additive: multiple NetworkPolicies targeting the same pods combine their allow rules.
- The default-deny policy only takes effect when a CNI plugin that supports NetworkPolicy is installed (e.g., Calico, Cilium, Weave Net).
