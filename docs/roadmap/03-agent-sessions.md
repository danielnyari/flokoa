# 03 — Conversation Sessions in the Agent Runtime

**Phase:** 1 · **Size:** L · **Depends on:** — · **Enables:** 04 (memory backends), 12 (usage accounting)

## Goal

Make agent invocations stateful: two A2A messages sharing a `contextId` see the same conversation history. This is the single biggest gap between Flokoa and the harness definition ("every harness session is stateful by default"). This unit delivers the runtime mechanics with an in-process store; unit 04 adds durable backends and the CRD surface.

## Current state

- **Agents are stateless.** `PydanticAIAgentExecutor.execute()` (`sdk/python/flokoa/src/flokoa/integrations/pydantic_ai/agent_executor.py`) calls `await self.agent.run(request, **run_kwargs)` with `toolsets`, `model`, and `instructions` kwargs — never `message_history`. Each message starts a fresh conversation. Same in `TemplatedPydanticAIAgentExecutor.execute()` (`flokoa-managed-agent/src/flokoa_managed_agent/agent_executor.py`).
- **Session identity already exists in the protocol.** The A2A `contextId` flows through `RequestContext.current_task.context_id` and is already echoed into `TaskArtifactUpdateEvent(context_id=task.context_id, …)` / `TaskStatusUpdateEvent`. Server side, `internal/server/trigger_session.go: ExtractSessionKey()` already derives deterministic 32-char context IDs from trigger events. Nothing consumes them.
- **The only stores are ephemeral**: `InMemoryTaskStore()` (a2a-sdk task records, wired in both `flokoa/__main__.py:_get_app` and `flokoa_managed_agent/__main__.py:main`) and `ConfigCache` (`flokoa/src/flokoa/cache.py`, TTL/mtime cache for tool/model config — not conversation state).
- **pydantic-ai provides the primitives** (verified against pydantic-ai ≥ 1.44, the pinned floor): `agent.run(…, message_history=list[ModelMessage])`, `result.new_messages_json() -> bytes`, and `ModelMessagesTypeAdapter` for deserialization. Crucially, the executor passes the system prompt via the `instructions` kwarg, and **instructions are re-sent on every run regardless of history** (unlike `system_prompt`, which is skipped when `message_history` is non-empty) — so the existing instruction wiring composes correctly with history.

## Target design

### Port: `SessionStore`

New module `flokoa/src/flokoa/sessions/` in the `flokoa` package (shared by CLI serving and the managed runtime):

```python
# flokoa/sessions/store.py
class SessionStore(Protocol):
    """Append-only conversation history, keyed by A2A contextId."""
    async def load(self, session_id: str) -> list[bytes]:
        """All message-batch blobs for the session, oldest first. [] if unknown."""
    async def append(self, session_id: str, blob: bytes) -> None:
        """Append one run's new-messages blob; refreshes the session TTL."""
    async def clear(self, session_id: str) -> None: ...
```

**Append-only batches** (one blob = one run's `new_messages_json()`) is the load-bearing decision: it maps to atomic `RPUSH` on Redis and `INSERT` on SQL, eliminating read-modify-write races between replicas without locks or versioning. Serialization stays inside the pydantic-ai executor (`ModelMessagesTypeAdapter`); the store handles opaque bytes so it works for future frameworks.

### Adapters (this unit)

- `InMemorySessionStore` — `dict[str, list[tuple[float, bytes]]]` + lazy TTL eviction on access and a periodic sweep task. Default when sessions are enabled without a backend. Documented as single-replica only.
- Registry mirroring the integrations pattern: `flokoa/sessions/__init__.py` exposes `create_session_store(settings) -> SessionStore | None` dispatching on backend name, with `_try_load`-style optional imports so backend deps stay optional extras (Redis arrives in 04 and just registers here).

### Settings

```python
# flokoa/sessions/settings.py — pydantic-settings or os.environ, matching house style
FLOKOA_SESSION_BACKEND   = "" | "memory" | "redis"   # "" = sessions disabled (current behavior)
FLOKOA_SESSION_TTL_SECONDS = 3600
FLOKOA_SESSION_MAX_TURNS   = 50                       # max stored blobs replayed per run
```

Env-var configuration makes this unit fully testable via `flokoa run` with zero operator changes; 04 makes the operator set these from the CRD.

### Executor integration

In `PydanticAIAgentExecutor.execute()` (the templated executor inherits this):

1. Resolve `session_id`: `context.current_task.context_id` if present, else the incoming message's `context_id` (the a2a-sdk sets one on `new_task()`); if no store configured → exact current behavior.
2. `blobs = await store.load(session_id)`; truncate to the **last** `FLOKOA_SESSION_MAX_TURNS` blobs; `history = [m for b in blobs for m in ModelMessagesTypeAdapter.validate_json(b)]`. A corrupt blob logs a warning and clears the session rather than failing the request (self-healing beats hard failure for cache-like state).
3. Pass `message_history=history` in `run_kwargs` when non-empty.
4. After a successful run: `await store.append(session_id, result.new_messages_json())`. Skip append on failure — a failed run must not poison the transcript.

Constructor gains `session_store: SessionStore | None = None` (dependency injection, matching the existing `cache`/`toolset_factory` params); `flokoa/__main__.py:_start_integration` and `flokoa_managed_agent/__main__.py:main` build it via `create_session_store()`.

### Concurrency note

Two simultaneous runs on one `contextId` will interleave batches; append-only means neither is lost and the transcript stays valid chronology. Per-session serialization (queueing) is explicitly deferred — document the semantics.

## Implementation plan

1. `flokoa/sessions/{__init__,store,settings,memory}.py` with the protocol, registry, settings, and in-memory adapter (+ TTL sweep on an asyncio task started lazily).
2. Wire `PydanticAIAgentExecutor`: constructor param, session resolution helper `_session_id(context) -> str | None`, history load/append around `agent.run`. Keep `FlokoaAgentExecutor` (base) untouched — sessions are framework-specific because serialization is.
3. Wire both entrypoints (`flokoa/__main__.py`, `flokoa_managed_agent/__main__.py`) to construct the store from settings and pass it down.
4. `flokoa run` gains `--session-backend` / `--session-ttl` Click options that set the env settings before server start (CLI parity for local dev).
5. Google-ADK executor: out of scope; add a `NotImplemented` log line if sessions are configured with that framework so the gap is visible, not silent.
6. Docs: `docs/sessions.md` — contextId semantics, TTL, multi-replica caveat (until 04), trigger `sessionKeyFrom` cross-reference.

## Testing

(Existing layout: `flokoa/tests/flokoa_cli/…` mirrors `src/`; managed-agent tests build `RequestContext` fixtures as in `flokoa-managed-agent/tests/test_agent_executor.py`.)

- Unit: `InMemorySessionStore` TTL expiry, append/load ordering, max-turns truncation, corrupt-blob recovery.
- Executor (pydantic-ai `TestModel`, as in `tests/flokoa_cli/integrations/pydantic_ai/test_pydantic_ai_agent_executor.py`): second `execute()` with same `context_id` passes prior messages in `message_history` (assert via `capture_run_messages` or TestModel's received history); different `context_id` gets none; failed run appends nothing.
- E2E (extend `sdk/python/tests/e2e/test_managed_agent.py`): two A2A messages with a shared `contextId` — "remember X" then "what is X" — using TestModel-style echo or a deterministic fake model; assert the second request's history length.

## Acceptance criteria

- `FLOKOA_SESSION_BACKEND=memory flokoa run -m examples.agent:agent` gives multi-turn memory across A2A messages sharing a `contextId`; unset → byte-identical current behavior.
- No new required dependencies in `flokoa` (in-memory backend is stdlib).
- `make check && make test` green in `sdk/python/flokoa` and `flokoa-managed-agent`.

## Out of scope

- Durable backends + CRD field (04). Long-term/semantic memory (post-roadmap). Persistent A2A `TaskStore` (separate concern; note for 13). History summarization/token-budget compaction (follow-up; `MAX_TURNS` is the v1 bound). Per-session run queueing.
