"""Tests for flokoa.config.agent_config — AgentConfig."""

import pytest
from flokoa_types import IntegrationType
from pydantic import ValidationError

from flokoa.config.agent_config import (
    AgentConfig,
    LlmAgentConfig,
)


class TestAgentConfigDiscrimination:
    def test_defaults_to_llm(self):
        """When agentType is absent, defaults to LlmAgentConfig."""
        config = AgentConfig.model_validate({
            "name": "test_agent",
            "instruction": "Be helpful.",
        })
        assert isinstance(config.root, LlmAgentConfig)
        assert config.root.agent_type == "llm"

    def test_explicit_llm(self):
        config = AgentConfig.model_validate({
            "agentType": "llm",
            "name": "test_agent",
        })
        assert isinstance(config.root, LlmAgentConfig)


class TestLlmAgentConfig:
    def test_full_config(self):
        config = AgentConfig.model_validate({
            "name": "my_agent",
            "description": "A helpful agent",
            "instruction": "You are helpful.",
            "framework": "pydantic-ai",
            "model": {
                "provider": {"type": "openai"},
                "model": "gpt-4o",
            },
            "outputSchema": {
                "name": "Answer",
                "description": "Structured answer",
                "jsonSchema": {
                    "type": "object",
                    "properties": {"answer": {"type": "string"}},
                    "required": ["answer"],
                },
            },
        })
        inner = config.root
        assert isinstance(inner, LlmAgentConfig)
        assert inner.name == "my_agent"
        assert inner.description == "A helpful agent"
        assert inner.instruction == "You are helpful."
        assert inner.framework == IntegrationType.PYDANTIC_AI
        assert inner.model is not None
        assert inner.model.model == "gpt-4o"
        assert inner.output_schema is not None
        assert inner.output_schema.name == "Answer"

    def test_default_framework(self):
        config = AgentConfig.model_validate({"name": "test"})
        assert config.root.framework == IntegrationType.PYDANTIC_AI

    def test_google_adk_framework(self):
        config = AgentConfig.model_validate({
            "name": "test",
            "framework": "google-adk",
        })
        assert config.root.framework == IntegrationType.GOOGLE_ADK

    def test_with_tools(self):
        config = AgentConfig.model_validate({
            "name": "test",
            "tools": [
                {
                    "name": "search",
                    "type": "function",
                    "code": {"name": "my_app.tools.search"},
                },
                {
                    "name": "api",
                    "type": "openapi",
                    "openApi": {"spec": {}},
                },
            ],
        })
        assert len(config.root.tools) == 2
        assert config.root.tools[0].name == "search"
        assert config.root.tools[1].name == "api"

    def test_with_callbacks(self):
        config = AgentConfig.model_validate({
            "name": "test",
            "beforeAgentCallbacks": [{"name": "my_app.hooks.before"}],
            "afterAgentCallbacks": [{"name": "my_app.hooks.after"}],
        })
        assert len(config.root.before_agent_callbacks) == 1
        assert len(config.root.after_agent_callbacks) == 1

    def test_with_agent_class(self):
        config = AgentConfig.model_validate({
            "name": "test",
            "agentClass": {"name": "my_app.agents.CustomAgent"},
        })
        assert config.root.agent_class is not None
        assert config.root.agent_class.name == "my_app.agents.CustomAgent"

    def test_extra_fields_forbidden(self):
        with pytest.raises(ValidationError):
            AgentConfig.model_validate({
                "name": "test",
                "unknownField": "bad",
            })


class TestJsonSchemaGeneration:
    def test_generates_json_schema(self):
        """AgentConfig should generate a valid JSON Schema."""
        schema = AgentConfig.model_json_schema()
        assert isinstance(schema, dict)
        assert "$defs" in schema or "anyOf" in schema or "$ref" in schema
