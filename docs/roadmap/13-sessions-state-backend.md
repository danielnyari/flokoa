# 13 — Sessions State Backend (Postgres)

**Phase:** P1 · **Size:** L · **Depends on:** 07 (injection mechanism) · **Co-designed with:** 12 (the registry is the router's lookup), 14 (lifecycle reads/writes it)

## Goal

The first real state layer (brief: "Sessions Postgres layer — storage decision closes here"): one Postgres-backed store serving two distinct needs with two distinct schemas — the **sandbox registry** (router/pool control state) and **session persistence** (conversation state surviving sandbox reaping, delivered as the injected `flokoa.platform/session-persistence` capability).

## Storage decision (closing it)

**Postgres** over Redis: the registry needs transactional claims (one sandbox per context, exactly-once under concurrent router replicas — `INSERT … ON CONFLICT` / `SELECT … FOR UPDATE SKIP LOCKED` are the right primitives), persistence wants durable append-only history, and one operational dependency beats two. Redis adds nothing Postgres can't do at these rates (session events, not request-path reads — the router caches resolutions). Provisioning: flokoa **consumes** a connection (Helm: `state.postgres.dsnSecretRef`); an optional dev-mode single-pod Postgres ships in the chart, but production DB lifecycle is the platform team's (CloudNativePG/RDS/etc. — documented, not owned). Migrations via embedded goose/atlas run by the operator on startup, versioned with the operator.

## Schema sketch

```sql
-- registry (router/pool control plane)
sessions(context_id PK, agent, namespace, sandbox_name, state {pending|active|parked|reaped},
         identity_subject, created_at, last_activity, expires_hint)
sandboxes(name PK, agent, namespace, pool, address, state {warm|claimed|draining|gone}, claimed_by_context, heartbeat_at)

-- persistence (conversation state, append-only — the proven design from the pre-pivot sessions spec)
session_messages(id BIGSERIAL, context_id, seq, blob BYTEA /* one run's new-messages batch */, created_at)
```

Append-only message batches (one blob = one run's serialized new messages) eliminate read-modify-write races across replicas/sandboxes; load = ordered replay with a max-turns window.

## The injected capability: `flokoa.platform/session-persistence`

- Runner-baseline implementation (07 mechanism): lifecycle hooks load history before the run (contextId from the A2A task, exposed via `flokoa.context`) and append the run's new-messages batch after success — using pydantic-ai's message serialization on the pinned version. Failed runs append nothing.
- Config from operator injection: DSN **indirection** (`${secret:…}` → env per contract 03 — the DSN secret is projected into runner pods only for agents with persistence active), TTL, max-turns window.
- Active by default for `isolation: session` agents (it's the reap-survival mechanism — brief's truth-in-docs requirement); cluster-policy opt-in for shared-tier agents (multi-replica shared agents get correct cross-replica history for free via the append-only design).
- **Truth-in-docs deliverable**: the sessions docs page states plainly that *only* state persisted through this capability (or a user memory/checkpoint capability) survives reaping; the sandbox filesystem does not; snapshot/restore is v2+.

## Registry API (Go, used by router 12 and pool controller 14)

`internal/state/` package: `ResolveOrClaim(ctx, contextID, agent) (Sandbox, claimed bool, err)` (transactional), `TouchActivity(batch)`, `ParkExpired(ttl) []Sandbox`, `ReleaseSandbox(…)` — small, table-tested against a real Postgres in CI (testcontainers), with the concurrency cases (two routers claiming one new context; claim during pool exhaustion) as explicit race tests.

## Implementation plan

1. Chart values + dev-mode Postgres + migration machinery.
2. `internal/state/` registry with transactional claim semantics + race tests.
3. Persistence schema + the runner capability (hooks, serialization, windowing) + `flokoa.context` session accessors.
4. Operator wiring: DSN secret projection for persistence-active agents; injection config (07).
5. Retention: TTL sweeps for `session_messages` (per-agent TTL from spec), registry GC for reaped rows.
6. E2E: conversation continuity across **sandbox deletion** (the durability money-test: turn 1 → kill sandbox → turn 2 recalls), and across shared-tier replicas.

## Acceptance criteria

- Same contextId resolves to the same sandbox under concurrent router replicas (race-tested); conversation survives sandbox reap/reschedule via the injected capability; nothing else claims to.
- One Postgres serves registry + persistence with documented, operator-run migrations; dev-mode works on Kind out of the box.

## Out of scope

- Long-term/semantic memory (user capabilities own it). Filesystem snapshot/restore (v2+). A2A TaskStore durability (tracked separately; InMemoryTaskStore remains acceptable for v1 task records). Budget accounting tables (14's guardrail keeps per-run state only; aggregate budgets are a later RFC).
