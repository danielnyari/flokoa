# 05 — Inbound Auth on Agent A2A Endpoints

**Phase:** 1 · **Size:** L · **Depends on:** — (01 recommended for webhook enforcement) · **Enables:** the "secured endpoints" harness claim; 11 (gateway)

## Goal

Close the most embarrassing gap: today **anyone with network reach can invoke any agent**. Add declarative inbound authentication to the Agent CRD, enforced by the managed runtime as JWT validation, with the A2A agent card advertising the scheme and the Argo A2A plugin able to present credentials.

## Current state

- The managed runtime serves A2A unauthenticated: `A2AFastAPIApplication(agent_card, http_handler).build()` in both `flokoa_managed_agent/__main__.py` and `flokoa/__main__.py:_get_app`, plus a `health_router`. No middleware of any kind.
- The control plane has the only auth: `operator/internal/server/auth.go` — `AuthInterceptorConfig{IssuerURL, Audience, PublicMethods}`, go-oidc discovery, `Claims{Subject, Email, Name, Groups}`. Useful as the *naming/semantics* precedent; the enforcement point here is Python.
- The CRD card types already model security *metadata*: `AgentSkill.Security []map[string][]string` (`agent_types.go`) — but nothing enforces anything.
- The Argo A2A executor plugin (`operator/plugins/a2a/plugin/plugin.go`) calls agent endpoints (resolved from `status.url`/DNS) with no outbound credentials. It *validates* its own inbound Argo token from `/var/run/argo/token` — inbound-auth precedent in Go.
- SDK has JWT-adjacent code only in `flokoa-common/auth/` (scheme types: `ExtendedOAuth2`, `OpenIdConnectWithConfig`) used for *outbound tool* credentials — not endpoint enforcement.

## Target design

### CRD

```go
// api/v1alpha1/agent_types.go

// AgentAuthMode selects inbound authentication for the agent endpoint.
// +kubebuilder:validation:Enum=none;oidc
type AgentAuthMode string

const (
    AgentAuthModeNone AgentAuthMode = "none"
    AgentAuthModeOIDC AgentAuthMode = "oidc"
)

// AgentAuthSpec configures inbound authentication on the agent's A2A endpoint.
type AgentAuthSpec struct {
    // +kubebuilder:default=none
    Mode AgentAuthMode `json:"mode"`
    // OIDC configuration. Required when mode is "oidc".
    // +optional
    OIDC *AgentOIDCSpec `json:"oidc,omitempty"`
}

type AgentOIDCSpec struct {
    // IssuerURL for OIDC discovery (JWKS fetched from the issuer metadata).
    // +kubebuilder:validation:Pattern=`^https?://`
    IssuerURL string `json:"issuerURL"`
    // Audiences accepted in the token's aud claim. At least one required.
    // +kubebuilder:validation:MinItems=1
    Audiences []string `json:"audiences"`
    // JWKSURL overrides discovery (air-gapped issuers).
    // +optional
    JWKSURL string `json:"jwksURL,omitempty"`
    // RequiredScopes that must all be present in the scope claim.
    // +optional
    RequiredScopes []string `json:"requiredScopes,omitempty"`
}
```

`AgentSpec` gains `Auth *AgentAuthSpec \`json:"auth,omitempty"\``. Default-`none` keeps existing behavior; flipping the default later is a Helm-values decision, not a code change.

**Design choice — `mode: kubernetes` (TokenReview of ServiceAccount tokens) is deferred**: it drags apiserver RBAC into every agent pod and couples data-plane latency to the apiserver. OIDC covers it anyway — K8s SA projected tokens *are* OIDC JWTs; with service-account-issuer discovery enabled, pointing `issuerURL` at the cluster issuer works without TokenReview. Document this pattern instead of building a second mode.

### Runtime enforcement (pure ASGI middleware)

New `flokoa/src/flokoa/auth/middleware.py`:

- `JWTAuthMiddleware` — pure ASGI (not `BaseHTTPMiddleware`, which breaks streaming responses; A2A streams SSE). Validates `Authorization: Bearer` with **PyJWT** + `PyJWKClient` (built-in JWKS caching/rotation): `iss`, `aud` (any-of configured audiences), `exp/nbf`, alg allowlist (`RS256`, `ES256`), then scope check. 401 with `WWW-Authenticate: Bearer` JSON-RPC-shaped error body on failure.
- **Exempt paths**: the agent-card discovery route and health route must stay public (callers need the card to learn the auth scheme). Determine exact paths from the installed a2a-sdk (≥0.3.22) — the well-known card path (`/.well-known/agent-card.json` or `/.well-known/agent.json`; verify at implementation) and the `health_router` route. Exemptions are an explicit allowlist constant, mirroring `PublicMethods` semantics in the Go interceptor.
- Validated claims are stashed in `scope["state"]` (subject, scopes) for logging and future authz; log line per rejected request (no token contents).
- Config via env, constructed in both entrypoints: `FLOKOA_AUTH_MODE`, `FLOKOA_AUTH_ISSUER_URL`, `FLOKOA_AUTH_AUDIENCES` (comma-sep), `FLOKOA_AUTH_JWKS_URL`, `FLOKOA_AUTH_REQUIRED_SCOPES`. New optional extra `flokoa[auth] = ["pyjwt[crypto]>=2.8"]`; managed-agent image includes it. Fail-closed: `mode=oidc` with missing/invalid config or unreachable JWKS at startup → process exits non-zero (matches the A2A plugin's fatal-at-startup precedent).

### Operator projection & card

- App layer maps `spec.auth` → the env vars above (same pattern as 04's `memory_env.go`; one shared `runtime_env.go` is fine once both exist).
- **Agent card**: builder populates the card's `securitySchemes`/`security` so the published card declares bearer/OIDC requirements (the card JSON is operator-built into `/etc/flokoa/agent-card.json`; extend the Go card construction where `AgentCardOverride` is rendered). This is what makes auth *discoverable* A2A-natively.
- Webhook: `oidc` block required iff `mode: oidc`; HTTPS issuer enforced unless host is cluster-internal (`.svc`/`.cluster.local`).

### Caller side (minimum viable)

- Argo A2A plugin: optional static bearer from env `FLOKOA_A2A_BEARER_TOKEN_FILE` (mounted secret, re-read per request to honor rotation — same file-token pattern it already uses for `/var/run/argo/token`), attached to outbound agent calls. Per-`AgentCall` credentials and client-credentials flows belong to 11.
- Control-plane playground endpoint (`internal/server/playground.go`) forwards the **caller's own bearer** to the agent (identity passthrough — the server must not mint or share a god token).
- `flokoa invoke` (09) gets `--bearer-token-file`/`FLOKOA_TOKEN`.

## Implementation plan

1. CRD types + webhook validation → `make manifests generate` + `make generate-python-models`; sample CR with Dex issuer (the chart already ships Dex — use it as the e2e issuer).
2. `flokoa/auth/` middleware + settings + extra; wire into `_get_app()` and managed-agent `main()` (`app.add_middleware` equivalent for raw ASGI: wrap `server.build()` result).
3. Operator env projection + card `securitySchemes` population + tests with `repo/fakes`.
4. Plugin outbound bearer + playground passthrough.
5. Docs: `docs/security.md` section — issuer setup with Dex, SA-projected-token recipe for in-cluster callers, threat model (what this does/doesn't protect).

## Testing

- Python unit: middleware against tokens minted with a test RSA key + local JWKS (httpx-mock or a tiny JWKS app): valid, expired, wrong-aud, wrong-iss, missing scope, exempt paths, SSE response passthrough (assert streaming still works through the middleware — regression-critical).
- Managed-agent test: `mode=oidc` boot fails fast on bad config.
- Go: card rendering includes security schemes; env projection; webhook cases.
- E2E (Kind): Agent with `auth.mode: oidc` against in-cluster Dex; unauthenticated A2A call → 401; call with Dex-issued token → 200; workflow with plugin bearer secret → succeeds.

## Acceptance criteria

- `spec.auth.mode: oidc` agents reject anonymous A2A calls and accept valid JWTs; card advertises the scheme; agents without `auth` behave exactly as today.
- Streaming (SSE) responses unaffected by the middleware.
- E2E proves plugin→agent and playground→agent authenticated paths.

## Out of scope

- AuthZ beyond scopes (per-skill policy → 11). mTLS between pods (mesh territory; NetworkPolicies in 01/06 are the v1 network story). `kubernetes` TokenReview mode (documented alternative). Standard-runtime enforcement (user images own their middleware; document the env contract so they *can* reuse `flokoa.auth`).
