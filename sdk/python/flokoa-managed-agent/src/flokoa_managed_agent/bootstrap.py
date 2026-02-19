from __future__ import annotations

import logging
from typing import Any

<<<<<<< claude/improve-agent-config-AgzoG
from pydantic_ai import Agent, StructuredDict

from flokoa.config import AgentConfig, LlmAgentConfig, get_builder
=======
>>>>>>> main
from flokoa_types import TemplateConfig
from pydantic_ai import Agent, StructuredDict

logger = logging.getLogger(__name__)


class TemplatedAgentBuilder:
    """Builder for templated pydantic-ai agent configuration.

    Retained for backward compatibility.  New code should prefer
    :func:`build_agent_from_config` which uses the unified
    :class:`AgentConfig` and builder registry.

    Usage:
        config = load_templated_config()
        builder = TemplatedAgentBuilder(config=config)
        # The executor uses the builder to access config and build the agent.
    """

    def __init__(self, config: TemplateConfig) -> None:
        self._config = config

    @property
    def config(self) -> TemplateConfig:
        return self._config

    @property
    def output_schema(self) -> type[dict[str, Any]]:
        return StructuredDict(
            self.config.output_schema.json_schema,
            name=self.config.output_schema.name,
            description=self.config.output_schema.description,
        )

    @classmethod
    def from_config(cls, config: TemplateConfig) -> Agent[None, dict[str, Any]]:
        """Create a builder instance from the given config."""
        instance = cls(config=config)
<<<<<<< claude/improve-agent-config-AgzoG
        return Agent(output_type=instance.output_schema)


def build_agent_from_config(config: AgentConfig) -> Any:
    """Build an agent from a unified :class:`AgentConfig`.

    Uses the builder registry to dispatch to the appropriate builder
    based on ``agent_type`` and ``framework``.

    Args:
        config: A validated :class:`AgentConfig`.

    Returns:
        A live agent instance (framework-specific type).
    """
    inner = config.root
    agent_type = inner.agent_type

    if isinstance(inner, LlmAgentConfig):
        framework = inner.framework.value
    else:
        framework = "marvin"

    builder_cls = get_builder(agent_type, framework)
    return builder_cls.from_config(inner)
=======
        return Agent(output_type=instance.output_schema, instrument=True)
>>>>>>> main
