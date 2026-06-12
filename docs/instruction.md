# Instruction

The `Instruction` resource is a **versioned, shareable system-prompt block**. One Instruction
can be referenced by many [Agents](agent.md); editing it recompiles and rolls out every Agent
that references it — prompt reuse, versioning, and canary rollout without touching each agent.

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Instruction
metadata:
  name: support-policy
spec:
  content: |
    You are a friendly customer service agent. Be empathetic and concise.
    Never promise refunds without checking the policy first.
```

## Spec

| Field | Description |
|-------|-------------|
| `content` | The system-prompt text. Required. |

## How it compiles

An Agent references Instructions via `spec.instructionRefs` (a list). During compilation the
controller appends each referenced Instruction's `content`, **in declared order**, into the
resolved [AgentSpec](agent.md)'s `instructions`, followed by any inline
`spec.spec.instructions`. The merged result is delivered to the generic runner as part of the
compiled-spec ConfigMap — see the [runtime contract](reference/runtime-contract.md).

## Status

The controller writes the prompt to a ConfigMap and surfaces it on the status:

| Field | Description |
|-------|-------------|
| `status.configMapName` | Name of the generated ConfigMap holding the prompt. |
| `status.conditions` | Standard conditions (e.g. `Ready`). |
| `status.observedGeneration` | The spec generation last reconciled. |

## See also

- [Agent](agent.md) — references Instructions via `instructionRefs`
- [Model](model.md) · [AgentTool](agenttool.md) — the other composable building blocks
