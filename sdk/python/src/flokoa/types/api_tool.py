from typing import Annotated

from pydantic import BaseModel, ConfigDict, Field


class APIToolSpec(BaseModel):
    model_config = ConfigDict(validate_by_alias=True, validate_by_name=True, frozen=True)
    endpoint: Annotated[str, Field(description="The HTTP endpoint URL for the API tool.")]
    method: Annotated[
        str,
        Field(description="The HTTP method to use when calling the API tool (e.g., GET, POST)."),
    ]
