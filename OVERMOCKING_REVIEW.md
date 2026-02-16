# Test Overmocking Review

Audit of test files for overmocking patterns: cases where mocks replace so much
real logic that assertions become tautological or verify mock wiring rather than
actual behavior.

**Scope**: Operator unit tests (excluding e2e), all Python SDK tests (unit + e2e).

---

## Operator Unit Tests: No Issues Found

The operator tests are well-structured and have **no overmocking problems**.

All controller and server tests run against a real embedded Kubernetes API server
via `envtest`. The pattern is:

1. Create real CRD resources via `k8sClient.Create()`
2. Run real reconciliation via `controllerReconciler.Reconcile()`
3. Assert on real Kubernetes resources (Deployments, Services, ConfigMaps)

No mock clients, no fake reconcilers, no stubbed API calls. The remaining test
categories (webhook validators, converter tests, plugin tests, workflow compiler
tests) are pure-function tests with zero mocking.

---

## Python Tests: Findings

### FINDING 1 — `test_google_adk_agent_executor.py`: Execute Tests Are Mock-Wiring Tests

**Severity**: High
**Files**: `sdk/python/flokoa/tests/flokoa_cli/integrations/google_adk/test_google_adk_agent_executor.py`

#### Problem

The three tests in `TestGoogleADKAgentExecutorExecute` (lines 265-405) mock
the entire ADK execution pipeline and then assert only that the mocks were
called. Every component in the chain is a `MagicMock` or `AsyncMock`:

```
Runner (mock) → session_service (mock) → create_session (mock)
    → run_async (mock generator) → event.content.parts[0].text (mock)
        → event_queue.enqueue_event (mock)
```

**`test_execute_creates_runner`** (line 268): Mocks `Runner`, then asserts
`mock_runner_cls.assert_called_once()` and checks `call_kwargs`. This is
testing that the code calls the constructor — i.e., it verifies mock wiring,
not that a runner actually runs anything.

**`test_execute_sends_response`** (line 318): Mocks the full runner pipeline
with a canned text response, then asserts `mock_event_queue.enqueue_event
.assert_called_once()`. This confirms the mock was called but doesn't verify
that the response text `"This is the agent's response"` was correctly
transformed into an event or that the event structure is correct.

**`test_execute_handles_empty_response`** (line 365): Mocks `event.content =
None`, then asserts `mock_event_queue.enqueue_event.assert_not_called()`.
Same pattern: all logic is replaced by mocks.

#### Why This Matters

These tests will pass even if the `execute()` method is completely rewritten
to do something different, as long as it still calls `Runner()` and
`enqueue_event()`. They don't catch regressions in response parsing, session
creation logic, or error handling because all of that logic is mocked out.

#### Recommendation

The fundamental issue is that Google ADK imports are unavailable in the test
environment, forcing the `mock_adk_modules` fixture to mock 13 `sys.modules`
entries. There are two approaches:

**Option A: Make google-adk a test dependency.** Add `google-adk` to the test
extras in `pyproject.toml`. This would allow tests similar to the
`test_canonical_tools_integration.py` pattern, which already uses real ADK
imports and works well. Then rewrite the execute tests to:
- Use a real `LlmAgent` with a simple deterministic tool
- Use real `InMemorySessionService`, `InMemoryArtifactService`
- Assert on the actual text content of the enqueued event
- Assert that session state was created correctly

**Option B: If ADK must remain unmocked, narrow the mock boundary.** Instead
of mocking the entire pipeline, extract the response-parsing logic into a
testable function. For example:

```python
# In agent_executor.py
def _extract_response_text(events: list) -> str | None:
    """Extract final text from ADK events."""
    ...

# In test
def test_extract_response_text_with_content():
    event = SimpleNamespace(content=SimpleNamespace(parts=[SimpleNamespace(text="Hello")]))
    assert _extract_response_text([event]) == "Hello"

def test_extract_response_text_with_no_content():
    event = SimpleNamespace(content=None)
    assert _extract_response_text([event]) is None
```

This tests real parsing logic without needing the full ADK stack.

**Specific mocks to remove**: `mock_runner_cls`, `mock_runner_instance`,
`mock_session`, `mock_session_service`, `mock_run_async` — these are the
mocks that replace the logic the tests claim to verify.

---

### FINDING 2 — `test_google_adk_agent_executor.py`: FlokoaToolset Tests Are Near-Tautological

**Severity**: Medium
**Files**: `sdk/python/flokoa/tests/flokoa_cli/integrations/google_adk/test_google_adk_agent_executor.py`

#### Problem

**`test_get_tools_returns_pre_built_tools`** (line 514):

```python
mock_tool_1 = MagicMock()
mock_tool_2 = MagicMock()
toolset = FlokoaToolset(tools=[mock_tool_1, mock_tool_2])
tools = await toolset.get_tools()
assert tools[0] is mock_tool_1
assert tools[1] is mock_tool_2
```

This stores two MagicMocks in a list and asserts they come back in the same
order. It verifies that a Python list returns its elements — not that
`FlokoaToolset` integrates with ADK's tool resolution.

**`test_get_tools_returns_same_list`** (line 530): Same pattern — verifies
that `self._tools` returns the same object reference on two calls.

#### Recommendation

Replace these with integration tests that verify `FlokoaToolset` works with
ADK's `canonical_tools()` resolution (this pattern already exists in
`test_canonical_tools_integration.py` and works well). The toolset's actual
job is to provide tools to ADK's tool resolution system, not to store a list.

If keeping unit tests, test with real `FunctionTool` instances:

```python
async def test_get_tools_returns_adk_function_tools():
    from google.adk.tools import FunctionTool

    def my_tool(x: int) -> int:
        return x + 1

    real_tool = FunctionTool(func=my_tool)
    toolset = FlokoaToolset(tools=[real_tool])
    tools = await toolset.get_tools()

    assert len(tools) == 1
    assert tools[0].name == "my_tool"
```

---

### FINDING 3 — `test_openapi.py` / `test_auth.py`: HTTP Client Mocking is Appropriate but Some Assertions Are Weak

**Severity**: Low
**Files**: `sdk/python/flokoa/tests/flokoa_cli/tools/test_openapi.py`,
`sdk/python/flokoa/tests/flokoa_cli/tools/test_auth.py`

#### Pattern

```python
mock_response = httpx.Response(200, json={"id": 1, "name": "Buddy"}, ...)
mock_client.request = AsyncMock(return_value=mock_response)
result = await callable_fn(ctx, pet_id=1)
assert result == {"id": 1, "name": "Buddy"}
```

The HTTP client mock is justified — you don't want real HTTP calls in unit
tests. However, in the simple JSON response case, the test mocks a response
body and then asserts that exact body comes back. The assertion passes through
real parsing logic (content-type detection, JSON decoding), so it's not purely
tautological, but it's weak because it doesn't exercise the code's
transformation behavior.

#### What's Good

The `test_auth.py` tests are well-designed. They mock HTTP at the boundary
and then assert on real credential processing: token expiry math, refresh
token handling, retry logic, error classification. These are meaningful tests.

The content-type tests in `test_auth.py:TestContentTypeResponseParsing`
(lines 647-847) are also good — they verify different content-type handling
paths with mocked HTTP, which is the right approach.

#### Recommendation

For the end-to-end JSON round-trip tests in `test_openapi.py`
(`TestEndToEnd`, lines 889-1037), strengthen assertions by also verifying the
HTTP request that was sent. Most of these tests already do this (checking
URL path, method, headers, body), so this is already partially addressed.
The few tests that only assert `result == <mocked data>` should add request
verification to be more meaningful. For example, `test_petstore_get_pet_by_id`
already asserts `/pet/42` is in the URL — this is good.

No immediate action needed; this is a minor observation.

---

## Summary

| Finding | Severity | File | Recommendation |
|---------|----------|------|----------------|
| Execute tests verify mock wiring only | High | `test_google_adk_agent_executor.py` | Make ADK a test dep and use real ADK objects, or extract response-parsing into testable functions |
| FlokoaToolset tests are near-tautological | Medium | `test_google_adk_agent_executor.py` | Replace MagicMock tools with real FunctionTool instances |
| HTTP mocking with weak assertions | Low | `test_openapi.py` | Ensure request verification alongside response checks (mostly already done) |

### Files With No Issues

- **All operator tests** (32 files) — envtest-backed integration tests with zero mocking
- `test_pydantic_ai_agent_executor.py` — Uses real Agent, TestModel, FunctionModel
- `test_canonical_tools_integration.py` — Real ADK imports, real canonical_tools()
- `test_model_config.py` — Real file I/O, real deserialization
- `test_auth.py` — Appropriate HTTP mocking with meaningful behavioral assertions
- `test_openapi.py` — Real spec parsing, real schema resolution
- `test_cache.py` — Pure unit tests, no mocking
- `test_cached_loading.py` — Real file I/O with monkeypatched paths
- `test_load_agent_card.py` — Real file I/O
- `test_load_model_config.py` — Real file I/O
- `test_agent_card_builder.py` — Real objects
- `test_load_tools.py` — Real file I/O
- All managed-agent e2e tests — Real HTTP endpoints
