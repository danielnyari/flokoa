import json

import pytest

from flokoa_types import ToolDefinition, ToolType
from flokoa_types.agenttool import AgentToolSpec


MINIMAL_OPENAPI_SPEC = {
    "openapi": "3.0.0",
    "info": {"title": "Test API", "version": "1.0.0"},
    "servers": [{"url": "https://api.example.com"}],
    "paths": {
        "/test": {
            "post": {
                "operationId": "testEndpoint",
                "summary": "A test API tool",
                "requestBody": {
                    "content": {
                        "application/json": {
                            "schema": {
                                "type": "object",
                                "properties": {"input": {"type": "string"}},
                            }
                        }
                    }
                },
                "responses": {"200": {"description": "OK"}},
            }
        },
        "/another": {
            "get": {
                "operationId": "anotherEndpoint",
                "summary": "Another API tool",
                "responses": {"200": {"description": "OK"}},
            }
        },
    },
}


@pytest.fixture
def tools_config():
    """Tool configuration in the OpenAPI AgentToolSpec format."""
    return [
        {
            "name": "test_api_tool",
            "spec": {
                "type": "openapi",
                "description": "A test API tool",
                "openApi": {
                    "openApiSchema": {"value": MINIMAL_OPENAPI_SPEC},
                    "url": "https://api.example.com",
                },
            },
        },
        {
            "name": "another_api_tool",
            "metadata": {"version": "2.0"},
            "spec": {
                "type": "openapi",
                "description": "Another API tool",
                "openApi": {
                    "openApiSchema": {"value": MINIMAL_OPENAPI_SPEC},
                    "url": "https://api.example.com",
                },
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
        assert definitions[0].type == ToolType.OPENAPI
        assert definitions[1].metadata == {"version": "2.0"}

    def test_agent_returns_injected_agent(self, dummy_agent, dummy_agent_executor):
        assert dummy_agent_executor.agent is dummy_agent
        assert dummy_agent_executor.agent.tools == ["tool1", "tool2"]


class TestDummyAgentExecutorFixture:
    def test_add_tools_registers_tools_on_agent(self, dummy_agent_executor):
        initial_count = len(dummy_agent_executor.agent.tools)
        dummy_agent_executor.add_tools()
        assert len(dummy_agent_executor.agent.tools) >= initial_count
