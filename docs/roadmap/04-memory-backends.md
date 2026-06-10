# 04 — Memory Backends + `Agent.spec.memory`

**Phase:** 1 · **Size:** M · **Depends on:** 03 · **Enables:** the "sessions and memory" harness claim; 10 (docs)

## Goal

Durable, declarative conversation memory: a `memory` block on the Agent CRD, a Redis `SessionStore` adapter, and the operator wiring that turns one into the other. After this unit, agent pods are disposable and horizontally scalable without losing conversations.

## Current state

- Unit 03 ships `SessionStore` + `InMemorySessionStore` + `FLOKOA_SESSION_*` env settings; the managed runtime reads them, but nothing sets them in-cluster.
- `AgentSpec` (`operator/api/v1alpha1/agent_types.go:126`) has `CardOverride`, `Runtime`, `Model`, `Instruction`, `Framework`, `Tools` — no memory/session field.
- The builder (`operator/internal/infra/builder/deployment.go`) constructs the template-runtime Deployment; `TemplatedRuntimeSpec.Env` already supports arbitrary env injection (escape hatch, not the product surface).
- Established CRD patterns to follow: discriminated union with per-type optional blocks (`ModelProviderSpec`: `openai`/`anthropic`/… + inferred type), credentials via `*corev1.SecretKeySelector` (`APIKeySecretRef`), TLS via the existing `TLSConfig` type (`modelprovider_types.go:106`), validation in `api/v1alpha1/agent_webhook.go`.

## Target design

### CRD

```go
// api/v1alpha1/agent_types.go

// MemoryBackendType represents the session memory backend.
// +kubebuilder:validation:Enum=inMemory;redis
type MemoryBackendType string

const (
    MemoryBackendInMemory MemoryBackendType = "inMemory"
    MemoryBackendRedis    MemoryBackendType = "redis"
)

// AgentMemorySpec configures conversation session memory for the agent.
type AgentMemorySpec struct {
    // Type selects the backend. inMemory is per-pod and non-durable.
    // +kubebuilder:default=inMemory
    Type MemoryBackendType `json:"type"`

    // TTLSeconds is the idle expiry for sessions.
    // +kubebuilder:default=3600
    // +kubebuilder:validation:Minimum=60
    // +optional
    TTLSeconds *int32 `json:"ttlSeconds,omitempty"`

    // MaxTurns caps how many stored run-batches are replayed per request.
    // +kubebuilder:default=50
    // +kubebuilder:validation:Minimum=1
    // +optional
    MaxTurns *int32 `json:"maxTurns,omitempty"`

    // Redis configuration. Required when type is "redis".
    // +optional
    Redis *RedisMemorySpec `json:"redis,omitempty"`
}

type RedisMemorySpec struct {
    // Address is the host:port of the Redis endpoint.
    // +kubebuilder:validation:MinLength=1
    Address string `json:"address"`
    // +optional
    Database int32 `json:"database,omitempty"`
    // PasswordSecretRef references a secret key holding the Redis password.
    // +optional
    PasswordSecretRef *corev1.SecretKeySelector `json:"passwordSecretRef,omitempty"`
    // +optional
    TLS *TLSConfig `json:"tls,omitempty"`
    // KeyPrefix namespaces keys; defaults to "flokoa:sess:{namespace}:{agent}:".
    // +optional
    KeyPrefix string `json:"keyPrefix,omitempty"`
}
```

`AgentSpec` gains `Memory *AgentMemorySpec \`json:"memory,omitempty"\`` (+optional). Applies to `runtime.type: template` only; for `standard` runtimes it's the user's image's job — webhook warns (admission.Warnings) if set with standard runtime.

### Operator projection (env, not files)

Memory config is connection info + tuning — env vars are the right channel (12-factor; secret via `valueFrom`, never materialized — tenet 4). In the app layer (`internal/app/agent/reconcile.go`), translate `spec.memory` into env vars appended to the builder's `DeploymentParams`:

| CRD | Env |
|---|---|
| `type` | `FLOKOA_SESSION_BACKEND` (`inMemory`→`memory`) |
| `ttlSeconds` / `maxTurns` | `FLOKOA_SESSION_TTL_SECONDS` / `FLOKOA_SESSION_MAX_TURNS` |
| `redis.address`, `.database`, `.keyPrefix` | `FLOKOA_SESSION_REDIS_ADDRESS` / `_DATABASE` / `_KEY_PREFIX` |
| `redis.passwordSecretRef` | `FLOKOA_SESSION_REDIS_PASSWORD` via `EnvVar.ValueFrom.SecretKeyRef` |
| `redis.tls` | `FLOKOA_SESSION_REDIS_TLS=true`, CA via mounted secret volume + `FLOKOA_SESSION_REDIS_CA_FILE` |

Keep `BuildDeployment` pure: compute the `[]corev1.EnvVar` in the app layer (new `memory_env.go` beside the existing sub-reconcilers), pass through `DeploymentParams`. Watch wiring: `SetupWithManager` already `Watches` Secrets via `findAgentsForSecret` — extend that mapper to index `memory.redis.passwordSecretRef` so password rotation triggers reconcile.

### Python adapter

- `flokoa/sessions/redis.py`: `RedisSessionStore` on `redis.asyncio` — `RPUSH key blob` + `EXPIRE key ttl` per append (pipeline), `LRANGE key -maxturns -1` on load, `DELETE` on clear. Key = `{prefix}{session_id}`.
- New optional extra in `flokoa/pyproject.toml`: `redis = ["redis>=5.0"]`; managed-agent depends on `flokoa[pydantic-ai,redis]` so the operator image always has it. Register in the 03 registry via `_try_load`-style import.

### Status

Add a `MemoryConfigured` `metav1.Condition` on `AgentStatus.Conditions` (operator validates shape, not Redis liveness — the runtime logs/fails readiness on connection errors; probe wiring is unit 13).

## Implementation plan

1. CRD types + `SchemeBuilder` untouched (same file) → `make manifests generate`; sync chart CRDs (01's target).
2. Webhook validation in `agent_webhook.go`: `redis` block required iff `type: redis`; reject `memory` with unknown fields; warning on standard runtime.
3. App layer `memory_env.go` + builder `DeploymentParams` extension + secret watch mapper extension. Unit-test with `repo/fakes` (assert rendered env on `FakeDeploymentRepo`).
4. `make generate-python-models` — confirm the agent CRD schema additions flow into `flokoa-types` generated models (and extend the unified `LlmAgentConfig` in `flokoa/config/agent_config.py` with an optional `session` block for the day the operator writes `agent-config.json`; env remains the active channel).
5. `RedisSessionStore` + extra + registry entry + managed-agent dependency bump.
6. Sample CR in `operator/config/samples/` and `docs/examples/` (agent with Redis memory + secret).

## Testing

- Go: envtest controller case — Agent with `memory.redis` produces Deployment env incl. `SecretKeyRef`; webhook rejection cases (Ginkgo, alongside the ~167 existing controller cases).
- Python: `RedisSessionStore` against `fakeredis` (dev dep) — append/load/TTL/truncation parity with the in-memory adapter via a shared conformance test parametrized over both stores (the port's contract test — write once, run per adapter).
- E2E (Kind, existing `test/e2e`): Redis from a one-pod manifest; Agent with redis memory; two-turn conversation; **delete the agent pod between turns**; assert recall after reschedule. This test *is* the durability claim.

## Acceptance criteria

- `kubectl apply` of the sample agent + secret yields a runtime whose sessions survive pod deletion and scale-out (`replicas: 2` with shared Redis behaves correctly thanks to append-only writes).
- Invalid specs rejected at admission with actionable messages.
- Full pipeline artifacts committed: CRD YAML, generated deepcopy, generated Python types.

## Out of scope

- Postgres adapter (same port; follow-up). Semantic/long-term memory. Argo A2A plugin passing user-chosen contextIds into workflow agent calls (today the plugin controls task creation — note as a small follow-up: thread `AgentCall` contextId through `plugins/a2a`).
