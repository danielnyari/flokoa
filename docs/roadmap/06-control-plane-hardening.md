# 06 — Control-Plane Hardening: TLS + Authorization

**Phase:** 1 · **Size:** M · **Depends on:** — (01 for cert-manager plumbing) · **Enables:** multi-user/team deployments

## Goal

Two fixes to the gRPC/REST server: encrypt it (TLS) and authorize it (today *any authenticated user can perform any operation* — RELEASE_REVIEW P1 #3). Authorization delegates to Kubernetes RBAC via SubjectAccessReview, so cluster admins manage one permission system, not two.

## Current state

- **Plaintext**: the server (entrypoint under `operator/cmd/`, services in `internal/server/`) listens gRPC + grpc-gateway REST (`/api/v1alpha1/...`, SSE watch endpoints, playground) with no TLS support at all.
- **AuthN exists, authZ doesn't**: `internal/server/auth.go` validates OIDC bearer tokens (`AuthInterceptorConfig{IssuerURL, Audience, PublicMethods}`; `UnaryInterceptor`/`StreamInterceptor`; claims = `Subject, Email, Name, Groups`; `ClaimsFromContext(ctx)`), gated by `AUTH_ENABLED`. After authentication, every method is allowed.
- **RBAC raw material exists**: 27 generated roles in `operator/config/rbac/` (admin/editor/viewer per CRD) — designed for kubectl users, unused by the server.
- Services and their K8s verbs are mechanically mappable: `AgentService` (Get/List/Watch → get/list/watch `agents`), `AgentToolService`, `ModelService`, `ModelProviderService` (CRUD → create/update/delete/get/list), `AgentWorkflowService` (`SubmitRun` → create `workflows.argoproj.io` or a dedicated subresource verb), `AgentTriggerService`, plus the REST-only playground (maps naturally to a custom verb or `get agents` + invoke).

## Target design

### TLS

- Server reads `TLS_CERT_FILE`/`TLS_KEY_FILE` (paths); when set, gRPC uses `credentials.NewServerTLSFromFile` and the HTTP/SSE listener serves TLS with the same pair. Unset → current plaintext (dev). grpc-gateway's internal dial to the gRPC port: keep it loopback and move it to a localhost listener with plaintext **only if** the gateway runs in-process on localhost — verify the wiring in the server entrypoint; if gateway dials a non-loopback address, it must use TLS creds.
- Helm (`server.tls.enabled`): cert-manager `Certificate` for the server Service DNS names, secret mounted, env set. Reuses 01's issuer.
- Healthcheck/probes updated for scheme.

### Authorization: SAR interceptor

New `internal/server/authz.go`:

```go
// MethodPermission maps a gRPC full method to a Kubernetes RBAC check.
type MethodPermission struct {
    Group, Resource, Verb string
    // NamespaceFrom extracts the namespace from the request message
    // (all flokoa request protos carry namespace fields); empty = cluster-scoped list.
}

// AuthzInterceptor performs a SubjectAccessReview as the authenticated user.
func (a *AuthzInterceptor) check(ctx context.Context, method string, req proto.Message) error
```

- Static table `methodPermissions: map[string]MethodPermission` covering every registered service method; **interceptor fails closed** — an unmapped method is denied and logged. This makes "forgot to add authz" a loud bug, not a hole.
- The check builds `authorizationv1.SubjectAccessReview` with `User: claims.Email` (fallback `Subject`), `Groups: claims.Groups`, and the mapped `ResourceAttributes{Group: "agent.flokoa.ai", Resource, Verb, Namespace}`, submitted with the server's own ServiceAccount client. Cache positive results ~10s keyed by (user, groups-hash, attrs) — `k8s.io/apiserver`'s `webhook.NewDefaultAuthenticator`-style TTL cache or a small LRU; SAR latency must not ride every watch event (check once per stream at accept time).
- Wire after the existing authn interceptor (same chain in the server entrypoint), gated by `AUTHZ_ENABLED` (default: follows `AUTH_ENABLED`). REST path: grpc-gateway forwards to gRPC, so the interceptor covers REST automatically; the SSE watch handlers and playground in `internal/server/{watch,playground}.go` bypass gRPC — give them a shared `authorize(ctx, perm)` helper call (same table).
- RBAC for users: ship a `flokoa-api-user` ClusterRole example (get/list/watch all six resources) and document binding OIDC groups to the existing generated editor/viewer roles — the whole point is that `kubectl`-world RBAC now governs the API too.
- Server's own SA needs `create` on `subjectaccessreviews.authorization.k8s.io` — add to chart RBAC.

### Why SAR (decision record)

Alternatives considered: static role→method config (drifts from cluster RBAC, second policy system), OPA/casbin (new dependency + policy language for a 6-service API). SAR keeps Kubernetes as the single policy source, works with any OIDC groups via group bindings, and is the same trick used by kube-apiserver extension servers. Cost: apiserver round-trip — mitigated by the TTL cache.

## Implementation plan

1. Inspect the server entrypoint (`operator/cmd/`, likely `cmd/server/main.go`) — document current listener/gateway wiring in the PR. Add TLS config + env plumbing.
2. `internal/server/authz.go` + method table + cache + interceptor chain wiring; `authorize()` helper into SSE watch + playground handlers.
3. Tests-first for the table: a unit test asserting **every** method in the registered proto service descriptors has a `methodPermissions` entry (reflection over `grpc.ServiceRegistrar` registrations) — this is the guard rail that keeps future services safe.
4. Helm: `server.tls.*` values, Certificate template, SAR RBAC, `AUTHZ_ENABLED`.
5. Docs (`docs/security.md`): TLS setup, group-binding examples for Dex, the permission table.

## Testing

- Unit (Go): SAR interceptor against a fake `SubjectAccessReview` client — allow, deny, cache-hit, unmapped-method-denied; claims extraction edge cases (no groups, no email).
- Completeness test from step 3 (fails CI when someone adds an RPC without a permission mapping — e.g. the missing `InstructionService`, P1 #1, will be forced through it when added).
- E2E: Dex user in `viewers` group can `List` but gets `PermissionDenied` on `CreateModel`; TLS handshake verified via the gateway with the cert-manager CA.

## Acceptance criteria

- With `AUTH_ENABLED=true` + `AUTHZ_ENABLED=true`, API operations succeed/fail according to RoleBindings on the caller's groups; with both false, dev behavior unchanged.
- All listeners TLS when configured; no plaintext fallback when certs are set.
- CI guard: unmapped RPC = failing test.

## Out of scope

- Per-object (name-level) authz granularity beyond what RBAC resourceNames offers. Audit logging (note: claims are in ctx; a follow-up log interceptor is trivial). mTLS client certs. AuthN changes (existing OIDC stays).
