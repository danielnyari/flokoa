"""Unified agent configuration.

Provides a single Pydantic model hierarchy that can describe any Flokoa agent
type — LLM agents (pydantic-ai, google-adk) — in one cohesive config object.

This replaces the scattered config approach (TemplateConfig + model.json +
instruction.txt + tools/*.json) with a unified model that can be loaded from
a single JSON/YAML file or assembled programmatically.

Note: This module lives in the ``flokoa`` SDK package (not ``flokoa-types``)
because ``flokoa-types`` is auto-generated from CRD schemas.  The unified
config is an SDK-level abstraction that composes the generated types.
"""

from __future__ import annotations

from typing import Any, Literal

from flokoa_types import IntegrationType
from flokoa_types.modelconfig import ModelConfig
from flokoa_types.templateconfig import OutputSchema
from pydantic import BaseModel, ConfigDict, Field, RootModel

from flokoa.config.code_ref import CodeRef
from flokoa.config.tool_config import ToolConfig


class BaseAgentConfig(BaseModel):
    """Fields common to all declarative agent types.

    Subclasses add type-specific fields while inheriting the shared base
    (name, instruction, model, tools, callbacks).
    """

    model_config = ConfigDict(extra="forbid", populate_by_name=True)

    name: str = Field(description="Agent name (used in agent card and logging).")
    description: str = Field(
        default="",
        description="Human-readable description of the agent.",
    )
    instruction: str | None = Field(
        default=None,
        description="System prompt / instruction for the agent.",
    )
    model: ModelConfig | None = Field(
        default=None,
        description="LLM model configuration (provider, model name, parameters).",
    )
    tools: list[ToolConfig] | None = Field(
        default=None,
        description="Tool definitions (OpenAPI, function, or class).",
    )

    # Lifecycle callbacks via code references
    before_agent_callbacks: list[CodeRef] | None = Field(
        default=None,
        alias="beforeAgentCallbacks",
        description="Callbacks invoked before the agent runs.",
    )
    after_agent_callbacks: list[CodeRef] | None = Field(
        default=None,
        alias="afterAgentCallbacks",
        description="Callbacks invoked after the agent runs.",
    )


class LlmAgentConfig(BaseAgentConfig):
    """Configuration for framework-backed LLM agents.

    Supports pydantic-ai, google-adk, and future framework integrations.
    The ``framework`` field selects which integration builds the agent.
    """

    model_config = ConfigDict(extra="forbid", populate_by_name=True)

    agent_type: Literal["llm"] = Field(
        default="llm",
        alias="agentType",
        description="Discriminator: 'llm' for LLM-based agents.",
    )
    framework: IntegrationType = Field(
        default=IntegrationType.PYDANTIC_AI,
        description="Framework integration to use for building the agent.",
    )
    output_schema: OutputSchema | None = Field(
        default=None,
        alias="outputSchema",
        description="JSON Schema constraints for structured output.",
    )
    input_schema: Any | None = Field(
        default=None,
        alias="inputSchema",
        description="Input validation schema.",
    )

    # LLM-specific callbacks
    before_model_callbacks: list[CodeRef] | None = Field(
        default=None,
        alias="beforeModelCallbacks",
        description="Callbacks invoked before model inference.",
    )
    after_model_callbacks: list[CodeRef] | None = Field(
        default=None,
        alias="afterModelCallbacks",
        description="Callbacks invoked after model inference.",
    )

    # Code reference for custom agent class (advanced)
    agent_class: CodeRef | None = Field(
        default=None,
        alias="agentClass",
        description="Custom agent class to instantiate (fully-qualified path).",
    )


class AgentConfig(RootModel[LlmAgentConfig]):
    """Top-level agent configuration.

    Wraps an :class:`LlmAgentConfig` as the root model.

    Usage::

        # From a dict (e.g., loaded from JSON/YAML)
        config = AgentConfig.model_validate({
            "name": "my_agent",
            "instruction": "You are helpful.",
            "model": {"provider": {"type": "openai"}, "model": "gpt-4o"},
        })
        # config.root is an LlmAgentConfig
    """

    pass
