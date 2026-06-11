# 06 — Virtual Endpoint Identity

**Phase:** P0a · **Size:** S · **Depends on:** 04 · **Enables:** 12 (router inserts without breaking callers)

## Goal

Make the Agent's published endpoint a **flokoa-owned identity** from day one (brief §3, decision 1): in v1 it resolves to the agent's Service exactly as today, but the *contract* is that callers use `status.url` and flokoa may change what stands behind it — so inserting the P1 session router is a backend swap, not a caller migration.

## Current state

- `AgentStatus.URL` exists and is populated by the agent controller (de facto the Service DNS). Consumers found in-repo: the Argo A2A executor plugin resolves endpoints "via Agent CR `status.url` or DNS convention" (the DNS-convention fallback is the problem), the playground (`internal/server/playground.go`) reads `status.url`, the trigger invoke path delivers via the agent endpoint, and docs/examples reference Service DNS directly in places.
- Nothing defines the URL's format as a contract; nothing owns a stable name distinct from the workload Service.

## Target design

1. **Two Services per Agent**, both operator-owned:
   - `{agent}-runtime` — selector on the runner pods (today's Service, renamed role: internal workload endpoint, **not** part of the public contract).
   - `{agent}` — the **published endpoint**: a stable Service whose backend flokoa controls. v1: same selector as `-runtime` (zero added hops, zero new components). P1 session-tier: the operator flips this Service's selector to the session router deployment; callers see nothing.
   - `status.url = http://{agent}.{namespace}.svc.cluster.local:{port}/` — normative format, documented in the runtime contract's companion section and in the A2A card (`card.url`).
2. **Eliminate DNS-convention fallbacks**: every in-repo consumer (plugin, playground, push delivery, docs, samples, e2e) resolves via `status.url` (or the `{agent}` Service name derived from the CR — same thing). A grep-able lint (CI check) for the old `-runtime`-style direct references in docs/samples.
3. **Conformance note in docs**: external callers must treat `status.url` as opaque; port, path, and backing topology may change behind it. The agent card served at the endpoint is the discovery mechanism (auth schemes land there in 12).
4. Builder/app-layer change is small: `EnsureService` ×2 with the naming rule; card URL population already flows through the operator's card ConfigMap.

## Why not an Ingress/Gateway-API resource now

The identity must be cheap and universal in v1; HTTP routing machinery would add a dependency every cluster configures differently. The two-Service pattern gives flokoa the swap point with zero new infrastructure. External exposure (Ingress/Gateway API) remains the cluster operator's choice layered on the `{agent}` Service, documented but not owned.

## Testing

- envtest: both Services created/owned, `status.url` format, selector flip simulation (patch the published Service's selector → URL unchanged).
- e2e: invoke via `status.url` only; CI lint for direct workload-Service references.

## Acceptance criteria

- All flokoa components and docs reach agents exclusively through the published identity; swapping the published Service's backend in a test cluster breaks no caller.
- `status.url` format documented as normative.

## Out of scope

- The router itself (12). External ingress recipes (docs follow-up). Endpoint auth (12).
