from __future__ import annotations

import logging
from typing import Any

from flokoa.types import ManagedConfig

logger = logging.getLogger(__name__)


class ManagedAgentBuilder:
    """Builder for managed pydantic-ai agent configuration.

    Holds the managed config loaded from the operator and provides
    extension points for future agent construction sophistication
    (output schema constraints, result validators, retry policies, etc.).

    Usage:
        config = load_managed_config()
        builder = ManagedAgentBuilder(config=config)
        # The executor uses the builder to access config and build the agent.
    """

    def __init__(self, config: ManagedConfig | None = None) -> None:
        self._config = config

    @property
    def config(self) -> ManagedConfig | None:
        return self._config

    @property
    def output_schema(self) -> dict[str, Any] | None:
        """Get the output schema from the managed config, if any."""
        if self._config:
            return self._config.output_schema
        return None

    def build_output_type(self) -> type | None:
        """Build a structured output type from the output schema.

        TODO: Generate a Pydantic model from the JSON Schema output_schema
        to constrain agent responses to the declared format.
        """
        return None

    def build_result_validators(self) -> list | None:
        """Build result validators from managed config.

        TODO: Create pydantic-ai result validators from managed config
        constraints (e.g. schema validation, content filters).
        """
        return None
