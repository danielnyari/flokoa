"""Extended tool configuration supporting multiple resolution strategies.

Supports three tool types:

- **openapi** — OpenAPI spec-based tools (existing Flokoa pattern)
- **function** — Plain Python functions resolved by dotted path
- **class** — Tool classes instantiated from config

Inspired by Google ADK's multi-strategy tool resolution (instance, class,
factory function, plain function) adapted for Flokoa's declarative config.
"""

from __future__ import annotations

from enum import StrEnum
from typing import Any

from pydantic import BaseModel, ConfigDict, Field, model_validator

from flokoa.config.code_ref import CodeRef


class ToolRefType(StrEnum):
    """How a tool is specified in the agent config."""

    OPENAPI = "openapi"
    FUNCTION = "function"
    CLASS = "class"


class ToolConfig(BaseModel):
    """Unified tool configuration supporting OpenAPI, function, and class tools.

    Each tool must have exactly one source specified, matching its ``type``:

    - ``openapi`` — requires ``open_api`` with an OpenAPI spec
    - ``function`` — requires ``code`` pointing to a callable
    - ``class`` — requires ``code`` pointing to a tool class (with optional args)

    Examples::

        # OpenAPI tool (existing pattern, wrapped)
        ToolConfig(
            name="petstore",
            type="openapi",
            open_api={"openApiSchema": {"value": {...}}, "url": "https://..."},
        )

        # Function tool
        ToolConfig(
            name="calculate_price",
            type="function",
            code=CodeRef(name="my_app.tools.calculate_price"),
        )

        # Class tool with constructor args
        ToolConfig(
            name="web_search",
            type="class",
            code=CodeRef(
                name="my_app.tools.WebSearchTool",
                args=[{"name": "max_results", "value": 10}],
            ),
        )
    """

    model_config = ConfigDict(extra="forbid", populate_by_name=True)

    name: str = Field(description="Unique tool name.")
    type: ToolRefType = Field(
        default=ToolRefType.OPENAPI,
        description="Tool resolution strategy.",
    )
    description: str = Field(
        default="",
        description="Human-readable description for the LLM.",
    )

    # OpenAPI source
    open_api: dict[str, Any] | None = Field(
        default=None,
        alias="openApi",
        description="OpenAPI spec configuration (for type=openapi).",
    )

    # Code reference source (for function and class tools)
    code: CodeRef | None = Field(
        default=None,
        description="Python code reference (for type=function or type=class).",
    )

    @model_validator(mode="after")
    def _validate_source(self) -> ToolConfig:
        if self.type == ToolRefType.OPENAPI and self.open_api is None:
            raise ValueError("Tools with type='openapi' require the 'openApi' field.")
        if self.type in (ToolRefType.FUNCTION, ToolRefType.CLASS) and self.code is None:
            raise ValueError(f"Tools with type='{self.type.value}' require the 'code' field.")
        return self
