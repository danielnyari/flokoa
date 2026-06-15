# Agent CR Examples

This directory contains example Custom Resources (CRs) for the Flokoa Agent operator.

## Examples

### [agent/minimal-agent.yaml](agent/minimal-agent.yaml)
The absolute minimum: a card, an inline model, and instructions. No image,
no build — the operator compiles the spec and runs it on the generic runner.

**Use when:** You want to quickly test or deploy a simple agent.

### [agent/basic-agent.yaml](agent/basic-agent.yaml)
The composition shape: shared Model and Instruction resources, an MCP tool,
an inline fragment, and secret-backed placeholders. Rotate the Model CR and
every referencing agent recompiles and rolls.

**Use when:** You need the fleet-managed baseline for production deployments.

### [agent/advanced-agent.yaml](agent/advanced-agent.yaml)
The custom-image escape hatch plus scheduling overrides:
- `runtime.image` replacing the generic runner
- Structured output schema
- Node selectors and tolerations
- Security contexts (pod and container level)
- Image pull secrets
- Node scheduling (selectors, tolerations, affinity)
- Advanced health check configurations

**Use when:** You need fine-grained control over scheduling, security, and observability.

### Capability examples

Capabilities across the [source tiers](../capability.md#source-tiers) and Agents
that attach them. Together they show the shapes `flokoa capability build`/`push`
produce and how an Agent consumes them; see the
[capabilities guide](../guides/capabilities.md) for how to author, build, and
publish one (or attach a built-in with no build at all).

#### [capability/echo-capability.yaml](capability/echo-capability.yaml)
A digest-pinned Capability with a typed (`schemaPolicy: strict`) config schema,
mirroring the `echo` fixture
(`operator/test/e2e/fixtures/capabilities/echo/`). The `configSchema` is what
attaching agents validate their per-capability `config` against at admission.

**Use when:** You want to see the published Capability CR shape.

#### [capability/agent-with-capability.yaml](capability/agent-with-capability.yaml)
An Agent attaching the echo Capability by name with per-agent `config`
(`prefix: "agent-echo"`), validated against the capability's schema before any
pod starts.

**Use when:** You want to attach a published capability to an agent.

#### [capability/builtin-openapi-agent.yaml](capability/builtin-openapi-agent.yaml)
An Agent attaching the **built-in** `flokoa-openapi` capability (source tier
[`builtin`](../capability.md#source-tiers)) — baked into the runner image, so
there is no `Capability` artifact to publish and nothing is downloaded. Just a
`ref` by name plus per-agent `config`.

**Use when:** You want to use a first-party capability with zero build/publish
step.

#### [capability/git-sourced-capability.yaml](capability/git-sourced-capability.yaml)
A `Capability` built from a private git repo (source tier
[`git`](../capability.md#source-tiers)): a normal digest-pinned artifact with
recorded `provenance.git` (clean repo URL + resolved commit). Nothing is fetched
from git at deploy/run time.

**Use when:** You want to see the shape `flokoa capability build --from-git`
produces, or to refuse non-git sources via `allowedSources`.

## Applying Examples

```bash
# Apply directly
kubectl apply -f docs/examples/agent/basic-agent.yaml

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
- `spec.card` - The published A2A agent card (name, description, version, skills)
- A model: either `spec.modelRef` (a Model resource) or `spec.spec.model`
  (an inline pydantic-ai model identifier like `openai:gpt-5-mini`)

### Composition Fields
- `spec.modelRef` - References a Model resource (inline `spec.spec.model` wins conflicts)
- `spec.instructionRefs` - Instruction resources, composed in order before inline instructions
- `spec.tools` - AgentTool resources (declarative MCP endpoints)
- `spec.spec` - Inline pydantic-ai AgentSpec fragment (instructions, modelSettings,
  outputSchema, native capabilities, extra passthrough)
- `spec.secretRefs` - Named secrets resolvable via `${secret:NAME}` placeholders

### Runtime Fields
- `spec.runtime.image` - Custom image escape hatch (default: the generic runner)
- `spec.runtime.runnerVersion` - Pins a runner release
- `spec.runtime.isolation` - Session isolation tier (`shared` today; `session` is P1, not yet shipped)
- `spec.runtime.replicas` - Number of pod replicas (default: 1)
- `spec.runtime.env` / `resources` / `serviceAccountName` / `securityContext` /
  `nodeSelector` / `tolerations` / `affinity` - Pod-level configuration

The compiled spec lands in the `<agent>-agent-spec` ConfigMap; `kubectl get
agent <name> -o jsonpath='{.status.specHash}'` shows the resolved-spec hash.
