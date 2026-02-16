"""Configuration loading for managed tasks.

Loads TaskConfig from the FLOKOA_TASK_CONFIG env var,
ModelConfig from /etc/flokoa/model.json, and instruction
text from /etc/flokoa/instruction.txt.
"""

import os

from flokoa_types import ModelConfig, TaskConfig

TASK_CONFIG_ENV = "FLOKOA_TASK_CONFIG"
MODEL_CONFIG_PATH = "/etc/flokoa/model.json"
INSTRUCTION_PATH = "/etc/flokoa/instruction.txt"


def load_task_config() -> TaskConfig:
    """Load TaskConfig from the FLOKOA_TASK_CONFIG environment variable.

    Raises:
        RuntimeError: If the environment variable is not set.
    """
    raw = os.environ.get(TASK_CONFIG_ENV)
    if not raw:
        raise RuntimeError(f"{TASK_CONFIG_ENV} environment variable is not set")
    return TaskConfig.model_validate_json(raw)


def load_model_config() -> ModelConfig | None:
    """Load ModelConfig from /etc/flokoa/model.json if present."""
    path = os.environ.get("FLOKOA_MODEL_CONFIG_PATH", MODEL_CONFIG_PATH)
    if not os.path.exists(path):
        return None
    with open(path) as f:
        return ModelConfig.model_validate_json(f.read())


def load_instruction() -> str | None:
    """Load instruction text from /etc/flokoa/instruction.txt if present."""
    path = os.environ.get("FLOKOA_INSTRUCTION_PATH", INSTRUCTION_PATH)
    if not os.path.exists(path):
        return None
    with open(path) as f:
        return f.read()
