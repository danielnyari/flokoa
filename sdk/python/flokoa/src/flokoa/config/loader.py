"""Configuration loading for unified AgentConfig.

Supports loading from:
- A single JSON or YAML file
- A raw dict
- The legacy scattered-files layout (for backward compatibility with
  operator-mounted configs)
"""

from __future__ import annotations

import json
import logging
import os
from typing import Any

from flokoa.config.agent_config import AgentConfig

logger = logging.getLogger(__name__)

# Default path for the unified agent config file
AGENT_CONFIG_PATH = "/etc/flokoa/agent-config.json"

# Legacy paths (still supported for backward compat)
_LEGACY_TEMPLATE_CONFIG_PATH = "/etc/flokoa/template-config.json"
_LEGACY_INSTRUCTION_PATH = "/etc/flokoa/instruction.txt"
_LEGACY_MODEL_CONFIG_PATH = "/etc/flokoa/model.json"


def load_agent_config(
    path: str | None = None,
) -> AgentConfig:
    """Load an :class:`AgentConfig` from a JSON file.

    Checks (in order):
    1. Explicit ``path`` argument
    2. ``FLOKOA_AGENT_CONFIG_PATH`` environment variable
    3. Default path ``/etc/flokoa/agent-config.json``

    Args:
        path: Explicit path to the config file.

    Returns:
        A validated :class:`AgentConfig`.

    Raises:
        FileNotFoundError: If the config file does not exist.
    """
    config_path = path or os.environ.get("FLOKOA_AGENT_CONFIG_PATH") or AGENT_CONFIG_PATH

    if not os.path.exists(config_path):
        raise FileNotFoundError(f"Agent config file not found at {config_path}")

    with open(config_path) as f:
        raw = f.read()

    # Try YAML first if pyyaml is available, fall back to JSON
    data = _parse_content(raw, config_path)
    return AgentConfig.model_validate(data)


def load_agent_config_from_dict(data: dict[str, Any]) -> AgentConfig:
    """Load an :class:`AgentConfig` from a raw dictionary.

    Args:
        data: A dictionary matching the AgentConfig schema.

    Returns:
        A validated :class:`AgentConfig`.
    """
    return AgentConfig.model_validate(data)


def load_legacy_llm_config(
    template_config_path: str | None = None,
    instruction_path: str | None = None,
    model_config_path: str | None = None,
    name: str = "managed-agent",
) -> AgentConfig:
    """Build an :class:`AgentConfig` from legacy operator-mounted files.

    This provides backward compatibility with the existing operator layout
    where config is scattered across multiple files.

    Args:
        template_config_path: Path to template-config.json.
        instruction_path: Path to instruction.txt.
        model_config_path: Path to model.json.
        name: Agent name (defaults to ``"managed-agent"``).

    Returns:
        A validated :class:`AgentConfig` wrapping an :class:`LlmAgentConfig`.
    """
    t_path = template_config_path or os.environ.get("FLOKOA_TEMPLATE_CONFIG_PATH", _LEGACY_TEMPLATE_CONFIG_PATH)
    i_path = instruction_path or os.environ.get("FLOKOA_INSTRUCTION_PATH", _LEGACY_INSTRUCTION_PATH)
    m_path = model_config_path or _LEGACY_MODEL_CONFIG_PATH

    data: dict[str, Any] = {
        "agentType": "llm",
        "name": name,
    }

    # Load template config (output schema)
    if os.path.exists(t_path):
        with open(t_path) as f:
            template_data = json.load(f)
        if "outputSchema" in template_data:
            data["outputSchema"] = template_data["outputSchema"]
        if "inputSchema" in template_data:
            data["inputSchema"] = template_data["inputSchema"]

    # Load instruction
    if os.path.exists(i_path):
        with open(i_path) as f:
            data["instruction"] = f.read()

    # Load model config
    if os.path.exists(m_path):
        with open(m_path) as f:
            data["model"] = json.load(f)

    return AgentConfig.model_validate(data)


def _parse_content(raw: str, path: str) -> dict[str, Any]:
    """Parse raw file content as YAML (if available) or JSON."""
    if path.endswith((".yaml", ".yml")):
        try:
            import yaml

            return yaml.safe_load(raw)
        except ImportError:
            logger.warning("PyYAML not installed; falling back to JSON parsing for %s", path)

    return json.loads(raw)
