"""Entry point for the flokoa-managed-task runtime.

Supports two configuration modes:

1. **Unified config** — a single ``agent-config.json`` with ``agentType: task``.
2. **Legacy config** — ``FLOKOA_TASK_CONFIG`` env var + scattered files.

Usage:
    python -m flokoa_managed_task
"""

from __future__ import annotations

import json
import logging
import os
from typing import Any

from a2a.types import Artifact
from flokoa.config import TaskAgentConfig

from flokoa_managed_task.bootstrap import execute_task, execute_task_from_config
from flokoa_managed_task.config import (
    load_instruction,
    load_managed_task_config,
    load_model_config,
    load_task_config,
)

RESULT_PATH = "/tmp/result"  # noqa: S108
ARTIFACT_PATH = "/tmp/artifact"  # noqa: S108

logger = logging.getLogger(__name__)


def extract_text(artifact: Artifact) -> str:
    """Extract plain text content from an A2A Artifact.

    Returns the text from the first TextPart, or a JSON representation
    of the first DataPart's data if no text parts are found.
    """
    for part in artifact.parts:
        if hasattr(part, "text") and isinstance(part.text, str):
            return part.text
    for part in artifact.parts:
        if hasattr(part, "data"):
            return json.dumps(part.data, default=_json_default)
    return ""


def _json_default(obj: Any) -> Any:
    """Fallback serializer for non-standard types."""
    return str(obj)


def main() -> None:
    """Load config, execute the Marvin task, and write output."""
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(name)s %(levelname)s %(message)s",
    )

    # Try unified config first
    unified_path = os.environ.get("FLOKOA_AGENT_CONFIG_PATH", "/etc/flokoa/agent-config.json")
    if os.path.exists(unified_path):
        agent_config = load_managed_task_config()
        if isinstance(agent_config.root, TaskAgentConfig):
            logger.info("Using unified AgentConfig (task_type=%s)", agent_config.root.task_type.value)
            artifact = execute_task_from_config(agent_config)
        else:
            raise TypeError(f"Expected TaskAgentConfig in unified config, got {type(agent_config.root).__name__}")
    else:
        # Legacy path
        task_config = load_task_config()
        logger.info("Loaded task config: type=%s", task_config.type.value)

        model_config = load_model_config()
        if model_config:
            logger.info(
                "Loaded model config: %s/%s",
                model_config.provider.type.value,
                model_config.model,
            )

        instruction = load_instruction()
        if instruction:
            logger.info("Loaded instruction (%d chars)", len(instruction))

        artifact = execute_task(task_config, model_config, instruction)

    # Write A2A Artifact JSON (for field-level access via {{tasks.x.output.field}})
    artifact_json = artifact.model_dump_json()
    logger.info("Task completed, writing A2A artifact (%d chars)", len(artifact_json))
    with open(ARTIFACT_PATH, "w") as f:
        f.write(artifact_json)

    # Write plain text result (for {{tasks.x.output}})
    result_text = extract_text(artifact)
    logger.info("Writing plain text result (%d chars)", len(result_text))
    with open(RESULT_PATH, "w") as f:
        f.write(result_text)


if __name__ == "__main__":
    main()
