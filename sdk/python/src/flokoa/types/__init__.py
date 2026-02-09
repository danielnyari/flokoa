from enum import StrEnum
from typing import Annotated, Any

from pydantic import BaseModel, ConfigDict, Field, computed_field

from flokoa.types.agenttool import AgentToolSpec, Type
from flokoa.types.managedconfig import ManagedConfig
from flokoa.types.modelconfig import (
    AnthropicModelParameters,
    AnthropicProviderConfig,
    BedrockModelParameters,
    BedrockProviderConfig,
    GoogleModelParameters,
    GoogleProviderConfig,
    ModelConfig,
    ModelParameters,
    OpenAIModelParameters,
    OpenAIProviderConfig,
    ProviderType,
)

ProviderConfigType = Annotated[
    AnthropicProviderConfig | OpenAIProviderConfig | GoogleProviderConfig | BedrockProviderConfig,
    Field(description="The provider-specific configuration for the model."),
]

ProviderModelParametersType = Annotated[
    AnthropicModelParameters | OpenAIModelParameters | GoogleModelParameters | BedrockModelParameters,
    Field(description="The provider-specific model parameters."),
]


class ToolType(StrEnum):
    """Tool type enum matching the Kubernetes CRD Type enum."""

    HTTP_API = "http-api"


class ToolDefinition(BaseModel):
    """Tool definition wrapper that adds name to AgentToolSpec.

    The name field comes from Kubernetes object metadata and is not part
    of the AgentToolSpec. This class provides a complete tool definition
    with convenient property accessors for the spec fields.
    """

    model_config = ConfigDict(validate_by_alias=True, validate_by_name=True, frozen=True)
    name: Annotated[str, Field(description="The unique name of the tool.")]
    spec: Annotated[AgentToolSpec, Field(description="The AgentTool specification from the CRD.")]
    metadata: Annotated[dict[str, Any] | None, Field(description="Additional metadata for the tool.")] = None

    @computed_field
    @property
    def type(self) -> ToolType:
        """Get the tool type from the spec."""
        if self.spec.type == Type.http_api:
            return ToolType.HTTP_API
        return ToolType.HTTP_API  # Default fallback

    @computed_field
    @property
    def description(self) -> str:
        """Get the tool description from the spec."""
        return self.spec.description

    @computed_field
    @property
    def input_json_schema(self) -> dict[str, Any]:
        """Get the input JSON schema from the spec."""
        return self.spec.input_schema or {}

    @computed_field
    @property
    def output_json_schema(self) -> dict[str, Any]:
        """Get the output JSON schema from the spec."""
        return self.spec.output_schema or {}


__all__ = [
    "ManagedConfig",
    "ModelConfig",
    "ModelParameters",
    "ProviderConfigType",
    "ProviderModelParametersType",
    "ProviderType",
    "ToolDefinition",
    "ToolType",
]
