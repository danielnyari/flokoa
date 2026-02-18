"""Tests for flokoa.config.agent_builder — factory method pattern."""

import pytest

from flokoa.config.agent_builder import (
    BaseAgentBuilder,
    MarvinTaskBuilder,
    PydanticAIAgentBuilder,
    get_builder,
    register_builder,
)
from flokoa.config.agent_config import AgentConfig, LlmAgentConfig, TaskAgentConfig
from flokoa_types import IntegrationType


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


class TestMarvinTaskBuilder:
    def test_builds_kwargs(self):
        config = TaskAgentConfig(
            name="classifier",
            task_type="classify",
            labels=["a", "b"],
            input="test input",
        )
        result = MarvinTaskBuilder.from_config(config)
        assert isinstance(result, dict)
        assert result["name"] == "classifier"
        assert result["task_type"].value == "classify"
        assert result["labels"] == ["a", "b"]
        assert result["input"] == "test input"

    def test_builds_with_model(self):
        config = TaskAgentConfig(
            name="test",
            task_type="run",
            model={
                "provider": {"type": "openai"},
                "model": "gpt-4o",
            },
        )
        result = MarvinTaskBuilder.from_config(config)
        assert "model_config" in result

    def test_config_type(self):
        assert MarvinTaskBuilder.config_type is TaskAgentConfig


class TestToolResolution:
    def test_openapi_tools_passed_through(self):
        """OpenAPI tools are kept as ToolConfig for executor-level resolution."""
        from flokoa.config.tool_config import ToolConfig

        config = LlmAgentConfig(
            name="test",
            tools=[{
                "name": "api",
                "type": "openapi",
                "openApi": {"spec": {}},
            }],
        )
        result = PydanticAIAgentBuilder.from_config(config)
        # We can't easily inspect tools on the agent, but the builder
        # should not raise for OpenAPI tools

    def test_function_tool_resolution(self):
        """Function tools are resolved to callables."""
        config = LlmAgentConfig(
            name="test",
            tools=[{
                "name": "joiner",
                "type": "function",
                "code": {"name": "os.path.join"},
            }],
        )
        # This should build without error (os.path.join is callable)
        agent = PydanticAIAgentBuilder.from_config(config)
        assert agent is not None


class TestBuilderRegistry:
    def test_get_builder_pydantic_ai(self):
        cls = get_builder("llm", IntegrationType.PYDANTIC_AI)
        assert cls is PydanticAIAgentBuilder

    def test_get_builder_marvin(self):
        cls = get_builder("task", "marvin")
        assert cls is MarvinTaskBuilder

    def test_get_builder_unknown_raises(self):
        with pytest.raises(KeyError, match="No builder registered"):
            get_builder("llm", "unknown-framework")

    def test_register_custom_builder(self):
        class CustomBuilder(BaseAgentBuilder):
            config_type = LlmAgentConfig

            @classmethod
            def _build(cls, config, kwargs):
                return {"custom": True}

        register_builder("llm", "custom-framework", CustomBuilder)
        cls = get_builder("llm", "custom-framework")
        assert cls is CustomBuilder

        # Clean up
        from flokoa.config.agent_builder import _BUILDER_REGISTRY
        del _BUILDER_REGISTRY[("llm", "custom-framework")]
