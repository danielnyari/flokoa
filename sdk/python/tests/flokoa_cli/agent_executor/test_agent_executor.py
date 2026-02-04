import json

import pytest

from flokoa.types import ToolDefinition, ToolType
from flokoa.types.agenttool import AgentToolSpec


@pytest.fixture
def tools_config():
    """Tool configuration in the new AgentToolSpec format."""
    return [
        {
            "name": "test_api_tool",
            "spec": {
                "type": "http-api",
                "description": "A test API tool",
                "inputSchema": {"type": "object", "properties": {"input": {"type": "string"}}},
                "outputSchema": {"type": "object", "properties": {"output": {"type": "string"}}},
                "httpApi": {"url": "https://api.example.com/test", "method": "POST"},
            },
        },
        {
            "name": "another_api_tool",
            "metadata": {"version": "2.0"},
            "spec": {
                "type": "http-api",
                "description": "Another API tool",
                "inputSchema": {"type": "object"},
                "outputSchema": {"type": "object"},
                "httpApi": {"url": "https://api.example.com/another", "method": "GET"},
            },
        },
    ]


@pytest.fixture
def tools_file(tools_config, monkeypatch, tmp_path):
    tools_file = tmp_path / "tools.json"
    tools_file.write_text(json.dumps(tools_config))

    def patched_load_tools(use_cache=True, cache=None):
        with open(tools_file) as f:
            tools_cfg = json.load(f)
        return [
            ToolDefinition(
                name=t["name"],
                spec=AgentToolSpec(**t["spec"]),
                metadata=t.get("metadata", None),
            )
            for t in tools_cfg
        ]

    monkeypatch.setattr("flokoa.agent_executor.load_tools", patched_load_tools)
    return tools_file


class TestFlokoaAgentExecutorInit:
    def test_init_sets_agent(self, dummy_agent, dummy_agent_executor):
        assert dummy_agent_executor._agent is dummy_agent

    def test_init_loads_tool_definitions_from_file(self, dummy_agent, tools_file, tools_config, dummy_agent_executor):
        assert len(dummy_agent_executor._tool_definitions) == 2
        assert dummy_agent_executor._tool_definitions[0].name == tools_config[0]["name"]
        assert dummy_agent_executor._tool_definitions[1].name == tools_config[1]["name"]


class TestFlokoaAgentExecutorProperties:
    def test_tool_definitions_returns_loaded_tools(self, tools_file, tools_config, dummy_agent_executor):
        definitions = dummy_agent_executor.tool_definitions
        assert len(definitions) == 2
        assert definitions[0].name == "test_api_tool"
        assert definitions[0].type == ToolType.HTTP_API
        assert definitions[1].metadata == {"version": "2.0"}

    def test_agent_returns_injected_agent(self, dummy_agent, dummy_agent_executor):
        assert dummy_agent_executor.agent is dummy_agent
        assert dummy_agent_executor.agent.tools == ["tool1", "tool2"]


class TestFlokoaAgentExecutorGetToolCallable:
    def test_get_tool_callable_returns_api_handler(self, tools_file, dummy_agent_executor):
        tool_def = dummy_agent_executor.tool_definitions[0]
        result = dummy_agent_executor._get_tool_callable(tool_def)

        assert result.__name__ == "call_http_api_tool"
        assert callable(result)

    def test_get_tool_callable_for_all_loaded_tools(self, tools_file, dummy_agent_executor):
        for tool_def in dummy_agent_executor.tool_definitions:
            callable_fn = dummy_agent_executor._get_tool_callable(tool_def)
            assert callable(callable_fn)


class TestDummyAgentExecutorFixture:
    def test_add_tools_registers_tools_on_agent(self, dummy_agent_executor):
        initial_count = len(dummy_agent_executor.agent.tools)
        dummy_agent_executor.add_tools()
        assert len(dummy_agent_executor.agent.tools) >= initial_count
