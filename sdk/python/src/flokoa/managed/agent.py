from __future__ import annotations

import logging
from typing import Any

from pydantic import BaseModel, Field, create_model
from pydantic_ai import Agent

from flokoa.types import ManagedConfig

logger = logging.getLogger(__name__)


class ManagedAgentBuilder:
    """Builder for constructing a pydantic-ai Agent from managed runtime configuration.

    This builder creates an Agent entirely from declarative configuration
    (instruction, output schema) rather than from user-provided code.

    Usage:
        builder = ManagedAgentBuilder()
        builder.set_instruction("You are a helpful assistant.")
        builder.set_output_schema({"type": "object", "properties": {"answer": {"type": "string"}}})
        agent = builder.build()
    """

    def __init__(self) -> None:
        self._instruction: str | None = None
        self._output_schema: dict[str, Any] | None = None
        self._name: str | None = None

    def set_instruction(self, instruction: str) -> ManagedAgentBuilder:
        self._instruction = instruction
        return self

    def set_output_schema(self, schema: dict[str, Any]) -> ManagedAgentBuilder:
        self._output_schema = schema
        return self

    def set_name(self, name: str) -> ManagedAgentBuilder:
        self._name = name
        return self

    @classmethod
    def from_managed_config(cls, config: ManagedConfig, instruction: str | None = None) -> ManagedAgentBuilder:
        """Create a builder pre-populated from a ManagedConfig and instruction.

        Args:
            config: The managed config loaded from the operator ConfigMap.
            instruction: The system instruction loaded from the instruction ConfigMap.
        """
        builder = cls()
        if instruction:
            builder.set_instruction(instruction)
        if config.output_schema:
            builder.set_output_schema(config.output_schema)
        return builder

    def _build_output_type(self) -> type[BaseModel] | None:
        """Build a Pydantic model from the JSON Schema output_schema."""
        if not self._output_schema:
            return None

        properties = self._output_schema.get("properties", {})
        required = set(self._output_schema.get("required", []))

        if not properties:
            return None

        field_definitions: dict[str, Any] = {}
        for field_name, field_schema in properties.items():
            field_type = _json_schema_type_to_python(field_schema)
            description = field_schema.get("description", "")

            if field_name in required:
                field_definitions[field_name] = (field_type, Field(description=description))
            else:
                field_definitions[field_name] = (field_type | None, Field(default=None, description=description))

        model_name = self._name or "ManagedAgentOutput"
        model_description = self._output_schema.get("description", "")

        output_model = create_model(model_name, **field_definitions)
        if model_description:
            output_model.__doc__ = model_description

        logger.info("Built output model '%s' with %d fields from JSON Schema", model_name, len(field_definitions))
        return output_model

    def build(self) -> Agent:
        """Build and return a configured pydantic-ai Agent.

        Returns:
            A pydantic-ai Agent configured with instruction and optional output schema.
        """
        output_type = self._build_output_type()

        kwargs: dict[str, Any] = {}
        if self._instruction:
            kwargs["instructions"] = self._instruction
        if self._name:
            kwargs["name"] = self._name

        if output_type is not None:
            kwargs["output_type"] = output_type
            logger.info("Building managed agent with structured output: %s", output_type.__name__)
        else:
            logger.info("Building managed agent with plain text output")

        agent: Agent = Agent(**kwargs)
        return agent


_JSON_SCHEMA_TYPE_MAP: dict[str, type] = {
    "string": str,
    "integer": int,
    "number": float,
    "boolean": bool,
}


def _json_schema_type_to_python(schema: dict[str, Any]) -> type:
    """Convert a JSON Schema type to a Python type.

    Handles basic types and arrays. Nested objects are represented as dict.
    """
    schema_type = schema.get("type", "string")

    if schema_type == "array":
        items = schema.get("items", {})
        item_type = _json_schema_type_to_python(items)
        return list[item_type]  # type: ignore[valid-type]

    if schema_type == "object":
        return dict[str, Any]

    return _JSON_SCHEMA_TYPE_MAP.get(schema_type, str)
