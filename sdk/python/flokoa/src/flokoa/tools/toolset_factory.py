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
from typing import Generic, Protocol, TypedDict, TypeVar, cast

from flokoa_types import IntegrationType, ToolDefinition, ToolType

logger = logging.getLogger("flokoa." + __name__)


class ToolObject(Protocol):
    """Marker protocol for framework-specific tool objects."""


ToolObjectT = TypeVar("ToolObjectT", bound=ToolObject)

ToolsetBuilder = Callable[[ToolDefinition], list[ToolObjectT]]
"""A builder takes a ToolDefinition and returns framework-specific tool objects."""


class IntegrationBuilders(TypedDict, Generic[ToolObjectT], total=False):
    by_integration: dict[IntegrationType, ToolsetBuilder[ToolObjectT]]


class BuilderRegistry(TypedDict, Generic[ToolObjectT], total=False):
    openapi: IntegrationBuilders[ToolObjectT]


@dataclass(slots=True)
class ToolsetFactory[ToolObjectT: ToolObject]:
    """Framework-agnostic factory for building tools from Flokoa tool definitions.

    Builders are registered per ``(ToolType, IntegrationType)`` pair so each
    framework integration (PydanticAI, Google ADK, ...) can provide its
    own builder for every tool type.

    Usage::

        from flokoa.tools import default_factory
        from flokoa_types import IntegrationType, ToolType

        default_factory.register(
            ToolType.OPENAPI, IntegrationType.PYDANTIC_AI, my_builder
        )

        tools = default_factory.build(
            tool_definitions, integration=IntegrationType.PYDANTIC_AI
        )
    """

    _builders: BuilderRegistry[ToolObjectT] = field(
        default_factory=lambda: cast(BuilderRegistry[ToolObjectT], {})
    )

    def register(
        self,
        tool_type: ToolType,
        integration: IntegrationType,
        builder: ToolsetBuilder[ToolObjectT],
    ) -> None:
        """Register a builder for a ``(tool_type, integration)`` pair.

        Args:
            tool_type: The tool type this builder handles.
            integration: The framework integration this builder targets.
            builder: Callable that converts a ToolDefinition into
                framework-specific tool objects.
        """
        if tool_type != ToolType.OPENAPI:
            logger.warning(
                "Unsupported tool_type='%s', skipping builder registration",
                tool_type,
            )
            return

        openapi_builders = self._builders.setdefault("openapi", {"by_integration": {}})
        openapi_builders.setdefault("by_integration", {})[integration] = builder

    def build(
        self,
        tool_definitions: list[ToolDefinition],
        integration: IntegrationType,
    ) -> list[ToolObjectT]:
        """Build tools for *integration* from all tool definitions.

        Each definition is dispatched to its registered builder.

        Args:
            tool_definitions: List of Flokoa tool definitions.
            integration: The framework integration to build tools for.

        Returns:
            A flat list of framework-specific tool objects.
        """
        tools: list[ToolObjectT] = []
        for td in tool_definitions:
            if td.type != ToolType.OPENAPI:
                builder = None
            else:
                builder = self._builders.get("openapi", {}).get("by_integration", {}).get(integration)
            if builder is None:
                logger.warning(
                    "No builder for (tool_type='%s', integration='%s'), skipping '%s'",
                    td.type,
                    integration.value,
                    td.name,
                )
                continue
            built = builder(td)
            tools.extend(built)
            logger.info(
                "Built %d tool(s) from '%s' for integration '%s'",
                len(built),
                td.name,
                integration.value,
            )
        return tools


default_factory: ToolsetFactory[ToolObject] = ToolsetFactory()
