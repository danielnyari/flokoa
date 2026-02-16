"""Entry point for the flokoa-managed-task runtime.

Usage:
    python -m flokoa_managed_task
"""

import logging

from flokoa_managed_task.bootstrap import execute_task
from flokoa_managed_task.config import (
    load_instruction,
    load_model_config,
    load_task_config,
)

OUTPUT_PATH = "/tmp/output"

logger = logging.getLogger(__name__)


def main() -> None:
    """Load config, execute the Marvin task, and write output."""
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(name)s %(levelname)s %(message)s",
    )

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
