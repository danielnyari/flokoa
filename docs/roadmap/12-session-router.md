# 12 — The Session Router

**Phase:** P1 · **Size:** XL — the single biggest new build (brief §2/§5) · **Depends on:** 06 (virtual endpoint), 11 (gate), 13 (sessions/sandbox registry — co-designed) · **Subsumes:** old specs 05 (endpoint auth) and 11 (gateway)

## Goal

The A2A-aware data-plane gateway: terminates requests on the published endpoint for session-tier agents, extracts `contextId` from the A2A body, claims/looks up the session's sandbox, proxies (including SSE streaming), and enforces endpoint authentication. HA- and latency-critical; shared-tier traffic must not depend on it in v1 (brief §11).

## Current state

No flokoa data path exists (verified). Assets to build on: the virtual endpoint swap point (06: flip the published Service's selector to the router for session-tier agents only); `trigger_session.go` contextId minting; `push_gateway.go` outbound delivery; the OIDC verification machinery in `internal/server/auth.go` (issuer discovery, claims) reusable as a library; W3C traceparent propagation conventions.

## Design

### Topology

- New deployment `flokoa-router` (Go module `operator/router/` or top-level `router/` — separate binary, separate scaling), ≥2 replicas, stateless: **all session state lives in the 13 store**; any replica can route any request (no sticky LB requirement — stickiness is data, not memory).
- Insertion per-agent: only `runtime.isolation: session` agents get their published Service selector flipped to the router. Shared-tier agents keep direct Service routing — graceful degradation by construction.
- Router config via informers on Agent CRs (which agents it fronts, their auth config, pool refs) — no per-request apiserver calls.

### Request path

1. **Terminate** HTTP on the published endpoint; pass through agent-card and health routes unauthenticated (discovery must work).
2. **Authenticate** (subsumed old 05): per-Agent `spec.auth` (mode none|oidc: issuerURL, audiences, requiredScopes) — JWT validation with cached JWKS (reuse/port the `auth.go` approach), 401 with `WWW-Authenticate`; validated identity attached to session records (13) for audit. Agent card advertises `securitySchemes` (operator card rendering).
3. **Extract contextId**: peek the JSON-RPC body (A2A methods carry `message.contextId` / task ids). Streaming-safe body handling: read+buffer the request (A2A requests are small; responses are the streams), re-emit upstream. New context (absent/unknown contextId) → mint one (reuse the `ExtractSessionKey` hashing conventions where trigger-originated) and return it per A2A semantics.
4. **Resolve sandbox** (13's registry, transactional): existing session → sandbox address; new session → **claim** from the pool (14) or request scale-up; queue/503-with-retry-after while a cold sandbox readies (the 11 spike's ready-latency numbers calibrate timeouts and whether first-message queuing is acceptable).
5. **Proxy**: streaming reverse proxy (httputil.ReverseProxy with flush-on-write for SSE; explicit test coverage for long-lived streams, client disconnects mid-stream, sandbox death mid-stream → A2A error event). Traceparent propagated; router span wraps the hop.
6. **Liveness signals**: per-request, update the session's `lastActivity` (13) — the input to TTL reaping (14). Batched/async writes; the hot path makes zero synchronous non-local calls beyond the resolve.

### Failure modes (design-reviewed, not discovered)

Sandbox gone (reschedule → claim new + session history rehydrates via the injected persistence capability, 13 — *filesystem state is lost and documented as such*); store unavailable (fail closed for new sessions, serve cached resolutions for active ones — bounded local cache with TTL); router deploy/restart (active SSE streams break, clients reconnect per A2A; document).

## Implementation plan

1. Module scaffold + informer-driven config + published-Service flip in the operator (06's swap, gated on isolation tier).
2. Auth middleware (port of old-05 design: PyJWT-equivalent in Go — `coreos/go-oidc` reuse) + card securitySchemes rendering.
3. ContextId extraction against the pinned a2a-sdk's JSON shapes (contract-tested with recorded request fixtures from the e2e suite).
4. Resolve/claim against 13's API + proxy with SSE hardening + activity reporting.
5. Observability: RED metrics per agent (route latency split: auth/resolve/upstream), claim-latency histogram (the 11 SLO regression check), structured access logs (no payloads).
6. E2E: session-tier agent — two contextIds land in two sandboxes; same contextId is sticky across router replicas; auth 401/200 paths; SSE stream integrity through the router.

## Acceptance criteria

- Session-tier agents get per-context sandbox routing with measured router overhead within the 11-derived budget (target: p95 added latency ≤ low single-digit ms excluding claim); shared-tier agents provably never traverse the router.
- Auth enforced per-Agent declaration; anonymous calls rejected, card stays discoverable.
- Router replicas are interchangeable (kill one mid-suite; sessions unaffected beyond in-flight streams).

## Out of scope

- Pool mechanics and reaping (14). Session persistence internals (13). mTLS/service-mesh integration (17 note). Shared-tier routing through the router (explicitly v1-excluded).
