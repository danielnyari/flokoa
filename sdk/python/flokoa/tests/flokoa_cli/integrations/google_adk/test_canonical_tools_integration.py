"""Integration tests for tool injection with real ADK components.

These tests verify that canonical_tools() returns our injected Flokoa tools.
"""

import pytest

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


@pytest.fixture
def flokoa_tool_definitions():
    """Create Flokoa tool definitions for injection."""
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
    """Factory that produces mock ADK FunctionTools for integration testing."""
    factory = ToolsetFactory()

    def adk_builder(td):
        from google.adk.tools import FunctionTool

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


class TestCanonicalToolsWithInjection:
    """Tests that canonical_tools() includes injected Flokoa tools."""

    async def test_canonical_tools_returns_injected_flokoa_tools(
        self, flokoa_tool_definitions, monkeypatch, mock_toolset_factory
    ):
        """Test that canonical_tools() returns injected Flokoa tools alongside existing agent tools."""
        from google.adk.agents import LlmAgent
        from google.adk.tools import FunctionTool

        from flokoa.integrations.google_adk.agent_executor import (
            GoogleADKAgentExecutor,
        )
        from flokoa.integrations.google_adk.toolset import FlokoaToolset

        # Patch load_tools to return our test definitions
        def patched_load_tools(use_cache=True, cache=None):
            return flokoa_tool_definitions

        monkeypatch.setattr("flokoa.agent_executor.load_tools", patched_load_tools)

        # Create an existing tool that the agent already has
        def existing_calculator(a: int, b: int) -> int:
            """Add two numbers together."""
            return a + b

        existing_tool = FunctionTool(func=existing_calculator)

        # Create a real ADK LlmAgent with the existing tool
        agent = LlmAgent(
            name="test_agent",
            model="gemini-2.0-flash",
            tools=[existing_tool],
        )

        # Create executor and inject Flokoa tools
        executor = GoogleADKAgentExecutor(agent=agent, toolset_factory=mock_toolset_factory)
        executor._inject_tools()

        # Verify agent.tools now contains both original tool and FlokoaToolset
        assert len(agent.tools) == 2
        assert agent.tools[0] == existing_tool
        assert isinstance(agent.tools[1], FlokoaToolset)

        # Call canonical_tools() - this is what ADK uses to resolve all tools
        canonical = await agent.canonical_tools()

        # Verify all tools are present:
        # - 1 existing tool (existing_calculator)
        # - 2 Flokoa tools (get_weather, send_email)
        assert len(canonical) == 3

        # Get tool names
        tool_names = [tool.name for tool in canonical]
        assert "existing_calculator" in tool_names
        assert "get_weather" in tool_names
        assert "send_email" in tool_names

    async def test_canonical_tools_with_agent_without_existing_tools(
        self, flokoa_tool_definitions, monkeypatch, mock_toolset_factory
    ):
        """Test canonical_tools() when agent has no existing tools."""
        from google.adk.agents import LlmAgent

        from flokoa.integrations.google_adk.agent_executor import (
            GoogleADKAgentExecutor,
        )

        # Patch load_tools to return our test definitions
        def patched_load_tools(use_cache=True, cache=None):
            return flokoa_tool_definitions

        monkeypatch.setattr("flokoa.agent_executor.load_tools", patched_load_tools)

        # Create agent with no tools
        agent = LlmAgent(
            name="test_agent",
            model="gemini-2.0-flash",
            tools=[],
        )

        # Create executor and inject Flokoa tools
        executor = GoogleADKAgentExecutor(agent=agent, toolset_factory=mock_toolset_factory)
        executor._inject_tools()

        # Call canonical_tools()
        canonical = await agent.canonical_tools()

        # Should have only the 2 Flokoa tools
        assert len(canonical) == 2
        tool_names = [tool.name for tool in canonical]
        assert "get_weather" in tool_names
        assert "send_email" in tool_names

    async def test_canonical_tools_with_multiple_toolsets(
        self, flokoa_tool_definitions, monkeypatch, mock_toolset_factory
    ):
        """Test canonical_tools() when agent has multiple toolsets."""
        from google.adk.agents import LlmAgent
        from google.adk.tools import FunctionTool
        from google.adk.tools.base_toolset import BaseToolset

        from flokoa.integrations.google_adk.agent_executor import (
            GoogleADKAgentExecutor,
        )

        # Patch load_tools to return our test definitions
        def patched_load_tools(use_cache=True, cache=None):
            return flokoa_tool_definitions

        monkeypatch.setattr("flokoa.agent_executor.load_tools", patched_load_tools)

        # Create a custom toolset that already exists on the agent
        class ExistingToolset(BaseToolset):
            async def get_tools(self, readonly_context=None):
                def multiply(a: int, b: int) -> int:
                    """Multiply two numbers."""
                    return a * b

                return [FunctionTool(func=multiply)]

        # Create agent with existing toolset
        agent = LlmAgent(
            name="test_agent",
            model="gemini-2.0-flash",
            tools=[ExistingToolset()],
        )

        # Create executor and inject Flokoa tools
        executor = GoogleADKAgentExecutor(agent=agent, toolset_factory=mock_toolset_factory)
        executor._inject_tools()

        # Call canonical_tools()
        canonical = await agent.canonical_tools()

        # Should have:
        # - 1 tool from ExistingToolset (multiply)
        # - 2 Flokoa tools (get_weather, send_email)
        assert len(canonical) == 3
        tool_names = [tool.name for tool in canonical]
        assert "multiply" in tool_names
        assert "get_weather" in tool_names
        assert "send_email" in tool_names
