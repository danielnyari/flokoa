"""Entry point for the flokoa-managed-task runtime.

Supports two configuration modes:

1. **Unified config** — a single ``agent-config.json`` with ``agentType: task``.
2. **Legacy config** — ``FLOKOA_TASK_CONFIG`` env var + scattered files.

Usage:
    python -m flokoa_managed_task
"""

import logging
import os

from flokoa.config import TaskAgentConfig
from flokoa_managed_task.bootstrap import execute_task, execute_task_from_config
from flokoa_managed_task.config import (
    load_instruction,
    load_managed_task_config,
    load_model_config,
    load_task_config,
)

OUTPUT_PATH = "/tmp/output"  # noqa: S108

logger = logging.getLogger(__name__)


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
            raise TypeError(
                f"Expected TaskAgentConfig in unified config, got {type(agent_config.root).__name__}"
            )
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

    output = artifact.model_dump_json()
    logger.info("Task completed, writing A2A artifact (%d chars)", len(output))

    with open(OUTPUT_PATH, "w") as f:
        f.write(output)


if __name__ == "__main__":
    main()
