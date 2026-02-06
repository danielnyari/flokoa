"""Integration tests for tool injection with real ADK components.

These tests verify that canonical_tools() returns our injected Flokoa tools.
"""

import pytest

from flokoa.types import ToolDefinition
from flokoa.types.agenttool import AgentToolSpec, HttpApi, Method, Type

pytestmark = pytest.mark.anyio


@pytest.fixture
def flokoa_tool_definitions():
    """Create Flokoa tool definitions for injection."""
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
                httpApi=HttpApi(
                    url="https://api.weather.com/current", method=Method.get
                ),
            ),
        ),
        ToolDefinition(
            name="send_email",
            spec=AgentToolSpec(
                type=Type.http_api,
                description="Send an email",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "to": {"type": "string"},
                        "body": {"type": "string"},
                    },
                    "required": ["to", "body"],
                },
                outputSchema={"type": "object"},
                httpApi=HttpApi(url="https://api.email.com/send", method=Method.post),
            ),
        ),
    ]


class TestCanonicalToolsWithInjection:
    """Tests that canonical_tools() includes injected Flokoa tools."""

    async def test_canonical_tools_returns_injected_flokoa_tools(
        self, flokoa_tool_definitions, monkeypatch
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
        executor = GoogleADKAgentExecutor(agent=agent)
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
        self, flokoa_tool_definitions, monkeypatch
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
        executor = GoogleADKAgentExecutor(agent=agent)
        executor._inject_tools()

        # Call canonical_tools()
        canonical = await agent.canonical_tools()

        # Should have only the 2 Flokoa tools
        assert len(canonical) == 2
        tool_names = [tool.name for tool in canonical]
        assert "get_weather" in tool_names
        assert "send_email" in tool_names

    async def test_canonical_tools_with_multiple_toolsets(
        self, flokoa_tool_definitions, monkeypatch
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
        executor = GoogleADKAgentExecutor(agent=agent)
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
