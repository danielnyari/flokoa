# Agent

The `Agent` resource is the **composition root** of a pydantic-ai agent: it
references shared building blocks ([Model](model.md),
[Instruction](instruction.md), [AgentTool](agenttool.md)) and an optional
inline AgentSpec fragment. The operator **compiles** them into one resolved
[pydantic-ai AgentSpec](https://ai.pydantic.dev/), validates it against the
runner's pinned AgentSpec JSON Schema, and delivers it to the generic runner
as a single ConfigMap — no image builds, no CI pipeline: edit YAML, behavior
changes.

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: support-agent
spec:
  card:
    name: support-agent
    description: "Answers support questions"
    version: "1.0.0"
    skills:
      - id: support
        name: Support
        description: "Answers support questions from the KB"
        tags: [support]

  modelRef:
    name: gpt-4o
  instructionRefs:
    - name: support-policy
  tools:
    - name: search-knowledge-base

  spec:
    instructions:
      - "Sign off as 'Support'."
    modelSettings:
      temperature: "0.2"
    capabilities:
      - name: WebSearch

  secretRefs:
    kb-token:
      name: kb-credentials
      key: token

  runtime:
    replicas: 2
```

## How composition works

The controller is a compiler. Merge precedence is deterministic and
documented:

1. Referenced CRs compose **in declared order**.
2. Inline fragment **scalars win** conflicts (e.g. `spec.spec.model` beats
   `modelRef`'s model string).
3. **List fields append**: `instructionRefs` content comes first, then the
   fragment's `instructions`; AgentTool-derived MCP capability entries are
   appended after the fragment's `capabilities`; platform-injected
   capabilities always come last.

The compiled spec is validated against the AgentSpec JSON Schema of the
agent's runner version **before** any Deployment update. A bad composition
surfaces as a `SpecValid=False` condition with a precise error path — the
last good spec keeps running. The resolved-spec hash is surfaced in
`status.specHash` and rolls the Deployment whenever any part of the
composition graph changes: rotate one Model CR and every referencing Agent
recompiles and follows.

## Spec fields

| Field | Description |
|---|---|
| `card` | The published A2A agent card (name, description, version, modes, capabilities, skills). Required. |
| `modelRef` | References a [Model](model.md). The fragment's inline `model` wins if both are set. |
| `instructionRefs` | [Instruction](instruction.md) resources composed in order before inline instructions. |
| `tools` | [AgentTool](agenttool.md) references (declarative MCP endpoints), compiled to MCP capability entries. |
| `capabilities` | [Capability](capability.md) CR attachments — each is a `ref` (name/namespace) plus optional per-attachment `config`. At admission the config is validated against the Capability's published JSON Schema, the `requires` tuple is checked against the agent's runner version, and dependency conflicts across attachments are rejected. |
| `spec` | Inline pydantic-ai AgentSpec fragment — see below. |
| `secretRefs` | Named secrets resolvable via `${secret:NAME}` placeholders (see [secrets](#secrets)). Keys must match `[A-Za-z0-9._-]+`. |
| `runtime` | How the compiled spec runs — see below. |

### The agent card (`spec.card`)

The card is the agent's published [A2A](https://a2a-protocol.org/) identity. It
is **required**, and `name`, `description`, `version`, and `skills` within it
are required too.

| Field | Description |
|---|---|
| `name`, `description`, `version` | Card identity. Required. |
| `skills` | What the agent advertises. Required (at least one). Each skill: `id`, `name`, `description`, and `tags` are required; `examples`, `inputModes`, `outputModes`, and `security` are optional. Skill `id`s must be unique. |
| `defaultInputModes` | MIME types the agent accepts. Defaults to `["application/json"]`. Allowed values: `text`, `application/json`. |
| `defaultOutputModes` | MIME types the agent produces. Defaults to `["application/json"]`. Same allowed values. |
| `capabilities` | A2A card capabilities — `streaming`, `pushNotifications`, `stateTransitionHistory`, `extensions`. Defaults to `{streaming: false}`. *(Distinct from `spec.capabilities`, which attaches [Capability](capability.md) CRs.)* |

### The inline fragment (`spec.spec`)

Typed where pydantic-ai is stable, with an `extra` passthrough for additive
upstream fields:

| Field | Compiles to |
|---|---|
| `model` | AgentSpec `model` (e.g. `openai:gpt-5-mini`) |
| `name`, `description` | AgentSpec `name` / `description` |
| `instructions` | Appended after all `instructionRefs` content |
| `modelSettings` | Merged per-key over the Model's settings (inline wins) |
| `outputSchema` | AgentSpec `output_schema` (structured output) |
| `capabilities` | Native pydantic-ai capability entries by name + config (`WebSearch`, `WebFetch`, `MCP`, `Thinking`, `ToolSearch`, …). Harness/third-party class paths are rejected — those ship as Capability resources. |
| `extra` | Additional top-level AgentSpec fields (typed fields win conflicts) |

### Runtime (`spec.runtime`)

| Field | Description |
|---|---|
| `image` | Custom-image escape hatch. Custom images own their bootstrap and must honor the [runtime contract](reference/runtime-contract.md). |
| `runnerVersion` | Pins a runner release (default: the operator's current). |
| `isolation` | `shared` (default). `session` ships with the session router (P1) and is rejected until then. |
| `replicas`, `env`, `resources`, `imagePullSecrets`, `serviceAccountName`, `securityContext`, `nodeSelector`, `tolerations`, `affinity` | Pod-level configuration. User `env` wins name conflicts with operator-injected variables. |

## Secrets

`${secret:NAME}` placeholders may appear in any string of the fragment (e.g.
MCP headers). Each `NAME` must have a matching `spec.secretRefs[NAME]` entry;
the operator projects it as a `FLOKOA_SECRET_*` environment variable and the
runner resolves placeholders at hydration. **Secret values never appear in
the compiled spec ConfigMap.**

## Admission checks

The validating webhook checks the whole composition *statically*, before any
Deployment is touched — a rejected Agent never reaches the runner:

- Inline-fragment `capabilities` names must be **baseline-native** (no `.`, `/`,
  or `:` in the name); harness/third-party capabilities are attached as
  [Capability](capability.md) CRs instead.
- Every `${secret:NAME}` placeholder — in the fragment *and* in capability
  attachment configs — must have a matching `spec.secretRefs[NAME]` entry.
- `spec.secretRefs` keys must match `[A-Za-z0-9._-]+`.
- Card skill `id`s must be unique.
- Capability attachments: `config` is validated against the Capability's
  published JSON Schema, the Capability's `requires` tuple is checked against
  the agent's runner version, and dependency conflicts (across attachments and
  the runner baseline) are rejected. Cross-namespace and duplicate attachments,
  and entry-name collisions, are rejected too.
- `runtime.isolation: session` is rejected until the session router ships (P1).

## Status

| Field | Description |
|---|---|
| `status.phase` | `Pending`, `Running`, or `Failed`. |
| `status.url` | The published endpoint. Treat as opaque — port, path, and backing topology may change behind it. |
| `status.specHash` | Hash of the resolved spec (drift detection, rollout trigger). |
| `status.runnerVersion` | Runner release the spec was validated against. |
| `status.injectedCapabilities` | Platform capability entries the operator appended. |
| `status.replicas` / `status.availableReplicas` | Desired vs. ready runner pods (mirrors the Deployment). |
| `status.observedGeneration` | The spec generation last reconciled. |
| Conditions | `SpecValid` (composition compiled + schema-valid), `SecretsReady` (referenced secrets exist), `Ready` (deployment available). |

## What the operator creates

- ConfigMap `<agent>-agent-spec` with `agent-spec.yaml` (the compiled spec,
  placeholders intact) and `agent-card.json`, mounted at `/etc/flokoa/`.
- A Deployment running the generic runner image
  (`ghcr.io/danielnyari/flokoa-runner:<runnerVersion>` unless overridden).
- The published Service backing `status.url`.
