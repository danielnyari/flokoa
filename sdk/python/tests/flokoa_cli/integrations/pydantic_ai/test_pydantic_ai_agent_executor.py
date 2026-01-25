"""Tests for PydanticAIAgentExecutor tool injection."""

import json
from unittest.mock import AsyncMock, MagicMock

import pytest
from pydantic_ai import Agent, models
from pydantic_ai.messages import ModelMessage, ModelResponse, TextPart
from pydantic_ai.models.function import AgentInfo, FunctionModel
from pydantic_ai.models.test import TestModel

from flokoa.integrations.pydantic_ai.agent_executor import PydanticAIAgentExecutor
from flokoa.types import ToolDefinition, ToolType
from flokoa.types.agenttool import AgentToolSpec, HttpApi, Method, Type

# Block real model requests during testing
models.ALLOW_MODEL_REQUESTS = False

pytestmark = pytest.mark.anyio


@pytest.fixture
def mock_http_api(monkeypatch):
    """Mock the HTTP API calls to avoid real network requests.

    The production code uses dynamic lookup via flokoa_tools.call_http_api_tool,
    so we patch the attribute on the flokoa.tools module.

    Returns a MagicMock that can be used to assert calls.
    """
    mock = MagicMock()

    async def mock_call_http_api_tool(endpoint: str, method: str, params: dict):
        # Track the call (using MagicMock to avoid async coroutine warnings)
        mock(endpoint=endpoint, method=method, params=params)
        # Return mock data based on the endpoint
        if "weather" in endpoint:
            return {"temperature": 20, "condition": "sunny", "location": params.get("location", "unknown")}
        elif "email" in endpoint:
            return {"sent": True, "to": params.get("to"), "subject": params.get("subject")}
        return {"success": True, "params": params}

    # Patch on the flokoa.tools module where dynamic lookup happens
    monkeypatch.setattr("flokoa.tools.call_http_api_tool", mock_call_http_api_tool)
    return mock


@pytest.fixture
def api_tool_definition():
    """Create a sample API tool definition."""
    return ToolDefinition(
        name="get_weather",
        spec=AgentToolSpec(
            type=Type.http_api,
            description="Get the current weather for a location",
            inputSchema={
                "type": "object",
                "properties": {
                    "location": {"type": "string", "description": "The city name"},
                },
                "required": ["location"],
            },
            outputSchema={
                "type": "object",
                "properties": {
                    "temperature": {"type": "number"},
                    "condition": {"type": "string"},
                },
            },
            httpApi=HttpApi(url="https://api.weather.com/current", method=Method.GET),
        ),
    )


@pytest.fixture
def multiple_tool_definitions():
    """Create multiple tool definitions for testing."""
    return [
        ToolDefinition(
            name="get_weather",
            spec=AgentToolSpec(
                type=Type.http_api,
                description="Get the current weather for a location",
                inputSchema={
                    "type": "object",
                    "properties": {"location": {"type": "string"}},
                    "required": ["location"],
                },
                outputSchema={"type": "object"},
                httpApi=HttpApi(url="https://api.weather.com/current", method=Method.GET),
            ),
        ),
        ToolDefinition(
            name="send_email",
            spec=AgentToolSpec(
                type=Type.http_api,
                description="Send an email to a recipient",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "to": {"type": "string"},
                        "subject": {"type": "string"},
                        "body": {"type": "string"},
                    },
                    "required": ["to", "subject", "body"],
                },
                outputSchema={"type": "object"},
                httpApi=HttpApi(url="https://api.email.com/send", method=Method.POST),
            ),
        ),
    ]


@pytest.fixture
def tools_file(tmp_path, multiple_tool_definitions, monkeypatch, mock_http_api):
    """Create a tools configuration file and patch load_tools.

    Depends on mock_http_api to ensure HTTP mocking is set up first.
    """
    tools_config = [
        {
            "name": td.name,
            "spec": {
                "type": td.spec.type.value,
                "description": td.spec.description,
                "inputSchema": td.spec.inputSchema,
                "outputSchema": td.spec.outputSchema,
                "httpApi": {
                    "url": td.spec.httpApi.url,
                    "method": td.spec.httpApi.method.value,
                },
            },
        }
        for td in multiple_tool_definitions
    ]

    tools_file = tmp_path / "tools.json"
    tools_file.write_text(json.dumps(tools_config))

    def patched_load_tools():
        return multiple_tool_definitions

    monkeypatch.setattr("flokoa.agent_executor.load_tools", patched_load_tools)
    return tools_file


@pytest.fixture
def pydantic_agent():
    """Create a PydanticAI agent with a native tool for testing.

    This simulates a user's agent that already has tools defined.
    The executor should inject additional tools alongside this native tool.
    """
    agent = Agent(
        "test",  # Will be overridden in tests
        system_prompt="You are a helpful assistant.",
    )

    @agent.tool_plain
    def get_user_name(user_id: int) -> str:
        """Get the name of a user by their ID."""
        return f"User {user_id}"

    return agent


@pytest.fixture
def pydantic_agent_executor(pydantic_agent, tools_file):
    """Create a PydanticAIAgentExecutor with patched tool loading and mocked HTTP.

    HTTP mocking is handled via tools_file -> mock_http_api dependency chain.
    """
    return PydanticAIAgentExecutor(pydantic_agent)


class TestPydanticAIAgentExecutorInit:
    """Tests for PydanticAIAgentExecutor initialization."""

    def test_init_sets_agent(self, pydantic_agent, pydantic_agent_executor):
        """Verify that the executor stores the agent reference."""
        assert pydantic_agent_executor._agent is pydantic_agent

    def test_init_loads_tool_definitions(self, pydantic_agent_executor, multiple_tool_definitions):
        """Verify that tool definitions are loaded from configuration."""
        assert len(pydantic_agent_executor._tool_definitions) == len(multiple_tool_definitions)
        assert pydantic_agent_executor._tool_definitions[0].name == "get_weather"
        assert pydantic_agent_executor._tool_definitions[1].name == "send_email"


class TestPydanticAIAgentExecutorCreateTool:
    """Tests for the _create_tool method."""

    def test_create_tool_returns_pydantic_ai_tool(self, pydantic_agent_executor, api_tool_definition, monkeypatch):
        """Verify that _create_tool creates a valid PydanticAI Tool."""
        monkeypatch.setattr(pydantic_agent_executor, "_tool_definitions", [api_tool_definition])

        tool = pydantic_agent_executor._create_tool(api_tool_definition)

        assert tool.name == "get_weather"
        assert tool.description == "Get the current weather for a location"

    def test_create_tool_uses_input_schema(self, pydantic_agent_executor, api_tool_definition, monkeypatch):
        """Verify that the tool uses the input JSON schema from the definition."""
        monkeypatch.setattr(pydantic_agent_executor, "_tool_definitions", [api_tool_definition])

        tool = pydantic_agent_executor._create_tool(api_tool_definition)

        # The tool should have the schema from the definition
        assert tool.name == api_tool_definition.name
        assert tool.description == api_tool_definition.description
        # Check schema is applied
        assert "location" in str(tool.function_schema.json_schema)


class TestPydanticAIAgentExecutorGetToolset:
    """Tests for the _get_toolset method."""

    def test_get_toolset_returns_function_toolset(self, pydantic_agent_executor):
        """Verify that _get_toolset returns a FunctionToolset."""
        from pydantic_ai import FunctionToolset

        toolset = pydantic_agent_executor._get_toolset()

        assert isinstance(toolset, FunctionToolset)

    def test_get_toolset_includes_all_tools(self, pydantic_agent_executor, multiple_tool_definitions):
        """Verify that the toolset includes all loaded tools."""
        toolset = pydantic_agent_executor._get_toolset()
        # toolset.tools is a dict with tool names as keys
        tool_names = list(toolset.tools.keys())

        assert "get_weather" in tool_names
        assert "send_email" in tool_names
        assert len(tool_names) == len(multiple_tool_definitions)

    def test_get_toolset_tools_have_correct_properties(self, pydantic_agent_executor):
        """Verify that tools in the toolset have correct properties."""
        toolset = pydantic_agent_executor._get_toolset()

        weather_tool = toolset.tools["get_weather"]
        assert weather_tool.name == "get_weather"
        assert weather_tool.description == "Get the current weather for a location"

        email_tool = toolset.tools["send_email"]
        assert email_tool.name == "send_email"
        assert email_tool.description == "Send an email to a recipient"

    def test_get_toolset_creates_fresh_toolset_each_call(self, pydantic_agent_executor):
        """Verify that each call to _get_toolset creates a new toolset."""
        toolset1 = pydantic_agent_executor._get_toolset()
        toolset2 = pydantic_agent_executor._get_toolset()

        assert toolset1 is not toolset2


class TestPydanticAIAgentExecutorToolInjectionWithTestModel:
    """Tests for tool injection using TestModel."""

    async def test_all_tools_are_callable_and_called_with_correct_params(
        self, pydantic_agent, pydantic_agent_executor, mock_http_api
    ):
        """Verify that both native and injected tools can be called by TestModel.

        This is the core test case: users create agents with their own tools,
        and Flokoa injects additional tools that can all be executed.
        """
        # TestModel calls all tools by default
        test_model = TestModel()

        with pydantic_agent.override(model=test_model):
            toolset = pydantic_agent_executor._get_toolset()
            result = await pydantic_agent.run("What's the weather for user 123?", toolsets=[toolset])

            # Verify the agent completed successfully (all tools were callable)
            assert result.output is not None

            # Verify all tools were available
            assert test_model.last_model_request_parameters is not None
            function_tools = test_model.last_model_request_parameters.function_tools
            tool_names = [t.name for t in function_tools]

            # Native tool from user's agent
            assert "get_user_name" in tool_names

            # Injected tools from Flokoa
            assert "get_weather" in tool_names
            assert "send_email" in tool_names

            # Total: 1 native + 2 injected = 3 tools
            assert len(tool_names) == 3

            # Verify injected tools were called with correct parameters
            # TestModel generates test data based on schema, so we check the call structure
            calls = mock_http_api.call_args_list
            assert len(calls) == 2  # get_weather and send_email were called

            # Find the weather call and email call
            weather_call = next(c for c in calls if "weather" in c.kwargs["endpoint"])
            email_call = next(c for c in calls if "email" in c.kwargs["endpoint"])

            # Verify weather tool was called with correct endpoint and method
            assert weather_call.kwargs["endpoint"] == "https://api.weather.com/current"
            assert weather_call.kwargs["method"] == "GET"
            assert "location" in weather_call.kwargs["params"]

            # Verify email tool was called with correct endpoint and method
            assert email_call.kwargs["endpoint"] == "https://api.email.com/send"
            assert email_call.kwargs["method"] == "POST"
            assert "to" in email_call.kwargs["params"]
            assert "subject" in email_call.kwargs["params"]
            assert "body" in email_call.kwargs["params"]

    async def test_tools_have_correct_schema_in_model_request(
        self, pydantic_agent, pydantic_agent_executor, mock_http_api
    ):
        """Verify that both native and injected tools have correct JSON schema."""
        test_model = TestModel()

        with pydantic_agent.override(model=test_model):
            toolset = pydantic_agent_executor._get_toolset()
            await pydantic_agent.run("Check weather", toolsets=[toolset])

            function_tools = test_model.last_model_request_parameters.function_tools

            # Native tool schema
            user_tool = next(t for t in function_tools if t.name == "get_user_name")
            assert "user_id" in str(user_tool.parameters_json_schema)

            # Injected tool schema
            weather_tool = next(t for t in function_tools if t.name == "get_weather")
            assert "location" in str(weather_tool.parameters_json_schema)


class TestPydanticAIAgentExecutorToolInjectionWithFunctionModel:
    """Tests for tool injection using FunctionModel for controlled testing."""

    async def test_function_model_sees_native_and_injected_tools(self, pydantic_agent, pydantic_agent_executor):
        """Verify that FunctionModel can see both native and injected tools."""
        captured_info = None

        def model_function(messages: list[ModelMessage], info: AgentInfo) -> ModelResponse:
            nonlocal captured_info
            captured_info = info
            return ModelResponse(parts=[TextPart("Done")])

        with pydantic_agent.override(model=FunctionModel(model_function)):
            toolset = pydantic_agent_executor._get_toolset()
            await pydantic_agent.run("Test", toolsets=[toolset])

        assert captured_info is not None
        tool_names = [t.name for t in captured_info.function_tools]

        # Native tool
        assert "get_user_name" in tool_names

        # Injected tools
        assert "get_weather" in tool_names
        assert "send_email" in tool_names

        # Total: 1 native + 2 injected
        assert len(tool_names) == 3

    async def test_function_model_tool_has_correct_schema(self, pydantic_agent, pydantic_agent_executor):
        """Verify that injected tools have the correct JSON schema in FunctionModel."""
        captured_info = None

        def model_function(messages: list[ModelMessage], info: AgentInfo) -> ModelResponse:
            nonlocal captured_info
            captured_info = info
            return ModelResponse(parts=[TextPart("Done")])

        with pydantic_agent.override(model=FunctionModel(model_function)):
            toolset = pydantic_agent_executor._get_toolset()
            await pydantic_agent.run("Test", toolsets=[toolset])

        assert captured_info is not None
        weather_tool = next(t for t in captured_info.function_tools if t.name == "get_weather")
        assert "location" in str(weather_tool.parameters_json_schema)

        email_tool = next(t for t in captured_info.function_tools if t.name == "send_email")
        schema_str = str(email_tool.parameters_json_schema)
        assert "to" in schema_str
        assert "subject" in schema_str
        assert "body" in schema_str

    async def test_function_model_receives_tool_descriptions(self, pydantic_agent, pydantic_agent_executor):
        """Verify that FunctionModel receives correct tool descriptions."""
        captured_info = None

        def model_function(messages: list[ModelMessage], info: AgentInfo) -> ModelResponse:
            nonlocal captured_info
            captured_info = info
            return ModelResponse(parts=[TextPart("Done")])

        with pydantic_agent.override(model=FunctionModel(model_function)):
            toolset = pydantic_agent_executor._get_toolset()
            await pydantic_agent.run("Test", toolsets=[toolset])

        assert captured_info is not None
        weather_tool = next(t for t in captured_info.function_tools if t.name == "get_weather")
        assert weather_tool.description == "Get the current weather for a location"


class TestPydanticAIAgentExecutorExecuteMethod:
    """Tests for the execute method with full integration."""

    async def test_execute_calls_all_tools_with_correct_params(
        self, pydantic_agent, pydantic_agent_executor, mock_http_api
    ):
        """Verify that execute calls both native and injected tools with correct params."""
        test_model = TestModel()

        mock_context = MagicMock()
        mock_context.get_user_input.return_value = "What's the weather?"

        mock_event_queue = AsyncMock()
        mock_event_queue.enqueue_event = AsyncMock()

        with pydantic_agent.override(model=test_model):
            await pydantic_agent_executor.execute(mock_context, mock_event_queue)

            # Verify event was enqueued (execution completed)
            mock_event_queue.enqueue_event.assert_called_once()

            # Verify all tools were available and callable
            assert test_model.last_model_request_parameters is not None
            function_tools = test_model.last_model_request_parameters.function_tools
            tool_names = [t.name for t in function_tools]

            # Native tool preserved
            assert "get_user_name" in tool_names

            # Injected tools added
            assert "get_weather" in tool_names
            assert "send_email" in tool_names

            # Verify injected tools were called with correct parameters
            calls = mock_http_api.call_args_list
            assert len(calls) == 2  # Both injected tools were called

            # Verify each tool was called with correct endpoint and method
            endpoints_called = {c.kwargs["endpoint"] for c in calls}
            assert "https://api.weather.com/current" in endpoints_called
            assert "https://api.email.com/send" in endpoints_called

    async def test_execute_enqueues_agent_output(
        self, pydantic_agent, pydantic_agent_executor, mock_http_api
    ):
        """Verify that execute enqueues the agent's output as an event."""
        expected_output = "The weather is sunny!"
        test_model = TestModel(custom_output_text=expected_output)

        mock_context = MagicMock()
        mock_context.get_user_input.return_value = "Weather?"

        mock_event_queue = AsyncMock()
        mock_event_queue.enqueue_event = AsyncMock()

        with pydantic_agent.override(model=test_model):
            await pydantic_agent_executor.execute(mock_context, mock_event_queue)

        # Verify event queue received a call
        mock_event_queue.enqueue_event.assert_called_once()


class TestPydanticAIAgentExecutorToolCallableMapping:
    """Tests for tool callable mapping functionality."""

    def test_get_tool_callable_returns_api_wrapper(self, pydantic_agent_executor, api_tool_definition, monkeypatch):
        """Verify that API tools return a wrapper that calls the HTTP API handler."""
        monkeypatch.setattr(pydantic_agent_executor, "_tool_definitions", [api_tool_definition])

        callable_fn = pydantic_agent_executor._get_tool_callable(api_tool_definition)

        assert callable(callable_fn)
        # The wrapper accepts schema parameters and forwards to call_http_api_tool
        assert callable_fn.__name__ == "api_tool_wrapper"

    def test_tool_callable_is_async(self, pydantic_agent_executor, api_tool_definition, monkeypatch):
        """Verify that tool callables are async functions."""
        import inspect

        monkeypatch.setattr(pydantic_agent_executor, "_tool_definitions", [api_tool_definition])

        callable_fn = pydantic_agent_executor._get_tool_callable(api_tool_definition)

        assert inspect.iscoroutinefunction(callable_fn)

    def test_get_tool_callable_for_all_tool_types(self, pydantic_agent_executor, multiple_tool_definitions, monkeypatch):
        """Verify that all tool types return callables."""
        monkeypatch.setattr(pydantic_agent_executor, "_tool_definitions", multiple_tool_definitions)

        for tool_def in pydantic_agent_executor._tool_definitions:
            callable_fn = pydantic_agent_executor._get_tool_callable(tool_def)
            assert callable(callable_fn)


class TestPydanticAIAgentExecutorProperties:
    """Tests for executor properties."""

    def test_tool_definitions_property_returns_loaded_tools(self, pydantic_agent_executor):
        """Verify that tool_definitions property returns loaded tools."""
        definitions = pydantic_agent_executor.tool_definitions

        assert len(definitions) == 2
        assert definitions[0].name == "get_weather"
        assert definitions[0].type == ToolType.HTTP_API

    def test_agent_property_returns_injected_agent(self, pydantic_agent, pydantic_agent_executor):
        """Verify that agent property returns the injected agent."""
        assert pydantic_agent_executor.agent is pydantic_agent
