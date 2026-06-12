# flokoa-openapi

An OpenAPI document as pydantic-ai agent tools: one **typed** tool per
operation, executed through a hardened REST engine. This package owns only the
spec→tool mapping and the authenticated `call_tool` path — discovery
(ToolSearch) and code-mode execution come from upstream pydantic-ai and the
harness, and compose with it rather than being bundled.

## Usage

```python
from pydantic_ai import Agent
from flokoa_openapi import OpenAPI

agent = Agent(
    "openai:gpt-5",
    capabilities=[
        OpenAPI(
            spec=petstore_spec,                  # dict or JSON/YAML string
            base_url="https://petstore.example.com",
            auth={
                "scheme": {"type": "http", "scheme": "bearer"},
                "credential": {
                    "auth_type": "http",
                    "http": {"scheme": "bearer", "credentials": {"token": "..."}},
                },
            },
        )
    ],
)
```

Or from a spec file (how the flokoa runner hydrates it from a compiled
AgentSpec):

```yaml
capabilities:
  - flokoa.OpenAPI:
      spec: {...}                      # inline OpenAPI document
      base_url: https://api.example.com
      auth:
        scheme: {type: apiKey, in: header, name: X-API-Key}
        credential: {auth_type: apiKey, api_key: "${secret:API_KEY}"}
      defer_tools: auto                # all | none | auto (threshold: defer_threshold)
      prefix: petstore
      allowed_operations: [list_pets, get_pet_by_id]
```

```python
agent = Agent.from_spec(spec_doc, custom_capability_types=[OpenAPI])
```

`${secret:NAME}` placeholders are resolved by the runner's secret stage before
hydration; secret values never live in the spec ConfigMap.

## What each tool carries

- `parameters_json_schema` — from the operation's parameters and request body.
- `return_schema` — from the operation's 2xx `application/json` response
  schema. Function tools rarely have good return schemas; OpenAPI operations
  always do, which is what makes large code-mode workflows reliable.
- `defer_loading` — per the `defer_tools` config (`auto` defers everything
  once the spec exceeds `defer_threshold` operations, default 25).

## Execution hardening

`call_tool` performs, per request: async credential exchange
(`flokoa_common.auth.exchangers` — OAuth2 refresh, Google service-account
minting with token caching), auth parameter injection that never appears in
the model-visible schema, SSRF validation of every constructed URL
(`allow_internal` only for operator-resolved in-cluster URLs), and
content-type-routed response parsing. Upstream error bodies are truncated and
logged server-side only — the model sees a sanitized envelope, which prevents
both data leakage and prompt-injection amplification from error pages.

## Stacking with ToolSearch and CodeMode

The three pieces compose independently — this capability never hard-bundles
them:

- **ToolSearch (pydantic-ai core, auto-injected).** With `defer_tools: all`
  (or `auto` over threshold), a 500-operation spec costs the model **one**
  visible tool (`search_tools`) instead of 500 schemas. Discovery surfaces
  matching operations — provider-natively (prompt-cache-preserving) on
  Anthropic/OpenAI, local keyword fallback elsewhere. Add `ToolSearch`
  explicitly only to tune strategy or result count.
- **CodeMode (pydantic-ai-harness).** Add `CodeMode()` to the agent's
  capabilities and discovered operations fold into the `run_code` sandbox as
  typed Python functions (`return_schema` gives them real return types, not
  `-> Any`), so the model can loop, filter, and `asyncio.gather` across
  endpoints without a model round-trip per call. Credentials are applied
  host-side in `call_tool`; LLM-generated code never sees them.
- **Approval-required tools stay native under CodeMode.** Tools needing
  human approval or deferred execution are excluded from the sandbox by
  CodeMode and surface as normal tool calls — if you wrap destructive
  operations with `.approval_required()`, expect them outside `run_code`.

Small specs degrade gracefully: `defer_tools: none` yields plain native tool
calling with no search surface, and CodeMode is optional per taste.

## Tests

```bash
uv run --package flokoa-openapi pytest flokoa-openapi/tests
```
