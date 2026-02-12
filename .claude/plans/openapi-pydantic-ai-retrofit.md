# Plan: Retrofit OpenAPI `rest_api_tool.py` and `openapi_toolset.py` for Pydantic AI

## Summary

Rewrite two Google ADK placeholder files to be idiomatic Pydantic AI, using `RunContext[Deps]` for context passing, `Tool.from_schema(takes_ctx=True)` for tool creation, and `FunctionToolset` for grouping. All other files (`common.py`, `operation_parser.py`, `openapi_spec_parser.py`, `auth/`) remain untouched.

---

## Files to Modify

| # | File | Action |
|---|------|--------|
| 1 | `sdk/python/src/flokoa/tools/openapi/rest_api_tool.py` | **Complete rewrite** (create fresh in worktree) |
| 2 | `sdk/python/src/flokoa/tools/openapi/openapi_toolset.py` | **Complete rewrite** (create fresh in worktree) |
| 3 | `sdk/python/src/flokoa/tools/openapi/__init__.py` | **Update exports** |
| 4 | `sdk/python/src/flokoa/tools/openapi/auth/auth_helpers.py` | **Fix import** `from ..common.common` → `from ..common` |

Files NOT modified: `common.py`, `operation_parser.py`, `openapi_spec_parser.py`, entire `auth/` folder (except the one import fix).

---

## Step 1: Fix `auth/auth_helpers.py` broken import

**Change:**
```python
# Before (broken — there's no common/common.py subpackage)
from ..common.common import ApiParameter

# After
from ..common import ApiParameter
```

---

## Step 2: Rewrite `rest_api_tool.py`

### Design: Deps-based context via `RunContext`

The key Pydantic AI pattern: tools that need runtime context accept `ctx: RunContext[DepsType]` as their first parameter and are created with `takes_ctx=True`. The deps are passed at `agent.run()` time.

For OpenAPI tools, the "deps" carry the HTTP client and auth configuration. Each tool's callable is a closure over its static config (endpoint, operation, parser) but receives the `httpx.AsyncClient` and any dynamic headers via `RunContext[OpenAPIDeps]`.

### New types

```python
@dataclass
class OpenAPIDeps:
    """Dependencies injected at agent.run() time for OpenAPI tools.

    Users compose this into their own Deps dataclass, or use it directly
    as the agent's deps_type.
    """
    client: httpx.AsyncClient
    # Optional: dynamic headers callback receives deps, returns headers
    header_provider: Callable[[], Dict[str, str]] | None = None
```

### New `RestApiToolConfig` dataclass

Replaces the `RestApiTool(BaseTool)` class. Holds all **static** config for one REST API operation:

```python
@dataclass
class RestApiToolConfig:
    name: str
    description: str
    endpoint: OperationEndpoint
    operation: Operation
    operation_parser: OperationParser
    auth_scheme: AuthScheme | None = None
    auth_credential: AuthCredential | None = None
    ssl_verify: bool | str | ssl.SSLContext | None = None
    default_headers: Dict[str, str] = field(default_factory=dict)
    credential_exchanger: AutoAuthCredentialExchanger = field(
        default_factory=AutoAuthCredentialExchanger
    )

    @classmethod
    def from_parsed_operation(cls, parsed: ParsedOperation, ...) -> RestApiToolConfig: ...
```

### Factory: `create_rest_api_callable(config) -> Callable`

Returns an `async def rest_api_call(ctx: RunContext[OpenAPIDeps], **kwargs)` that:

1. Gets `ApiParameter` list from `config.operation_parser`
2. Fills in missing required args with defaults
3. Exchanges auth credentials via `config.credential_exchanger.exchange_credential()`
4. Converts auth → request params via `credential_to_param()`
5. Builds full HTTP request via `_prepare_request_params()` (extracted as module function)
6. Gets `httpx.AsyncClient` from `ctx.deps.client`
7. Optionally applies `ctx.deps.header_provider()` headers
8. Applies `config.ssl_verify`
9. Executes request via client
10. Returns JSON response or error dict

**The callable signature:**
```python
async def rest_api_call(ctx: RunContext[OpenAPIDeps], **kwargs: Any) -> Dict[str, Any]:
    ...
```

### Factory: `create_rest_api_tool(config) -> Tool`

```python
def create_rest_api_tool(config: RestApiToolConfig) -> Tool:
    callable_fn = create_rest_api_callable(config)
    return Tool.from_schema(
        function=callable_fn,
        name=config.name,
        description=config.description,
        json_schema=config.operation_parser.get_json_schema(),
        takes_ctx=True,   # <-- receives RunContext[OpenAPIDeps] as first param
        sequential=False,
    )
```

### Module-level `_prepare_request_params()`

Extracted from the current `RestApiTool._prepare_request_params()` method. Takes endpoint, operation, default_headers as explicit params instead of `self`:

```python
def _prepare_request_params(
    endpoint: OperationEndpoint,
    operation: Operation,
    default_headers: Dict[str, str],
    tool_name: str,
    parameters: List[ApiParameter],
    kwargs: Dict[str, Any],
    auth_additional_headers: Dict[str, str] | None = None,
) -> Dict[str, Any]:
```

- User-Agent changes from `google-adk/{version}` → `flokoa/{version}`
- Logic otherwise identical to current implementation

### Module-level `_request()` becomes using `ctx.deps.client`

Instead of creating a new `httpx.AsyncClient` per request, we use the one from deps:

```python
# Inside the callable:
client = ctx.deps.client
response = await client.request(**request_params)
```

This is more efficient (connection pooling) and follows the Pydantic AI weather example pattern exactly.

### All Google ADK imports removed

| Removed Import | Reason |
|---|---|
| `BaseTool`, `ToolContext`, `ReadonlyContext` | Google ADK base classes |
| `FunctionDeclaration`, `_to_gemini_schema` | Gemini-specific |
| `ToolAuthHandler` | Interactive OAuth flow (dropped) |
| `FeatureName`, `is_feature_enabled` | ADK feature flags |
| `....version import __version__` | ADK version |

| New Import | Source |
|---|---|
| `from pydantic_ai import Tool, RunContext` | Pydantic AI |
| `from ..utils import _to_snake_case` | Existing Flokoa util |

---

## Step 3: Rewrite `openapi_toolset.py`

### Design

Plain class (no `BaseToolset` inheritance) that:
1. Parses OpenAPI spec → `List[RestApiToolConfig]`
2. Creates `Tool` objects via `create_rest_api_tool()`
3. Provides `to_function_toolset()` → `FunctionToolset` for agent integration

### Class signature

```python
class OpenAPIToolset:
    def __init__(
        self,
        *,
        spec_dict: Dict[str, Any] | None = None,
        spec_str: str | None = None,
        spec_str_type: Literal["json", "yaml"] = "json",
        auth_scheme: AuthScheme | None = None,
        auth_credential: AuthCredential | None = None,
        tool_filter: list[str] | None = None,
        tool_name_prefix: str | None = None,
        ssl_verify: bool | str | ssl.SSLContext | None = None,
    ): ...

    def get_tools(self) -> list[Tool]: ...
    def get_tool(self, tool_name: str) -> Tool | None: ...
    def to_function_toolset(self) -> FunctionToolset: ...
```

### Key differences from Google ADK version

| Google ADK | Pydantic AI |
|---|---|
| `BaseToolset` inheritance | Plain class |
| `ToolPredicate` for dynamic filtering | Simple `list[str]` name filter |
| `ReadonlyContext` param on `get_tools()` | No context param (sync) |
| `AuthConfig` / `get_auth_config()` | Auth baked into each `RestApiToolConfig` |
| `header_provider: Callable[[ReadonlyContext], ...]` | Removed from toolset; lives in `OpenAPIDeps` |
| `async close()` | Not needed |
| Returns `List[RestApiTool]` (BaseTool) | Returns `list[Tool]` (Pydantic AI) |

### `to_function_toolset()` method

```python
def to_function_toolset(self) -> FunctionToolset:
    toolset = FunctionToolset()
    for tool in self.get_tools():
        toolset.add_tool(tool)
    if self._tool_name_prefix:
        return toolset.prefixed(self._tool_name_prefix)
    return toolset
```

### Usage pattern

```python
from dataclasses import dataclass
import httpx
from pydantic_ai import Agent
from flokoa.tools.openapi import OpenAPIToolset, OpenAPIDeps
from flokoa.auth.auth_credential import AuthCredential, AuthCredentialTypes

# Parse spec & create toolset
toolset = OpenAPIToolset(
    spec_dict=my_openapi_spec,
    auth_scheme=my_auth_scheme,
    auth_credential=my_auth_credential,
)

# Create agent with toolset
agent = Agent(
    'openai:gpt-4o',
    deps_type=OpenAPIDeps,
    toolsets=[toolset.to_function_toolset()],
)

# Run with deps (httpx client for connection pooling)
async with httpx.AsyncClient() as client:
    deps = OpenAPIDeps(client=client)
    result = await agent.run("List all users", deps=deps)
```

---

## Step 4: Update `__init__.py`

```python
from .openapi_toolset import OpenAPIToolset
from .rest_api_tool import OpenAPIDeps, RestApiToolConfig, create_rest_api_callable, create_rest_api_tool

__all__ = [
    "OpenAPIDeps",
    "OpenAPIToolset",
    "RestApiToolConfig",
    "create_rest_api_callable",
    "create_rest_api_tool",
]
```

---

## Auth Flow (Without ToolAuthHandler)

```
User provides auth_scheme + auth_credential at OpenAPIToolset construction time
    ↓
Each RestApiToolConfig stores auth_scheme + auth_credential
    ↓
At tool call time (inside the closure):
    1. credential_exchanger.exchange_credential(auth_scheme, auth_credential)
       - OAuth2 → extracts access_token → HttpAuth("bearer")
       - Service Account → fetches token via google.auth → HttpAuth("bearer")
       - API Key / HTTP → returned as-is
    2. credential_to_param(auth_scheme, exchanged_credential)
       → (ApiParameter, Dict[str, Any]) for request
    3. Merged into request params
```

No interactive flow, no "pending" state, no session caching. Credentials must be pre-resolved. Fresh exchange each call (acceptable for v0; can add TTL cache later).

---

## Implementation Order

1. **Fix `auth/auth_helpers.py` import** — one-line fix
2. **Write `rest_api_tool.py`** — `OpenAPIDeps`, `RestApiToolConfig`, `create_rest_api_callable()`, `create_rest_api_tool()`, `_prepare_request_params()`, `_to_snake_case` import
3. **Write `openapi_toolset.py`** — `OpenAPIToolset` class with `to_function_toolset()`
4. **Write `__init__.py`** — exports
