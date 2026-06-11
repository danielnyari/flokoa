"""Tests for flokoa.config.agent_builder — factory method pattern."""

import pytest

from flokoa.config.agent_builder import (
    BaseAgentBuilder,
    PydanticAIAgentBuilder,
    get_builder,
    register_builder,
)
from flokoa.config.agent_config import LlmAgentConfig


class TestPydanticAIAgentBuilder:
    def test_builds_agent(self):
        from pydantic_ai import Agent

        config = LlmAgentConfig(
            name="test_agent",
            instruction="Be helpful.",
        )
        agent = PydanticAIAgentBuilder.from_config(config)
        assert isinstance(agent, Agent)

    def test_builds_agent_with_output_schema(self):
        from pydantic_ai import Agent

        config = LlmAgentConfig(
            name="test_agent",
            output_schema={
                "name": "Answer",
                "description": "Structured answer",
                "jsonSchema": {
                    "type": "object",
                    "properties": {"answer": {"type": "string"}},
                },
            },
        )
        agent = PydanticAIAgentBuilder.from_config(config)
        assert isinstance(agent, Agent)
        assert agent._output_type is not None

    def test_config_type(self):
        assert PydanticAIAgentBuilder.config_type is LlmAgentConfig


class TestToolResolution:
    def test_openapi_tools_passed_through(self):
        """OpenAPI tools are kept as ToolConfig for executor-level resolution."""

        config = LlmAgentConfig(
            name="test",
            tools=[
                {
                    "name": "api",
                    "type": "openapi",
                    "openApi": {"spec": {}},
                }
            ],
        )
        PydanticAIAgentBuilder.from_config(config)
        # We can't easily inspect tools on the agent, but the builder
        # should not raise for OpenAPI tools

    def test_function_tool_resolution(self):
        """Function tools are resolved to callables."""
        config = LlmAgentConfig(
            name="test",
            tools=[
                {
                    "name": "joiner",
                    "type": "function",
                    "code": {"name": "os.path.join"},
                }
            ],
        )
        # This should build without error (os.path.join is callable)
        agent = PydanticAIAgentBuilder.from_config(config)
        assert agent is not None


class TestBuilderRegistry:
    def test_get_builder_llm(self):
        cls = get_builder("llm")
        assert cls is PydanticAIAgentBuilder

    def test_get_builder_unknown_raises(self):
        with pytest.raises(KeyError, match="No builder registered"):
            get_builder("unknown-type")

    def test_register_custom_builder(self):
        class CustomBuilder(BaseAgentBuilder):
            config_type = LlmAgentConfig

            @classmethod
            def _build(cls, config, kwargs):
                return {"custom": True}

        register_builder("custom-type", CustomBuilder)
        cls = get_builder("custom-type")
        assert cls is CustomBuilder

        # Clean up
        from flokoa.config.agent_builder import _BUILDER_REGISTRY

        del _BUILDER_REGISTRY["custom-type"]
