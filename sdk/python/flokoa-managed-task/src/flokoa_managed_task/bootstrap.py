"""Bootstrap logic for instantiating and executing Marvin tasks from config."""

from __future__ import annotations

import logging
from typing import Any

import marvin
from a2a.types import Artifact
from a2a.utils import new_data_artifact, new_text_artifact
from marvin import Agent as MarvinAgent
from pydantic_ai import StructuredDict

from flokoa.integrations.pydantic_ai.model_factory import create_model_from_config
from flokoa_types import ModelConfig, TaskConfig, TaskResultType

logger = logging.getLogger(__name__)


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

    model = create_model_from_config(model_config)

    agent_kwargs: dict[str, Any] = {"model": model}

    if task_config.agent:
        agent_kwargs["name"] = task_config.agent.name
        if task_config.agent.instructions:
            agent_kwargs["instructions"] = task_config.agent.instructions

    return MarvinAgent(**agent_kwargs)


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
