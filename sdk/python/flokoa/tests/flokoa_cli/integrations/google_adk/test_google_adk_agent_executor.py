"""Tests for GoogleADKAgentExecutor.

Uses real Google ADK objects wherever possible, mocking only the LLM boundary
(Runner.run_async) to avoid requiring API credentials.
"""

from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from flokoa_types import IntegrationType, ToolDefinition, ToolType
from flokoa_types.agenttool import AgentToolSpec, OpenApi, OpenApiSchema, Type
from google.adk.agents import LlmAgent
from google.adk.tools import FunctionTool
from google.genai import types as genai_types

from flokoa.exceptions import CancelNotSupportedError
from flokoa.integrations.google_adk.agent_executor import (
    GoogleADKAgentExecutor,
    _extract_final_response,
)
from flokoa.integrations.google_adk.toolset import FlokoaToolset
from flokoa.tools import ToolsetFactory

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


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture
def mock_load_tools(monkeypatch):
    """Mock load_tools to return an empty list."""

    def patched_load_tools(use_cache=True, cache=None):
        return []

    monkeypatch.setattr("flokoa.agent_executor.load_tools", patched_load_tools)
    return patched_load_tools


@pytest.fixture
def adk_agent(mock_load_tools):
    """Create a real ADK LlmAgent for testing."""
    return LlmAgent(
        name="test_agent",
        model="gemini-2.0-flash",
        tools=[],
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
    """Factory that produces real ADK FunctionTools for testing."""
    factory = ToolsetFactory()

    def adk_builder(td):
        if "weather" in td.name:

            def get_weather(location: str = "test") -> dict:
                """Get the current weather for a location"""
                return {"temperature": 20}

            return [FunctionTool(func=get_weather)]
        elif "email" in td.name:

            def send_email(to: str = "", body: str = "") -> dict:
                """Send an email"""
                return {"sent": True}

            return [FunctionTool(func=send_email)]
        return []

    factory.register(ToolType.OPENAPI, IntegrationType.GOOGLE_ADK, adk_builder)
    return factory


@pytest.fixture
def mock_load_tools_with_definitions(monkeypatch, multiple_tool_definitions):
    """Mock load_tools to return tool definitions."""

    def patched_load_tools(use_cache=True, cache=None):
        return multiple_tool_definitions

    monkeypatch.setattr("flokoa.agent_executor.load_tools", patched_load_tools)
    return patched_load_tools


# ===========================================================================
# _extract_final_response — unit tests for response parsing logic
# ===========================================================================


class TestExtractFinalResponse:
    """Tests for the extracted response-parsing function using real ADK types."""

    def test_extracts_text_from_content_event(self):
        """Real Content with a text Part should return the text."""
        event = SimpleNamespace(
            content=genai_types.Content(
                role="model",
                parts=[genai_types.Part.from_text(text="Hello from the agent!")],
            )
        )
        assert _extract_final_response(event) == "Hello from the agent!"

    def test_returns_none_when_content_is_none(self):
        event = SimpleNamespace(content=None)
        assert _extract_final_response(event) is None

    def test_returns_none_when_no_content_attribute(self):
        event = SimpleNamespace()
        assert _extract_final_response(event) is None

    def test_returns_none_when_parts_is_empty(self):
        event = SimpleNamespace(
            content=genai_types.Content(role="model", parts=[])
        )
        assert _extract_final_response(event) is None

    def test_returns_last_text_part(self):
        """When multiple text parts exist, returns the last one."""
        event = SimpleNamespace(
            content=genai_types.Content(
                role="model",
                parts=[
                    genai_types.Part.from_text(text="First part"),
                    genai_types.Part.from_text(text="Second part"),
                ],
            )
        )
        assert _extract_final_response(event) == "Second part"

    def test_returns_none_when_parts_have_no_text(self):
        """Parts without text attributes should be skipped."""
        event = SimpleNamespace(
            content=SimpleNamespace(parts=[SimpleNamespace(data=b"binary")])
        )
        assert _extract_final_response(event) is None

    def test_returns_none_when_text_is_empty_string(self):
        """Empty string text should be treated as no text."""
        event = SimpleNamespace(
            content=genai_types.Content(
                role="model",
                parts=[genai_types.Part.from_text(text="")],
            )
        )
        assert _extract_final_response(event) is None


# ===========================================================================
# GoogleADKAgentExecutor — initialization
# ===========================================================================


class TestGoogleADKAgentExecutorInit:
    """Tests for GoogleADKAgentExecutor initialization."""

    def test_init_with_real_agent(self, adk_agent):
        executor = GoogleADKAgentExecutor(agent=adk_agent)
        assert executor.agent is adk_agent
        assert executor.agent.name == "test_agent"

    def test_init_with_custom_cache(self, adk_agent):
        from flokoa.cache import ConfigCache

        custom_cache = ConfigCache(ttl_seconds=120)
        executor = GoogleADKAgentExecutor(agent=adk_agent, cache=custom_cache)
        assert executor.cache == custom_cache


# ===========================================================================
# GoogleADKAgentExecutor — execute with real ADK objects
# ===========================================================================


class TestGoogleADKAgentExecutorExecute:
    """Tests for execute() using real ADK types and narrow mocking.

    Only Runner.run_async is mocked (the LLM boundary). Everything else —
    session creation, tool injection, response parsing, event enqueuing — uses
    real objects.
    """

    async def test_execute_enqueues_text_response(self, adk_agent):
        """Execute should parse a real Content event and enqueue the text."""
        response_event = SimpleNamespace(
            content=genai_types.Content(
                role="model",
                parts=[genai_types.Part.from_text(text="The weather is sunny!")],
            )
        )

        async def fake_run_async(**kwargs):
            yield response_event

        mock_context = MagicMock()
        mock_context.get_user_input.return_value = "What's the weather?"
        mock_event_queue = MagicMock()
        mock_event_queue.enqueue_event = AsyncMock()

        executor = GoogleADKAgentExecutor(agent=adk_agent)

        with patch(
            "google.adk.runners.Runner"
        ) as MockRunner:
            mock_runner = MagicMock()
            mock_runner.app_name = "test_agent"
            mock_session = MagicMock()
            mock_session.id = "sess-1"
            mock_session.user_id = "flokoa_user"
            mock_runner.session_service.create_session = AsyncMock(
                return_value=mock_session
            )
            mock_runner.run_async = fake_run_async
            MockRunner.return_value = mock_runner

            await executor.execute(mock_context, mock_event_queue)

        # The actual text from the Content event should be enqueued
        mock_event_queue.enqueue_event.assert_called_once()
        enqueued_msg = mock_event_queue.enqueue_event.call_args[0][0]
        # Verify the enqueued message contains the correct response text
        assert "The weather is sunny!" in str(enqueued_msg)

    async def test_execute_skips_empty_content_events(self, adk_agent):
        """Events with content=None should not enqueue anything."""
        empty_event = SimpleNamespace(content=None)

        async def fake_run_async(**kwargs):
            yield empty_event

        mock_context = MagicMock()
        mock_context.get_user_input.return_value = "Hello"
        mock_event_queue = MagicMock()
        mock_event_queue.enqueue_event = AsyncMock()

        executor = GoogleADKAgentExecutor(agent=adk_agent)

        with patch(
            "google.adk.runners.Runner"
        ) as MockRunner:
            mock_runner = MagicMock()
            mock_runner.app_name = "test_agent"
            mock_session = MagicMock()
            mock_session.id = "sess-1"
            mock_session.user_id = "flokoa_user"
            mock_runner.session_service.create_session = AsyncMock(
                return_value=mock_session
            )
            mock_runner.run_async = fake_run_async
            MockRunner.return_value = mock_runner

            await executor.execute(mock_context, mock_event_queue)

        mock_event_queue.enqueue_event.assert_not_called()

    async def test_execute_uses_last_text_across_events(self, adk_agent):
        """When multiple events have text, the last one wins."""
        event1 = SimpleNamespace(
            content=genai_types.Content(
                role="model",
                parts=[genai_types.Part.from_text(text="Thinking...")],
            )
        )
        event2 = SimpleNamespace(
            content=genai_types.Content(
                role="model",
                parts=[genai_types.Part.from_text(text="Final answer: 42")],
            )
        )

        async def fake_run_async(**kwargs):
            yield event1
            yield event2

        mock_context = MagicMock()
        mock_context.get_user_input.return_value = "What is the answer?"
        mock_event_queue = MagicMock()
        mock_event_queue.enqueue_event = AsyncMock()

        executor = GoogleADKAgentExecutor(agent=adk_agent)

        with patch(
            "google.adk.runners.Runner"
        ) as MockRunner:
            mock_runner = MagicMock()
            mock_runner.app_name = "test_agent"
            mock_session = MagicMock()
            mock_session.id = "sess-1"
            mock_session.user_id = "flokoa_user"
            mock_runner.session_service.create_session = AsyncMock(
                return_value=mock_session
            )
            mock_runner.run_async = fake_run_async
            MockRunner.return_value = mock_runner

            await executor.execute(mock_context, mock_event_queue)

        mock_event_queue.enqueue_event.assert_called_once()
        enqueued_msg = mock_event_queue.enqueue_event.call_args[0][0]
        assert "Final answer: 42" in str(enqueued_msg)

    async def test_execute_passes_user_input_as_message(self, adk_agent):
        """The user input from the context should be forwarded to the runner."""
        captured_kwargs = {}

        async def fake_run_async(**kwargs):
            captured_kwargs.update(kwargs)
            return
            yield  # make it an async generator

        mock_context = MagicMock()
        mock_context.get_user_input.return_value = "Tell me about Flokoa"
        mock_event_queue = MagicMock()
        mock_event_queue.enqueue_event = AsyncMock()

        executor = GoogleADKAgentExecutor(agent=adk_agent)

        with patch(
            "google.adk.runners.Runner"
        ) as MockRunner:
            mock_runner = MagicMock()
            mock_runner.app_name = "test_agent"
            mock_session = MagicMock()
            mock_session.id = "sess-1"
            mock_session.user_id = "flokoa_user"
            mock_runner.session_service.create_session = AsyncMock(
                return_value=mock_session
            )
            mock_runner.run_async = fake_run_async
            MockRunner.return_value = mock_runner

            await executor.execute(mock_context, mock_event_queue)

        # Verify the user message was passed through
        new_message = captured_kwargs["new_message"]
        assert new_message.parts[0].text == "Tell me about Flokoa"


# ===========================================================================
# GoogleADKAgentExecutor — cancel
# ===========================================================================


class TestGoogleADKAgentExecutorCancel:
    """Tests for GoogleADKAgentExecutor.cancel method."""

    async def test_cancel_raises_not_supported(self, adk_agent):
        executor = GoogleADKAgentExecutor(agent=adk_agent)

        mock_context = MagicMock()
        mock_event_queue = MagicMock()

        with pytest.raises(CancelNotSupportedError, match="cancel not supported"):
            await executor.cancel(mock_context, mock_event_queue)


# ===========================================================================
# GoogleADKAgentExecutor — tool injection
# ===========================================================================


class TestGoogleADKAgentExecutorToolInjection:
    """Tests for tool injection using real ADK agents and FunctionTools."""

    def test_get_toolset_returns_flokoa_toolset(
        self, mock_load_tools_with_definitions, mock_toolset_factory
    ):
        agent = LlmAgent(name="test_agent", model="gemini-2.0-flash", tools=[])
        executor = GoogleADKAgentExecutor(
            agent=agent, toolset_factory=mock_toolset_factory
        )
        toolset = executor._get_toolset()
        assert isinstance(toolset, FlokoaToolset)

    def test_inject_tools_adds_toolset_to_agent(
        self, mock_load_tools_with_definitions, mock_toolset_factory
    ):
        agent = LlmAgent(name="test_agent", model="gemini-2.0-flash", tools=[])
        executor = GoogleADKAgentExecutor(
            agent=agent, toolset_factory=mock_toolset_factory
        )

        executor._inject_tools()

        assert len(agent.tools) == 1
        assert isinstance(agent.tools[0], FlokoaToolset)

    def test_inject_tools_does_not_duplicate(
        self, mock_load_tools_with_definitions, mock_toolset_factory
    ):
        agent = LlmAgent(name="test_agent", model="gemini-2.0-flash", tools=[])
        executor = GoogleADKAgentExecutor(
            agent=agent, toolset_factory=mock_toolset_factory
        )

        executor._inject_tools()
        executor._inject_tools()

        assert len(agent.tools) == 1

    def test_inject_tools_skips_when_no_tools(self, mock_load_tools):
        agent = LlmAgent(name="test_agent", model="gemini-2.0-flash", tools=[])
        executor = GoogleADKAgentExecutor(agent=agent)

        executor._inject_tools()

        assert len(agent.tools) == 0

    def test_inject_tools_preserves_existing_tools(
        self, mock_load_tools_with_definitions, mock_toolset_factory
    ):
        """Existing agent tools should remain after injection."""
        existing_tool = FunctionTool(func=lambda x: x)
        agent = LlmAgent(
            name="test_agent", model="gemini-2.0-flash", tools=[existing_tool]
        )
        executor = GoogleADKAgentExecutor(
            agent=agent, toolset_factory=mock_toolset_factory
        )

        executor._inject_tools()

        assert len(agent.tools) == 2
        assert agent.tools[0] is existing_tool
        assert isinstance(agent.tools[1], FlokoaToolset)


# ===========================================================================
# FlokoaToolset — tests with real FunctionTool instances
# ===========================================================================


class TestFlokoaToolset:
    """Tests for FlokoaToolset using real ADK FunctionTool instances."""

    async def test_get_tools_returns_real_function_tools(self):
        """FlokoaToolset should return real FunctionTool instances with correct names."""

        def greet(name: str = "World") -> str:
            """Greet someone."""
            return f"Hello, {name}!"

        def add(a: int, b: int) -> int:
            """Add two numbers."""
            return a + b

        tool_1 = FunctionTool(func=greet)
        tool_2 = FunctionTool(func=add)

        toolset = FlokoaToolset(tools=[tool_1, tool_2])
        tools = await toolset.get_tools()

        assert len(tools) == 2
        assert tools[0].name == "greet"
        assert tools[1].name == "add"

    async def test_get_tools_returns_stable_reference(self):
        """Subsequent calls should return the same list object (caching)."""
        tool = FunctionTool(func=lambda: None)
        toolset = FlokoaToolset(tools=[tool])

        tools1 = await toolset.get_tools()
        tools2 = await toolset.get_tools()

        assert tools1 is tools2

    async def test_get_tools_with_empty_list(self):
        """Empty toolset should return empty list."""
        toolset = FlokoaToolset(tools=[])
        tools = await toolset.get_tools()
        assert tools == []

    async def test_close_is_noop(self):
        """close() should complete without error."""
        toolset = FlokoaToolset(tools=[])
        await toolset.close()
