"""Configuration loading for the managed-agent runtime.

Supports two loading modes:

1. **Unified config** — a single ``agent-config.json`` file containing the
   full :class:`AgentConfig` (preferred, new path).
2. **Legacy config** — scattered files (``template-config.json``,
   ``instruction.txt``, ``model.json``) for backward compatibility.
"""

import json
import os

from flokoa.config import AgentConfig, load_agent_config
from flokoa_types import TemplateConfig

# Legacy path
TEMPLATE_CONFIG_PATH = "/etc/flokoa/template-config.json"


def load_managed_agent_config() -> AgentConfig | None:
    """Try to load unified agent config from ``agent-config.json``.

    Returns:
        A validated :class:`AgentConfig` if the file exists, ``None`` otherwise.
    """
    unified_path = os.environ.get("FLOKOA_AGENT_CONFIG_PATH", "/etc/flokoa/agent-config.json")
    if os.path.exists(unified_path):
        return load_agent_config(unified_path)
    return None


def load_templated_config() -> TemplateConfig:
    """Load templated agent configuration from /etc/flokoa/template-config.json.

    Retained for backward compatibility with existing code that depends
    on the :class:`TemplateConfig` type directly.

    Returns:
        TemplateConfig parsed from the config file.

    Raises:
        FileNotFoundError: If the config file does not exist.
    """
    path = os.environ.get("FLOKOA_TEMPLATE_CONFIG_PATH", TEMPLATE_CONFIG_PATH)
    if not os.path.exists(path):
        raise FileNotFoundError(f"Templated config file not found at {path}")

    with open(path) as f:
        config_data = json.load(f)

    return TemplateConfig.model_validate(config_data)
