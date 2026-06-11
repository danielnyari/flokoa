# Copyright 2026 Flokoa Contributors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from __future__ import annotations

import logging
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Protocol, TypeVar

from flokoa_types import ToolDefinition, ToolType

logger = logging.getLogger("flokoa." + __name__)


class ToolObject(Protocol):
    """Marker protocol for framework-specific tool objects."""


ToolObjectT = TypeVar("ToolObjectT", bound=ToolObject)

ToolsetBuilder = Callable[[ToolDefinition], list[ToolObjectT]]
"""A builder takes a ToolDefinition and returns framework-specific tool objects."""


@dataclass(slots=True)
class ToolsetFactory[ToolObjectT: ToolObject]:
    """Factory for building tools from Flokoa tool definitions.

    Builders are registered per :class:`ToolType`.

    Usage::

        from flokoa.tools import default_factory
        from flokoa_types import ToolType

        default_factory.register(ToolType.OPENAPI, my_builder)

        tools = default_factory.build(tool_definitions)
    """

    _builders: dict[ToolType, ToolsetBuilder[ToolObjectT]] = field(default_factory=dict)

    def register(
        self,
        tool_type: ToolType,
        builder: ToolsetBuilder[ToolObjectT],
    ) -> None:
        """Register a builder for a tool type.

        Args:
            tool_type: The tool type this builder handles.
            builder: Callable that converts a ToolDefinition into
                framework-specific tool objects.
        """
        if tool_type != ToolType.OPENAPI:
            logger.warning(
                "Unsupported tool_type='%s', skipping builder registration",
                tool_type,
            )
            return

        self._builders[tool_type] = builder

    def build(
        self,
        tool_definitions: list[ToolDefinition],
    ) -> list[ToolObjectT]:
        """Build tools from all tool definitions.

        Each definition is dispatched to its registered builder.

        Args:
            tool_definitions: List of Flokoa tool definitions.

        Returns:
            A flat list of framework-specific tool objects.
        """
        logger.debug(
            "ToolsetFactory.build(): %d definition(s), registered_builders=%s",
            len(tool_definitions),
            list(self._builders.keys()),
        )
        tools: list[ToolObjectT] = []
        for td in tool_definitions:
            logger.debug(
                "Processing tool '%s': type=%s, has_openApi=%s",
                td.name,
                td.type,
                td.spec.open_api is not None if td.spec else False,
            )
            builder = self._builders.get(td.type)
            if builder is None:
                logger.warning(
                    "No builder for tool_type='%s', skipping '%s'",
                    td.type,
                    td.name,
                )
                continue
            try:
                built = builder(td)
            except Exception:
                logger.exception("Failed to build tool '%s'", td.name)
                continue
            tools.extend(built)
            logger.info(
                "Built %d tool(s) from '%s': %s",
                len(built),
                td.name,
                [getattr(t, "name", str(t)) for t in built],
            )
        logger.info("ToolsetFactory.build() complete: %d total tool(s)", len(tools))
        return tools


default_factory: ToolsetFactory[ToolObject] = ToolsetFactory()
