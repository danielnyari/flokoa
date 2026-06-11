"""Tests for PydanticAIAgentExecutor tool injection."""

import json
from unittest.mock import AsyncMock, MagicMock

import pytest
from flokoa_types import ToolDefinition, ToolType
from flokoa_types.agenttool import AgentToolSpec, OpenApi, OpenApiSchema, Type
from pydantic_ai import Agent, models
from pydantic_ai.messages import ModelMessage, ModelResponse, TextPart
from pydantic_ai.models.function import AgentInfo, FunctionModel
from pydantic_ai.models.test import TestModel

from flokoa.integrations.pydantic_ai.agent_executor import PydanticAIAgentExecutor
from flokoa.tools import ToolsetFactory

# Block real model requests during testing
models.ALLOW_MODEL_REQUESTS = False

pytestmark = pytest.mark.anyio


WEATHER_API_SPEC = {
    "openapi": "3.0.0",
    "info": {"title": "Weather API", "version": "1.0.0"},
    "servers": [{"url": "https://api.weather.com"}],
    "paths": {
        "/current": {
            "get": {
                "operationId": "getWeather",
                "summary": "Get the current weather for a location",
                "parameters": [
                    {
                        "name": "location",
                        "in": "query",
                        "required": True,
                        "schema": {"type": "string"},
                    }
                ],
                "responses": {"200": {"description": "OK"}},
            }
        }
    },
}

EMAIL_API_SPEC = {
    "openapi": "3.0.0",
    "info": {"title": "Email API", "version": "1.0.0"},
    "servers": [{"url": "https://api.email.com"}],
    "paths": {
        "/send": {
            "post": {
                "operationId": "sendEmail",
                "summary": "Send an email to a recipient",
                "requestBody": {
                    "content": {
                        "application/json": {
                            "schema": {
                                "type": "object",
                                "properties": {
                                    "to": {"type": "string"},
                                    "subject": {"type": "string"},
                                    "body": {"type": "string"},
                                },
                                "required": ["to", "subject", "body"],
                            }
                        }
                    }
                },
                "responses": {"200": {"description": "OK"}},
            }
        }
    },
}


@pytest.fixture
def api_tool_definition():
    """Create a sample API tool definition."""
    return ToolDefinition(
        name="weather_api",
        spec=AgentToolSpec(
            type=Type.openapi,
            description="Get the current weather for a location",
            openApi=OpenApi(
                openApiSchema=OpenApiSchema(value=WEATHER_API_SPEC),
                url="https://api.weather.com",
            ),
        ),
    )


@pytest.fixture
def multiple_tool_definitions():
    """Create multiple tool definitions for testing."""
    return [
        ToolDefinition(
            name="weather_api",
            spec=AgentToolSpec(
                type=Type.openapi,
                description="Get the current weather for a location",
                openApi=OpenApi(
                    openApiSchema=OpenApiSchema(value=WEATHER_API_SPEC),
                    url="https://api.weather.com",
                ),
            ),
        ),
        ToolDefinition(
            name="email_api",
            spec=AgentToolSpec(
                type=Type.openapi,
                description="Send an email to a recipient",
                openApi=OpenApi(
                    openApiSchema=OpenApiSchema(value=EMAIL_API_SPEC),
                    url="https://api.email.com",
                ),
            ),
        ),
    ]


@pytest.fixture
def mock_toolset_factory():
    """Factory that produces simple mock PydanticAI tools for testing.

    Instead of creating real OpenAPI tools (which require httpx deps),
    this factory produces simple Tool objects that TestModel can invoke.
    """
    from pydantic_ai import Tool

    factory = ToolsetFactory()

    def mock_builder(td):
        tools = []
        if "weather" in td.name:

            def get_weather(location: str = "test") -> dict:
                """Get the current weather for a location"""
                return {"temperature": 20, "condition": "sunny", "location": location}

            tools.append(Tool(get_weather))
        elif "email" in td.name:

            def send_email(to: str = "", subject: str = "", body: str = "") -> dict:
                """Send an email to a recipient"""
                return {"sent": True, "to": to, "subject": subject}

            tools.append(Tool(send_email))
        return tools

    factory.register(ToolType.OPENAPI, mock_builder)
    return factory


@pytest.fixture
def tools_file(tmp_path, multiple_tool_definitions, monkeypatch):
    """Patch load_tools to return the multiple_tool_definitions fixture."""
    tools_config = [
        {
            "name": td.name,
            "spec": {
                "type": td.spec.type.value,
                "description": td.spec.description,
                "openApi": {
                    "openApiSchema": {"value": td.spec.open_api.open_api_schema.value},
                    "url": td.spec.open_api.url,
                },
            },
        }
        for td in multiple_tool_definitions
    ]

    tools_file = tmp_path / "tools.json"
    tools_file.write_text(json.dumps(tools_config))

    def patched_load_tools(use_cache=True, cache=None):
        return multiple_tool_definitions

    monkeypatch.setattr("flokoa.agent_executor.load_tools", patched_load_tools)
    return tools_file


@pytest.fixture
def pydantic_agent():
    """Create a PydanticAI agent with a native tool for testing.

    This simulates a user's agent that already has tools defined.
    The executor should inject additional tools alongside this native tool.
    Uses TestModel as default to avoid requiring model provider configuration.
    """
    agent = Agent(
        TestModel(),  # Use TestModel as default to avoid ProviderNotConfiguredError
        system_prompt="You are a helpful assistant.",
    )

    @agent.tool_plain
    def get_user_name(user_id: int) -> str:
        """Get the name of a user by their ID."""
        return f"User {user_id}"

    return agent


@pytest.fixture
def pydantic_agent_executor(pydantic_agent, tools_file, mock_toolset_factory):
    """Create a PydanticAIAgentExecutor with patched tool loading and mock factory."""
    return PydanticAIAgentExecutor(pydantic_agent, toolset_factory=mock_toolset_factory)


class TestPydanticAIAgentExecutorInit:
    """Tests for PydanticAIAgentExecutor initialization."""

    def test_init_sets_agent(self, pydantic_agent, pydantic_agent_executor):
        """Verify that the executor stores the agent reference."""
        assert pydantic_agent_executor._agent is pydantic_agent

    def test_init_loads_tool_definitions(self, pydantic_agent_executor, multiple_tool_definitions):
        """Verify that tool definitions are loaded from configuration."""
        assert len(pydantic_agent_executor._tool_definitions) == len(multiple_tool_definitions)
        assert pydantic_agent_executor._tool_definitions[0].name == "weather_api"
        assert pydantic_agent_executor._tool_definitions[1].name == "email_api"


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

    def test_get_toolset_returns_cached_toolset(self, pydantic_agent_executor):
        """Verify that _get_toolset returns cached toolset when tools unchanged."""
        toolset1 = pydantic_agent_executor._get_toolset()
        toolset2 = pydantic_agent_executor._get_toolset()

        # With caching, the same toolset should be returned
        assert toolset1 is toolset2

    def test_get_toolset_rebuilds_after_invalidation(self, pydantic_agent_executor):
        """Verify that _get_toolset rebuilds toolset after cache invalidation."""
        toolset1 = pydantic_agent_executor._get_toolset()

        # Invalidate caches
        pydantic_agent_executor.invalidate_caches()

        toolset2 = pydantic_agent_executor._get_toolset()

        # After invalidation, a new toolset should be built
        assert toolset1 is not toolset2


class TestPydanticAIAgentExecutorToolInjectionWithTestModel:
    """Tests for tool injection using TestModel."""

    async def test_all_tools_are_callable(
        self, pydantic_agent, pydantic_agent_executor
    ):
        """Verify that both native and injected tools can be called by TestModel.

        This is the core test case: users create agents with their own tools,
        and Flokoa injects additional tools that can all be executed.
        """
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

    async def test_tools_have_correct_schema_in_model_request(
        self, pydantic_agent, pydantic_agent_executor
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

    async def test_execute_calls_all_tools(
        self, pydantic_agent, pydantic_agent_executor
    ):
        """Verify that execute calls both native and injected tools."""
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

    async def test_execute_enqueues_agent_output(
        self, pydantic_agent, pydantic_agent_executor
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


class TestPydanticAIAgentExecutorProperties:
    """Tests for executor properties."""

    def test_tool_definitions_property_returns_loaded_tools(self, pydantic_agent_executor):
        """Verify that tool_definitions property returns loaded tools."""
        definitions = pydantic_agent_executor.tool_definitions

        assert len(definitions) == 2
        assert definitions[0].name == "weather_api"
        assert definitions[0].type == ToolType.OPENAPI

    def test_agent_property_returns_injected_agent(self, pydantic_agent, pydantic_agent_executor):
        """Verify that agent property returns the injected agent."""
        assert pydantic_agent_executor.agent is pydantic_agent
