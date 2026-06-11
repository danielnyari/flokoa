# Model

A `Model` is a named, shareable model configuration: the model identifier
plus typed settings, backed by a [ModelProvider](modelprovider.md) for
connection configuration (API key secret, base URL). It compiles into the
agent's resolved AgentSpec as `model` + `model_settings`.

This is the fleet-management lever: rotate one Model CR and every Agent that
references it recompiles, its `specHash` changes, and the rollout follows.

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: gpt-4o
spec:
  model: "gpt-4o"
  providerRef:
    name: openai-provider
  settings:
    temperature: "0.7"
    maxTokens: 4096
    topP: "0.9"
```

Agents reference it via `spec.modelRef`. The compiled model identifier is
prefixed by the provider type (`openai:gpt-4o`); identifiers that already
carry a prefix are used as-is.

## Spec fields

| Field | Description |
|---|---|
| `model` | Model identifier (e.g. `gpt-4o`, `claude-sonnet-4-5`). |
| `providerRef` | The [ModelProvider](modelprovider.md) supplying connection config. |
| `settings` | Typed model settings — see below. |

### Settings

`settings` mirrors pydantic-ai's common `ModelSettings` with typed fields.
Decimal fields are strings (Kubernetes API convention — no floating-point
drift); the compiler emits JSON numbers.

| Field | Type | Compiled key |
|---|---|---|
| `maxTokens` | int | `max_tokens` |
| `temperature` | string ("0.0"–"2.0") | `temperature` |
| `topP` | string ("0.0"–"1.0") | `top_p` |
| `topK` | int | `top_k` |
| `timeoutSeconds` | int | `timeout` |
| `parallelToolCalls` | bool | `parallel_tool_calls` |
| `seed` | int | `seed` |
| `presencePenalty` | string ("-2.0"–"2.0") | `presence_penalty` |
| `frequencyPenalty` | string ("-2.0"–"2.0") | `frequency_penalty` |
| `logitBias` | map | `logit_bias` |
| `stopSequences` | list | `stop_sequences` |
| `extraHeaders` | map | `extra_headers` |

### Provider-specific knobs (`settings.extra`)

Anything beyond the common fields passes through `extra` and merges into the
compiled `model_settings` object as-is — use pydantic-ai's provider-prefixed
settings keys:

```yaml
settings:
  temperature: "0.7"
  extra:
    openai_reasoning_effort: high
    extra_body:
      service_tier: default
```

```yaml
settings:
  maxTokens: 8192
  extra:
    anthropic_thinking:
      type: enabled
      budget_tokens: 4096
```

Typed fields win key conflicts; the admission webhook rejects `extra` keys
that shadow a typed field. The compiled spec is validated against the
runner's pinned AgentSpec schema, so unknown settings still surface as
`SpecValid=False` on the Agent, never as a crashing pod.

## Settings precedence

An Agent's inline `spec.spec.modelSettings` merges **per-key over** the
Model's settings (inline wins conflicts; unset keys inherit).

## Status

The Model controller resolves the provider and surfaces `status.ready`,
`status.resolvedProvider`, and conditions. Agents only compile against ready
Models.
