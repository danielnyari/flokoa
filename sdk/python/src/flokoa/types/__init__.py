from enum import StrEnum
from typing import Annotated, Any

from pydantic import BaseModel, ConfigDict, Field


class ToolType(StrEnum):
    API = "api"


class APIToolSpec(BaseModel):
    model_config = ConfigDict(validate_by_alias=True, validate_by_name=True, frozen=True)
    endpoint: Annotated[str, Field(description="The HTTP endpoint URL for the API tool.")]
    method: Annotated[
        str,
        Field(description="The HTTP method to use when calling the API tool (e.g., GET, POST)."),
    ]


ToolSpecUnion = APIToolSpec


class ToolDefinition(BaseModel):
    model_config = ConfigDict(validate_by_alias=True, validate_by_name=True, frozen=True)
    name: Annotated[str, Field(description="The unique name of the tool.")]
    type: Annotated[ToolType, Field(description="The type of the tool.")]
    description: Annotated[str, Field(description="A brief description of the tool's functionality.")]
    input_json_schema: Annotated[
        dict[str, Any],
        Field(
            alias="inputJSONSchema",
            description="JSON schema defining the tool's input parameters.",
        ),
    ]
    output_json_schema: Annotated[
        dict[str, Any],
        Field(
            alias="outputJSONSchema",
            description="JSON schema defining the tool's output parameters.",
        ),
    ]
    metadata: Annotated[dict[str, Any] | None, Field(description="Additional metadata for the tool.")] = None

    spec: Annotated[ToolSpecUnion, Field(description="The specification details of the tool.")]
