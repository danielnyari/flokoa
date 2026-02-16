"""Tests for GoogleADKAgentExecutor."""

import sys
from unittest.mock import AsyncMock, MagicMock

import pytest

from flokoa.exceptions import CancelNotSupportedError
from flokoa.tools import ToolsetFactory
from flokoa_types import IntegrationType, ToolDefinition, ToolType
from flokoa_types.agenttool import AgentToolSpec, OpenApi, OpenApiSchema, Type

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
                "summary": "Send an email",
                "requestBody": {
                    "content": {
                        "application/json": {
                            "schema": {
                                "type": "object",
                                "properties": {
                                    "to": {"type": "string"},
                                    "body": {"type": "string"},
                                },
                                "required": ["to", "body"],
                            }
                        }
                    }
                },
                "responses": {"200": {"description": "OK"}},
            }
        }
    },
}


# Mock google.adk modules before importing the executor
@pytest.fixture(autouse=True)
def mock_adk_modules(monkeypatch):
    """Mock google.adk modules to avoid import errors."""
    # Create mock modules
    mock_google = MagicMock()
    mock_adk = MagicMock()
    mock_agents = MagicMock()
    mock_runners = MagicMock()
    mock_artifacts = MagicMock()
    mock_sessions = MagicMock()
    mock_memory = MagicMock()
    mock_genai = MagicMock()
    mock_types = MagicMock()

    # Set up module hierarchy
    mock_google.adk = mock_adk
    mock_google.genai = mock_genai
    mock_adk.agents = mock_agents
    mock_adk.runners = mock_runners
    mock_adk.artifacts = mock_artifacts
    mock_adk.sessions = mock_sessions
    mock_adk.memory = mock_memory
    mock_genai.types = mock_types

    # Mock tools module
    mock_tools = MagicMock()
    mock_adk.tools = mock_tools

    # Mock openapi_tool submodules for the builder import
    mock_openapi_tool = MagicMock()
    mock_openapi_spec_parser = MagicMock()
    mock_openapi_toolset = MagicMock()
    mock_adk.tools.openapi_tool = mock_openapi_tool
    mock_openapi_tool.openapi_spec_parser = mock_openapi_spec_parser
    mock_openapi_spec_parser.openapi_toolset = mock_openapi_toolset

    # Mock FunctionTool class
    mock_function_tool_cls = MagicMock()
    mock_tools.FunctionTool = mock_function_tool_cls

    # Mock Content and Part classes
    mock_content_cls = MagicMock()
    mock_part_cls = MagicMock()
    mock_part_cls.from_text = MagicMock(return_value=MagicMock())
    mock_types.Content = mock_content_cls
    mock_types.Part = mock_part_cls

    # Install mock modules
    sys.modules["google"] = mock_google
    sys.modules["google.adk"] = mock_adk
    sys.modules["google.adk.agents"] = mock_agents
    sys.modules["google.adk.runners"] = mock_runners
    sys.modules["google.adk.artifacts"] = mock_artifacts
    sys.modules["google.adk.sessions"] = mock_sessions
    sys.modules["google.adk.memory"] = mock_memory
    sys.modules["google.adk.tools"] = mock_tools
    sys.modules["google.adk.tools.openapi_tool"] = mock_openapi_tool
    sys.modules["google.adk.tools.openapi_tool.openapi_spec_parser"] = mock_openapi_spec_parser
    sys.modules["google.adk.tools.openapi_tool.openapi_spec_parser.openapi_toolset"] = mock_openapi_toolset
    sys.modules["google.genai"] = mock_genai
    sys.modules["google.genai.types"] = mock_types

    yield {
        "google": mock_google,
        "runners": mock_runners,
        "artifacts": mock_artifacts,
        "sessions": mock_sessions,
        "memory": mock_memory,
        "tools": mock_tools,
        "types": mock_types,
        "openapi_toolset": mock_openapi_toolset,
    }

    # Cleanup
    for mod in [
        "google",
        "google.adk",
        "google.adk.agents",
        "google.adk.runners",
        "google.adk.artifacts",
        "google.adk.sessions",
        "google.adk.memory",
        "google.adk.tools",
        "google.adk.tools.openapi_tool",
        "google.adk.tools.openapi_tool.openapi_spec_parser",
        "google.adk.tools.openapi_tool.openapi_spec_parser.openapi_toolset",
        "google.genai",
        "google.genai.types",
    ]:
        sys.modules.pop(mod, None)


@pytest.fixture
def mock_load_tools(monkeypatch):
    """Mock load_tools to return an empty list."""

    def patched_load_tools(use_cache=True, cache=None):
        return []

    monkeypatch.setattr("flokoa.agent_executor.load_tools", patched_load_tools)
    return patched_load_tools


@pytest.fixture
def mock_adk_agent():
    """Create a mock ADK BaseAgent."""
    agent = MagicMock()
    agent.name = "test_agent"
    agent.tools = []
    return agent


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
                description="Send an email",
                openApi=OpenApi(
                    openApiSchema=OpenApiSchema(value=EMAIL_API_SPEC),
                    url="https://api.email.com",
                ),
            ),
        ),
    ]


@pytest.fixture
def mock_toolset_factory():
    """Factory that produces mock ADK tools for testing."""
    factory = ToolsetFactory()

    def mock_builder(td):
        mock_tool = MagicMock()
        mock_tool.name = td.name
        return [mock_tool]

    factory.register(ToolType.OPENAPI, IntegrationType.GOOGLE_ADK, mock_builder)
    return factory


@pytest.fixture
def mock_load_tools_with_definitions(monkeypatch, multiple_tool_definitions):
    """Mock load_tools to return tool definitions."""

    def patched_load_tools(use_cache=True, cache=None):
        return multiple_tool_definitions

    monkeypatch.setattr("flokoa.agent_executor.load_tools", patched_load_tools)
    return patched_load_tools


class TestGoogleADKAgentExecutorInit:
    """Tests for GoogleADKAgentExecutor initialization."""

    def test_init_with_agent(self, mock_adk_modules, mock_load_tools, mock_adk_agent):
        """Test executor can be initialized with an ADK agent."""
        from flokoa.integrations.google_adk.agent_executor import (
            GoogleADKAgentExecutor,
        )

        executor = GoogleADKAgentExecutor(agent=mock_adk_agent)

        assert executor.agent == mock_adk_agent
        assert executor.agent.name == "test_agent"

    def test_init_with_custom_cache(
        self, mock_adk_modules, mock_load_tools, mock_adk_agent
    ):
        """Test executor can be initialized with a custom cache."""
        from flokoa.cache import ConfigCache
        from flokoa.integrations.google_adk.agent_executor import (
            GoogleADKAgentExecutor,
        )

        custom_cache = ConfigCache(ttl_seconds=120)
        executor = GoogleADKAgentExecutor(agent=mock_adk_agent, cache=custom_cache)

        assert executor.cache == custom_cache


class TestGoogleADKAgentExecutorExecute:
    """Tests for GoogleADKAgentExecutor.execute method."""

    async def test_execute_creates_runner(
        self, mock_adk_modules, mock_load_tools, mock_adk_agent
    ):
        """Test execute creates a Runner with correct parameters."""
        from flokoa.integrations.google_adk.agent_executor import (
            GoogleADKAgentExecutor,
        )

        # Setup mocks
        mock_session = MagicMock()
        mock_session.id = "test-session-id"
        mock_session.user_id = "flokoa_user"

        mock_session_service = MagicMock()
        mock_session_service.create_session = AsyncMock(return_value=mock_session)

        mock_runner_instance = MagicMock()
        mock_runner_instance.app_name = "test_agent"
        mock_runner_instance.session_service = mock_session_service

        # Mock run_async as async generator
        async def mock_run_async(**kwargs):
            event = MagicMock()
            event.content = MagicMock()
            part = MagicMock()
            part.text = "Hello from ADK!"
            event.content.parts = [part]
            yield event

        mock_runner_instance.run_async = mock_run_async

        mock_runner_cls = MagicMock(return_value=mock_runner_instance)
        mock_adk_modules["runners"].Runner = mock_runner_cls

        # Setup context and event queue
        mock_context = MagicMock()
        mock_context.get_user_input.return_value = "Hello"

        mock_event_queue = MagicMock()
        mock_event_queue.enqueue_event = AsyncMock()

        executor = GoogleADKAgentExecutor(agent=mock_adk_agent)
        await executor.execute(mock_context, mock_event_queue)

        # Verify Runner was created with correct args
        mock_runner_cls.assert_called_once()
        call_kwargs = mock_runner_cls.call_args.kwargs
        assert call_kwargs["app_name"] == "test_agent"
        assert call_kwargs["agent"] == mock_adk_agent

    async def test_execute_sends_response(
        self, mock_adk_modules, mock_load_tools, mock_adk_agent
    ):
        """Test execute sends the agent response to event queue."""
        from flokoa.integrations.google_adk.agent_executor import (
            GoogleADKAgentExecutor,
        )

        # Setup mocks
        mock_session = MagicMock()
        mock_session.id = "test-session-id"
        mock_session.user_id = "flokoa_user"

        mock_session_service = MagicMock()
        mock_session_service.create_session = AsyncMock(return_value=mock_session)

        mock_runner_instance = MagicMock()
        mock_runner_instance.app_name = "test_agent"
        mock_runner_instance.session_service = mock_session_service

        expected_response = "This is the agent's response"

        async def mock_run_async(**kwargs):
            event = MagicMock()
            event.content = MagicMock()
            part = MagicMock()
            part.text = expected_response
            event.content.parts = [part]
            yield event

        mock_runner_instance.run_async = mock_run_async
        mock_adk_modules["runners"].Runner = MagicMock(
            return_value=mock_runner_instance
        )

        mock_context = MagicMock()
        mock_context.get_user_input.return_value = "Hello"

        mock_event_queue = MagicMock()
        mock_event_queue.enqueue_event = AsyncMock()

        executor = GoogleADKAgentExecutor(agent=mock_adk_agent)
        await executor.execute(mock_context, mock_event_queue)

        # Verify response was sent
        mock_event_queue.enqueue_event.assert_called_once()

    async def test_execute_handles_empty_response(
        self, mock_adk_modules, mock_load_tools, mock_adk_agent
    ):
        """Test execute handles case where agent returns no content."""
        from flokoa.integrations.google_adk.agent_executor import (
            GoogleADKAgentExecutor,
        )

        mock_session = MagicMock()
        mock_session.id = "test-session-id"
        mock_session.user_id = "flokoa_user"

        mock_session_service = MagicMock()
        mock_session_service.create_session = AsyncMock(return_value=mock_session)

        mock_runner_instance = MagicMock()
        mock_runner_instance.app_name = "test_agent"
        mock_runner_instance.session_service = mock_session_service

        # Return event with no content
        async def mock_run_async(**kwargs):
            event = MagicMock()
            event.content = None
            yield event

        mock_runner_instance.run_async = mock_run_async
        mock_adk_modules["runners"].Runner = MagicMock(
            return_value=mock_runner_instance
        )

        mock_context = MagicMock()
        mock_context.get_user_input.return_value = "Hello"

        mock_event_queue = MagicMock()
        mock_event_queue.enqueue_event = AsyncMock()

        executor = GoogleADKAgentExecutor(agent=mock_adk_agent)
        await executor.execute(mock_context, mock_event_queue)

        # No event should be enqueued when there's no response
        mock_event_queue.enqueue_event.assert_not_called()


class TestGoogleADKAgentExecutorCancel:
    """Tests for GoogleADKAgentExecutor.cancel method."""

    async def test_cancel_raises_not_supported(
        self, mock_adk_modules, mock_load_tools, mock_adk_agent
    ):
        """Test cancel raises CancelNotSupportedError."""
        from flokoa.integrations.google_adk.agent_executor import (
            GoogleADKAgentExecutor,
        )

        executor = GoogleADKAgentExecutor(agent=mock_adk_agent)

        mock_context = MagicMock()
        mock_event_queue = MagicMock()

        with pytest.raises(CancelNotSupportedError, match="cancel not supported"):
            await executor.cancel(mock_context, mock_event_queue)


class TestGoogleADKAgentExecutorToolInjection:
    """Tests for GoogleADKAgentExecutor tool injection."""

    def test_get_toolset_returns_flokoa_toolset(
        self, mock_adk_modules, mock_load_tools_with_definitions, mock_adk_agent, mock_toolset_factory
    ):
        """Test _get_toolset returns a FlokoaToolset instance."""
        from flokoa.integrations.google_adk.agent_executor import (
            GoogleADKAgentExecutor,
        )
        from flokoa.integrations.google_adk.toolset import FlokoaToolset

        executor = GoogleADKAgentExecutor(agent=mock_adk_agent, toolset_factory=mock_toolset_factory)
        toolset = executor._get_toolset()

        assert isinstance(toolset, FlokoaToolset)

    def test_inject_tools_adds_toolset_to_agent(
        self, mock_adk_modules, mock_load_tools_with_definitions, mock_adk_agent, mock_toolset_factory
    ):
        """Test _inject_tools adds the toolset to agent.tools."""
        from flokoa.integrations.google_adk.agent_executor import (
            GoogleADKAgentExecutor,
        )
        from flokoa.integrations.google_adk.toolset import FlokoaToolset

        executor = GoogleADKAgentExecutor(agent=mock_adk_agent, toolset_factory=mock_toolset_factory)
        assert len(mock_adk_agent.tools) == 0

        executor._inject_tools()

        assert len(mock_adk_agent.tools) == 1
        assert isinstance(mock_adk_agent.tools[0], FlokoaToolset)

    def test_inject_tools_does_not_duplicate(
        self, mock_adk_modules, mock_load_tools_with_definitions, mock_adk_agent, mock_toolset_factory
    ):
        """Test _inject_tools doesn't add duplicate toolsets."""
        from flokoa.integrations.google_adk.agent_executor import (
            GoogleADKAgentExecutor,
        )

        executor = GoogleADKAgentExecutor(agent=mock_adk_agent, toolset_factory=mock_toolset_factory)

        # Inject tools twice
        executor._inject_tools()
        executor._inject_tools()

        # Should only have one toolset
        assert len(mock_adk_agent.tools) == 1

    def test_inject_tools_skips_when_no_tools(
        self, mock_adk_modules, mock_load_tools, mock_adk_agent
    ):
        """Test _inject_tools does nothing when there are no tool definitions."""
        from flokoa.integrations.google_adk.agent_executor import (
            GoogleADKAgentExecutor,
        )

        executor = GoogleADKAgentExecutor(agent=mock_adk_agent)
        executor._inject_tools()

        # Should not add any toolset
        assert len(mock_adk_agent.tools) == 0

    def test_inject_tools_handles_none_tools_list(
        self, mock_adk_modules, mock_load_tools_with_definitions, mock_adk_agent, mock_toolset_factory
    ):
        """Test _inject_tools handles agent with None tools list."""
        from flokoa.integrations.google_adk.agent_executor import (
            GoogleADKAgentExecutor,
        )
        from flokoa.integrations.google_adk.toolset import FlokoaToolset

        mock_adk_agent.tools = None
        executor = GoogleADKAgentExecutor(agent=mock_adk_agent, toolset_factory=mock_toolset_factory)
        executor._inject_tools()

        assert mock_adk_agent.tools is not None
        assert len(mock_adk_agent.tools) == 1
        assert isinstance(mock_adk_agent.tools[0], FlokoaToolset)


class TestFlokoaToolset:
    """Tests for FlokoaToolset class."""

    async def test_get_tools_returns_pre_built_tools(self, mock_adk_modules):
        """Test get_tools returns the pre-built tools passed to constructor."""
        from flokoa.integrations.google_adk.toolset import FlokoaToolset

        mock_tool_1 = MagicMock()
        mock_tool_1.name = "tool1"
        mock_tool_2 = MagicMock()
        mock_tool_2.name = "tool2"

        toolset = FlokoaToolset(tools=[mock_tool_1, mock_tool_2])
        tools = await toolset.get_tools()

        assert len(tools) == 2
        assert tools[0] is mock_tool_1
        assert tools[1] is mock_tool_2

    async def test_get_tools_returns_same_list(self, mock_adk_modules):
        """Test get_tools returns the same list on subsequent calls."""
        from flokoa.integrations.google_adk.toolset import FlokoaToolset

        toolset = FlokoaToolset(tools=[MagicMock(), MagicMock()])

        tools1 = await toolset.get_tools()
        tools2 = await toolset.get_tools()

        assert tools1 is tools2

    async def test_close_is_noop(self, mock_adk_modules):
        """Test close method completes without error."""
        from flokoa.integrations.google_adk.toolset import FlokoaToolset

        toolset = FlokoaToolset(tools=[])

        # Should not raise
        await toolset.close()
