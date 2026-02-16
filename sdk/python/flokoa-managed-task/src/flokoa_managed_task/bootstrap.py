"""Bootstrap logic for instantiating and executing Marvin tasks from config."""

from __future__ import annotations

import logging
from enum import Enum
from typing import Any

import marvin
from a2a.types import Artifact
from a2a.utils import new_data_artifact, new_text_artifact
from marvin import Agent as MarvinAgent
from pydantic import BaseModel
from pydantic_ai import StructuredDict
from pydantic_ai.models import Model
from pydantic_ai.models.anthropic import AnthropicModel
from pydantic_ai.models.bedrock import BedrockConverseModel
from pydantic_ai.models.google import GoogleModel
from pydantic_ai.models.openai import OpenAIResponsesModel
from pydantic_ai.providers import Provider, infer_provider_class
from pydantic_ai.settings import ModelSettings, merge_model_settings

from flokoa_types import ModelConfig, ModelParameters, TaskConfig, TaskResultType
from flokoa_types.modelconfig import ProviderType

logger = logging.getLogger(__name__)

# Provider → pydantic-ai model class mapping.
# Same mapping as flokoa SDK's PROVIDER_MODEL_MAP.
_PROVIDER_MODEL_MAP: dict[ProviderType, type[Model]] = {
    ProviderType.openai: OpenAIResponsesModel,
    ProviderType.anthropic: AnthropicModel,
    ProviderType.google: GoogleModel,
    ProviderType.bedrock: BedrockConverseModel,
}


def execute_task(
    task_config: TaskConfig,
    model_config: ModelConfig | None,
    instruction: str | None,
) -> Artifact:
    """Execute a Marvin task and return the result as an A2A Artifact.

    Args:
        task_config: The task configuration from FLOKOA_TASK_CONFIG.
        model_config: Optional model configuration from /etc/flokoa/model.json.
        instruction: Optional instruction text from /etc/flokoa/instruction.txt.

    Returns:
        An A2A Artifact wrapping the task result.
    """
    agent = _build_marvin_agent(model_config, task_config)

    # Inline instructions take precedence over mounted file
    instructions = task_config.instructions or instruction

    task_type = task_config.type.value

    match task_type:
        case "run":
            result = marvin.run(
                instructions or "",
                result_type=_build_result_type(task_config.result_type) or str,
                agent=agent,
            )
        case "classify":
            result = marvin.classify(
                task_config.input or "",
                labels=task_config.labels or [],
                multi_label=task_config.multi_label or False,
                instructions=instructions,
                agent=agent,
            )
        case "extract":
            result = marvin.extract(
                task_config.input or "",
                target=_build_result_type(task_config.result_type) or str,
                instructions=instructions,
                agent=agent,
            )
        case "cast":
            result = marvin.cast(
                task_config.input or "",
                target=_build_result_type(task_config.result_type),
                instructions=instructions,
                agent=agent,
            )
        case "generate":
            result = marvin.generate(
                target=_build_result_type(task_config.result_type),
                n=task_config.count or 1,
                instructions=instructions,
                agent=agent,
            )
        case _:
            raise ValueError(f"Unknown task type: {task_type}")

    return _build_artifact(result, task_config)


def _build_marvin_agent(
    model_config: ModelConfig | None,
    task_config: TaskConfig,
) -> MarvinAgent | None:
    """Build a Marvin Agent with pydantic-ai model settings.

    Returns None if no model_config is provided (Marvin will use its default).
    """
    if not model_config:
        return None

    model = _create_pydantic_ai_model(model_config)

    agent_kwargs: dict[str, Any] = {"model": model}

    if task_config.agent:
        agent_kwargs["name"] = task_config.agent.name
        if task_config.agent.instructions:
            agent_kwargs["instructions"] = task_config.agent.instructions

    return MarvinAgent(**agent_kwargs)


def _create_pydantic_ai_model(model_config: ModelConfig) -> Model:
    """Create a pydantic-ai Model from ModelConfig.

    Uses the same pattern as the flokoa SDK's PydanticAIAgentExecutor.
    """
    provider_type = model_config.provider.type
    provider = _create_provider(model_config)

    model_cls = _PROVIDER_MODEL_MAP.get(provider_type)
    if not model_cls:
        raise ValueError(f"No model mapping found for provider '{provider_type.value}'")

    model_settings = _build_model_settings(model_config)
    return model_cls(model_config.model, provider=provider, settings=model_settings)


def _create_provider(model_config: ModelConfig) -> Provider:
    """Create a pydantic-ai Provider from ModelConfig."""
    provider_type = model_config.provider.type
    provider_config = getattr(model_config.provider, provider_type.value, None)

    provider_cls = infer_provider_class(provider=provider_type.value)
    if provider_config:
        return provider_cls(**provider_config.model_dump())
    return provider_cls()


def _build_model_settings(model_config: ModelConfig) -> ModelSettings | None:
    """Build pydantic-ai ModelSettings from ModelConfig parameters."""
    if not model_config.parameters:
        return None

    params = model_config.parameters
    common = _params_to_settings(params)
    provider_specific = _provider_params_to_settings(model_config)

    return merge_model_settings(common, provider_specific)


def _params_to_settings(params: ModelParameters) -> ModelSettings:
    """Convert common ModelParameters to ModelSettings."""
    _PARAM_MAPPINGS: list[tuple[str, str, type | None]] = [
        ("max_tokens", "max_tokens", None),
        ("temperature", "temperature", float),
        ("top_p", "top_p", float),
        ("seed", "seed", None),
        ("presence_penalty", "presence_penalty", float),
        ("frequency_penalty", "frequency_penalty", float),
        ("logit_bias", "logit_bias", None),
        ("stop_sequences", "stop_sequences", None),
        ("extra_headers", "extra_headers", None),
        ("extra_body", "extra_body", None),
        ("parallel_tool_calls", "parallel_tool_calls", None),
        ("time_out", "timeout", float),
    ]

    settings: ModelSettings = {}
    for attr, key, transform in _PARAM_MAPPINGS:
        value = getattr(params, attr)
        if value is not None:
            settings[key] = transform(value) if transform else value  # type: ignore[literal-required]

    return settings


def _provider_params_to_settings(model_config: ModelConfig) -> ModelSettings | None:
    """Convert provider-specific parameters to prefixed ModelSettings."""
    if not model_config.parameters:
        return None

    provider_type = model_config.provider.type
    provider_params = getattr(model_config.parameters, provider_type.value, None)
    if not provider_params:
        return None

    prefix = provider_type.value + "_"
    settings: ModelSettings = {}

    for field_name in provider_params.__pydantic_fields__:
        value = getattr(provider_params, field_name)
        if value is not None:
            if isinstance(value, Enum):
                value = value.value
            elif isinstance(value, BaseModel):
                value = value.model_dump(exclude_none=True)
            settings[prefix + field_name] = value  # type: ignore[literal-required]

    return settings if settings else None


def _build_result_type(result_type: TaskResultType | None) -> type | None:
    """Build a dynamic type from TaskResultType using StructuredDict."""
    if result_type is None:
        return None

    return StructuredDict(
        result_type.json_schema,
        name=result_type.name,
        description=result_type.description,
    )


def _build_artifact(result: Any, task_config: TaskConfig) -> Artifact:
    """Wrap a Marvin task result in an A2A Artifact."""
    name = task_config.result_type.name if task_config.result_type else "result"
    description = (
        task_config.result_type.description if task_config.result_type else None
    )

    if isinstance(result, str):
        return new_text_artifact(name=name, text=result, description=description)
    elif isinstance(result, dict):
        return new_data_artifact(name=name, data=result, description=description)
    elif isinstance(result, list):
        return new_data_artifact(
            name=name, data={"items": result}, description=description
        )
    else:
        return new_text_artifact(name=name, text=str(result), description=description)
