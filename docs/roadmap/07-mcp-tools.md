# 07 — MCP Tool Support

**Phase:** 1 · **Size:** L · **Depends on:** — · **Enables:** 11 (gateway); the "MCP connectivity" harness claim

## Goal

Agents declaratively consume MCP servers: `AgentTool` gains `type: mcp`, and the managed runtime attaches them as pydantic-ai toolsets. Cheap leverage: pydantic-ai ships a native MCP client (`MCPServerStreamableHTTP`, registered via `Agent(..., toolsets=[...])`), so the work is CRD plumbing, type-pipeline, and a toolset-construction refactor — not protocol code.

## Current state

- **Tool type is closed**: `AgentToolType` enum has only `openapi` (`operator/api/v1alpha1/agenttool_types.go:25`, `+kubebuilder:validation:Enum=openapi`); `AgentToolSpec` = `Type` + `Description` + `OpenApi *OpenApiToolSpec`. Reusable shapes: `ServiceRef` (name/namespace/port), `Headers map[string]string`, `TimeoutSeconds`.
- **Tool delivery contract**: ToolReconciler (in `internal/app/agent/`) projects each tool's spec into `/etc/flokoa/tools/<name>/spec.json` (`builder.ToolsMountPath`, `ToolConfigMapKey`); the runtime's `FlokoaAgentExecutor._reload_tools()` loads them into `FlokoaToolDefinition`s via `ConfigCache`. MCP tools ride the same channel — no new transport needed.
- **Python dispatch is openapi-hard-coded in two places**:
  1. `flokoa_types.ToolDefinition.type` (hand-maintained wrapper in `flokoa-types/src/flokoa_types/__init__.py`) — `@computed_field` that **raises** `ValueError("Unsupported tool type")` for anything ≠ openapi. This will break the moment an MCP spec lands on disk; it must change in the same PR as the CRD.
  2. `PydanticAIAgentExecutor._build_toolset()` — builds a single `FunctionToolset` via `self._toolset_factory.build(self.tool_definitions, IntegrationType.PYDANTIC_AI)` and passes `toolsets=[toolset]` to `agent.run`. MCP servers are *toolsets themselves*, not `Tool` objects, so the factory abstraction needs widening, not just a new branch.
- **No MCP anywhere in the SDK** (verified: zero `pydantic_ai.mcp` / `MCPServer` references; `flokoa-codemode-mcp` uses FastMCP to *serve*, unrelated to consuming).
- OpenAPI precedent for trust/safety: `OpenAPIToolset.from_tool_definition` sets `allow_internal=True` for operator-provided specs and applies CRD headers; `validate_url` SSRF checks guard non-serviceRef URLs.

## Target design

### CRD

```go
// api/v1alpha1/agenttool_types.go

// +kubebuilder:validation:Enum=openapi;mcp
type AgentToolType string
const (
    AgentToolTypeOpenAPI AgentToolType = "openapi"
    AgentToolTypeMCP     AgentToolType = "mcp"
)

// +kubebuilder:validation:Enum=streamableHTTP;sse
type MCPTransport string

// MCPToolSpec configures a tool backed by a remote MCP server.
type MCPToolSpec struct {
    // URL of the MCP endpoint. Mutually exclusive with ServiceRef.
    // +optional
    URL string `json:"url,omitempty"`
    // ServiceRef references an in-cluster MCP server. Mutually exclusive with URL.
    // +optional
    ServiceRef *ServiceRef `json:"serviceRef,omitempty"`
    // Path appended to the ServiceRef base (default "/mcp").
    // +optional
    Path string `json:"path,omitempty"`
    // +kubebuilder:default=streamableHTTP
    Transport MCPTransport `json:"transport,omitempty"`
    // Headers added to every MCP request (non-secret).
    // +optional
    Headers map[string]string `json:"headers,omitempty"`
    // HeaderSecrets injects secret-valued headers, e.g. Authorization.
    // +optional
    HeaderSecrets []SecretHeader `json:"headerSecrets,omitempty"`
    // ToolPrefix disambiguates tool names across servers (pydantic-ai tool_prefix).
    // +optional
    ToolPrefix string `json:"toolPrefix,omitempty"`
    // AllowedTools whitelists tool names exposed to the agent; empty = all.
    // +optional
    AllowedTools []string `json:"allowedTools,omitempty"`
    // +kubebuilder:default=30
    // +optional
    TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

type SecretHeader struct {
    Name      string                   `json:"name"`      // header name
    SecretRef corev1.SecretKeySelector `json:"secretRef"` // header value
}
```

`AgentToolSpec` gains `MCP *MCPToolSpec` (+optional). `stdio` transport is intentionally absent — local-process MCP servers contradict the shared-runtime model until 13's isolation work.

**Secret headers**: the projected `spec.json` ConfigMap must not embed secret values (tenet 4). ToolReconciler renders the MCP spec with `headerSecrets` as **env-var indirections**: builder adds `FLOKOA_TOOL_SECRET_<hash>` env (`valueFrom.secretKeyRef`) to the Deployment and writes `{"header": "Authorization", "env": "FLOKOA_TOOL_SECRET_<hash>"}` into the spec file; the runtime resolves at client-construction time. Secret rotation already triggers reconcile via the existing `findAgentsForSecret` mapper — extend its index.

### Python: toolset building becomes type-dispatched

1. `flokoa-types`: extend generated models (`make generate-python-models` regenerates `agenttool.py` with the `mcp` block) and fix the hand-maintained `ToolDefinition.type` computed property + `ToolType` enum to include `MCP`.
2. New `flokoa/src/flokoa/tools/mcp/__init__.py`:
   ```python
   def create_mcp_toolset(tool_definition: ToolDefinition) -> AbstractToolset:
       """MCPServerStreamableHTTP/SSE from a ToolDefinition.
       - URL from url|serviceRef(+path); validate_url(allow_internal=bool(service_ref))
       - headers + env-resolved secret headers via a configured httpx.AsyncClient
       - tool_prefix, timeout; allowedTools via pydantic-ai's FilteredToolset wrapper
       """
   ```
   Optional extra `mcp = ["pydantic-ai[mcp]>=1.44.0"]` (verify exact extra/import: `from pydantic_ai.mcp import MCPServerStreamableHTTP, MCPServerSSE`); `_try_load`-style guarded import with an actionable `ImportError` naming `flokoa[mcp]`.
3. Executor refactor — replace single-toolset assumption:
   - `PydanticAIAgentExecutor._build_toolsets() -> list[AbstractToolset]`: partition `self.tool_definitions` by `type`; openapi → existing factory → one `FunctionToolset`; mcp → one server toolset each. `execute()` passes `toolsets=self._get_toolsets()`. Cache/rebuild semantics identical to today's `_cached_toolset` (rebuild when `tool_definitions` identity changes — `ConfigCache` already drives this).
   - **Connection lifecycle**: pydantic-ai enters MCP toolset contexts per-run when passed via `toolsets=` (verify on the pinned version; if per-run setup cost matters, keep server instances long-lived in the executor and let the library manage the connection pool — `MCPServerStreamableHTTP` holds an httpx pool).
4. `TemplatedPydanticAIAgentExecutor` inherits all of it (it already routes through `_get_toolset`).

### Webhook + card

- `agenttool_webhook.go` (`validateAgentTool`): `mcp` block required iff `type: mcp`; exactly one of url/serviceRef; https required for external URLs.
- google-adk executor: explicitly logs-and-skips MCP tool definitions (visible gap, not silent), mirroring 03's stance.

## Implementation plan

1. CRD types → `make manifests generate` → `make generate-python-models`; update `ToolDefinition`/`ToolType` in `flokoa-types` same PR; sample CRs (`docs/examples/`: external MCP + in-cluster serviceRef MCP).
2. Webhook validation + tests.
3. Operator: ToolReconciler MCP projection + secret-header env wiring + secret watch index; fakes-based unit tests asserting the rendered `spec.json` and Deployment env.
4. Python: `tools/mcp` module, extra, executor `_build_toolsets` refactor (pure refactor first — openapi-only, then add mcp dispatch), entrypoint untouched.
5. Docs page `docs/agenttool.md`: MCP section with both samples + auth-header recipe.

## Testing

- Python unit: `create_mcp_toolset` URL/serviceRef/headers/secret-env resolution; executor partitioning (mixed openapi+mcp definitions → correct toolsets list); existing openapi tests must pass unchanged through the refactor.
- Integration: in-process MCP server via FastMCP (already a workspace dep through `flokoa-codemode-mcp`; otherwise add as dev dep) — agent with TestModel calls an MCP tool end-to-end.
- Go: webhook cases; ToolReconciler projection tests; envtest case for an Agent referencing an mcp AgentTool.
- E2E (Kind): deploy a trivial MCP server pod; Agent with `toolRef` to it; A2A invocation triggers an MCP tool call (assert via tool result in artifact).

## Acceptance criteria

- `kubectl apply` of an mcp AgentTool + Agent reference yields a runtime that lists and calls MCP tools; openapi tools fully unaffected.
- Secret header values appear in no ConfigMap, CRD, or log.
- `flokoa run` locally with an mcp tool definition file works (parity for local dev).

## Out of scope

- MCP *resources*/*prompts* (tools only). stdio transport. OAuth flows for MCP (static headers now; dynamic credentials → 11). Gateway-mediated MCP (11). `flokoa-codemode-mcp` productization (14).
