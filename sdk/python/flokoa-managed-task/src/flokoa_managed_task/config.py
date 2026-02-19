"""Configuration loading for managed tasks.

Supports two loading modes:

1. **Unified config** — a single ``agent-config.json`` with ``agentType: task``
   (preferred, new path).
2. **Legacy config** — ``FLOKOA_TASK_CONFIG`` env var + scattered files
   for backward compatibility.
"""

import os

from flokoa.config import AgentConfig, load_agent_config, load_legacy_task_config
from flokoa.utils import load_instruction, load_model_config
from flokoa_types import TaskConfig

TASK_CONFIG_ENV = "FLOKOA_TASK_CONFIG"

# Re-export so existing imports from this module continue to work.
__all__ = ["load_instruction", "load_model_config", "load_task_config", "load_managed_task_config"]


def load_task_config() -> TaskConfig:
    """Load TaskConfig from the FLOKOA_TASK_CONFIG environment variable.

    Retained for backward compatibility.

    Raises:
        RuntimeError: If the environment variable is not set.
    """
    raw = os.environ.get(TASK_CONFIG_ENV)
    if not raw:
        raise RuntimeError(f"{TASK_CONFIG_ENV} environment variable is not set")
    return TaskConfig.model_validate_json(raw)


def load_managed_task_config() -> AgentConfig:
    """Load task config, trying unified config first, then legacy.

    Returns:
        A validated :class:`AgentConfig` wrapping a :class:`TaskAgentConfig`.

    Raises:
        FileNotFoundError: If unified config not found.
        RuntimeError: If legacy env var not set.
    """
    # 1. Try unified config
    unified_path = os.environ.get("FLOKOA_AGENT_CONFIG_PATH", "/etc/flokoa/agent-config.json")
    if os.path.exists(unified_path):
        return load_agent_config(unified_path)

    # 2. Fall back to legacy env var + files
    return load_legacy_task_config()
