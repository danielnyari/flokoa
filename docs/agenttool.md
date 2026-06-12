# AgentTool

An `AgentTool` is a **declarative MCP endpoint**: it expresses what a raw
pydantic-ai AgentSpec cannot — references to in-cluster Services and
Kubernetes Secrets. The Agent compiler turns each referenced AgentTool into
an `MCP` capability entry in the resolved spec, with `${secret:…}`
placeholders for header secrets (resolved in the runner; values never enter
the compiled spec).

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: AgentTool
metadata:
  name: search-knowledge-base
spec:
  type: mcp
  description: "Search the internal knowledge base"
  serviceRef:
    name: kb-service
    namespace: knowledge
    port: 8080
  path: /mcp
  transport: streamableHTTP
  headers:
    X-Env: production
  headerSecrets:
    - name: Authorization
      secretRef:
        name: kb-credentials
        key: token
  toolPrefix: kb
  allowedTools: [search, fetch_article]
  timeoutSeconds: 30
```

Agents reference it by name:

```yaml
spec:
  tools:
    - name: search-knowledge-base
```

## Spec fields

| Field | Description |
|---|---|
| `type` | `mcp` (default). The `openapi` type is retired — see [migration](#migrating-from-openapi-tools). |
| `description` | Human-readable description of the MCP server, surfaced to the model where supported. |
| `url` | Full URL of the MCP server. Mutually exclusive with `serviceRef`. |
| `serviceRef` | In-cluster Service (`name`, `namespace`, `port` or `portName`). |
| `path` | Endpoint path with `serviceRef`. Defaults to `/mcp` (`streamableHTTP`) or `/sse` (`sse`). |
| `transport` | `streamableHTTP` (default) or `sse`. The MCP client infers the transport from the URL; for SSE servers the path conventionally ends in `/sse`. |
| `headers` | Static HTTP headers sent to the MCP server. |
| `headerSecrets` | Headers sourced from Secret keys, delivered as `${secret:tool-<name>-<header>}` placeholders. |
| `toolPrefix` | Prefixes every tool name from this server (e.g. `kb` turns `search` into `kb_search`) — avoids collisions between servers. |
| `allowedTools` | Filters the server's tools to this list. |
| `timeoutSeconds` | Tool-call timeout. Compiles to the agent-level `tool_timeout` (the largest value across an agent's tools wins). |

## How it compiles

The example above becomes this capability entry in the agent's compiled
spec (wrapped in `PrefixTools` because `toolPrefix` is set):

```yaml
capabilities:
  - PrefixTools:
      prefix: kb
      capability:
        MCP:
          url: http://kb-service.knowledge.svc.cluster.local:8080/mcp
          id: search-knowledge-base
          native: false
          local: true
          headers:
            X-Env: production
            Authorization: ${secret:tool-search-knowledge-base-authorization}
          allowed_tools: [search, fetch_article]
```

The agent pod connects to the MCP server itself (`local: true`) — in-cluster
endpoints are not reachable from model providers' native MCP support.

## Migrating from OpenAPI tools

The `openapi` tool type is retired with the v2.1 pivot; the admission webhook
rejects it. To front a REST API:

- **Use the `flokoa.OpenAPI` capability** — the primary path. The
  [`flokoa-openapi`](https://github.com/danielnyari/flokoa/tree/main/sdk/python/flokoa-openapi)
  package turns an OpenAPI document into one **typed** tool per operation
  (parameter *and* return schemas), defers large specs behind ToolSearch
  discovery (a 500-operation spec costs the model one visible tool instead of
  500 schemas), optionally stacks with harness CodeMode, and authenticates
  with `${secret:NAME}` placeholders (API keys, OAuth2 refresh, Google
  service accounts — resolved in the runner, never stored in ConfigMaps).
  Usable today in SDK agents via
  `Agent.from_spec(..., custom_capability_types=[OpenAPI])`; the Capability
  CRD (P0b, in progress) is its CRD surface — AgentTool stays MCP-only.
  See the package README for configuration detail.
- **Run an MCP adapter** in front of the API — an MCP server that exposes the
  API's operations as tools (`flokoa-codemode-mcp`, or one of several
  generators that build MCP servers from OpenAPI specs) — and point the
  AgentTool at it. Choose this when credentials must stay out of the agent
  pod entirely (out-of-process isolation, e.g. multi-tenant clusters with
  user-supplied agent images).

## Status

The AgentTool controller validates the spec and surfaces a `Validated`
condition; the Agent compiler reads the spec directly (no ConfigMaps are
created). Editing an AgentTool recompiles every Agent that references it.
