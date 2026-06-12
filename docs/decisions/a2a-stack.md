# Decision: A2A serving stack (a2a-sdk vs FastA2A)

**Status:** Open — keep a2a-sdk for now. Tracked by roadmap unit 05; re-evaluate
before the session router (unit 12).

## Context

The generic runner serves agents over A2A through `flokoa.serving.build_app`
(`sdk/python/flokoa/src/flokoa/serving.py`): a2a-sdk's `A2AFastAPIApplication`
+ `DefaultRequestHandler` + `InMemoryTaskStore` on FastAPI, with a
flokoa-owned executor in between. pydantic-ai ships a native alternative —
`agent.to_a2a()`, backed by FastA2A — that could replace this stack outright.

## Decision

Keep the a2a-sdk + FastAPI stack. The executor is our seam: the playground
bridge, the trigger push gateway (`push_gateway.go`), and the Argo A2A plugin
speak the a2a-sdk task/event surface today, and the executor is where
injected-capability wiring (unit 07) and the router handshake (unit 12) attach.

Migrate to FastA2A/`to_a2a()` **iff** it can replace the executor seam without
losing task-event and push-notification compatibility. Unit 07 has shipped, so
the evaluation gate is open; the decision must close before the session router
(unit 12) hard-wires its handshake against one stack.

## Open question

FastA2A now persists conversation history per `context_id` with pluggable
storage. That overlaps the planned `flokoa.platform/session-persistence`
capability (unit 13) — exactly one of them must own conversation persistence;
running both would create two sources of truth (see ADR-001, Consequences).

## Consequences

- Until this closes, runner serving changes land in `flokoa.serving` and the
  executor, not in framework-native serving paths.
- Non-K8s users serving BYO agents can already use upstream `agent.to_a2a()`;
  this decision concerns only the platform runner.
