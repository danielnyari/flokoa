"""Configuration loading for managed tasks.

Loads TaskConfig from the FLOKOA_TASK_CONFIG env var.
Model and instruction loading is delegated to the flokoa SDK utils.
"""

import os

from flokoa.utils import load_instruction, load_model_config
from flokoa_types import TaskConfig

TASK_CONFIG_ENV = "FLOKOA_TASK_CONFIG"

# Re-export so existing imports from this module continue to work.
__all__ = ["load_instruction", "load_model_config", "load_task_config"]


def load_task_config() -> TaskConfig:
    """Load TaskConfig from the FLOKOA_TASK_CONFIG environment variable.

    Raises:
        RuntimeError: If the environment variable is not set.
    """
    raw = os.environ.get(TASK_CONFIG_ENV)
    if not raw:
        raise RuntimeError(f"{TASK_CONFIG_ENV} environment variable is not set")
    return TaskConfig.model_validate_json(raw)
