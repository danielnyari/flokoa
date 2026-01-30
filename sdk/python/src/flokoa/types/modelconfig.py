"""Model configuration types leveraging PydanticAI's ModelSettings.

These types represent the model configuration that is mounted at /etc/flokoa/model.json
by the Flokoa operator when an Agent references a Model or ModelConfig resource.
"""

from __future__ import annotations

from enum import Enum
from typing import Any

from pydantic import BaseModel, Field

# Type alias for settings - compatible with PydanticAI's ModelSettings TypedDict
# We use dict[str, Any] because ModelSettings contains httpx.Timeout which
# Pydantic can't directly serialize. At runtime, the dict is compatible with
# ModelSettings and can be passed directly to PydanticAI agents.
ModelSettings = dict[str, Any]


class ModelProvider(str, Enum):
    """Model provider enum matching the Kubernetes CRD ModelProvider enum."""

    OPENAI = "openai"
    ANTHROPIC = "anthropic"
    AZURE_OPENAI = "azure-openai"
    OLLAMA = "ollama"
    GEMINI = "gemini"
    GEMINI_VERTEX = "gemini-vertex"
    ANTHROPIC_VERTEX = "anthropic-vertex"
    BEDROCK = "bedrock"


class ModelConfig(BaseModel):
    """Model configuration loaded from /etc/flokoa/model.json.

    This represents the resolved configuration for an LLM model provider,
    created by the Flokoa operator when reconciling an Agent with a Model
    or ModelConfig reference.

    The JSON structure matches the operator's ModelProviderConfig:
    {
        "provider": "openai",
        "model": "gpt-4o",
        "config": {
            "baseURL": "https://api.openai.com/v1",
            "organizationID": "org-...",
            "timeoutSeconds": 60,
            "defaultHeaders": {...}
        },
        "settings": {
            "temperature": 0.7,
            "max_tokens": 4096,
            "frequency_penalty": 0.5
        }
    }

    The `settings` field uses PydanticAI's ModelSettings TypedDict, allowing
    direct usage with PydanticAI models:

        from pydantic_ai import Agent
        from flokoa.utils import load_model_config

        config = load_model_config()
        agent = Agent('openai:gpt-4o', model_settings=config.settings)

    Note: API keys and other secrets are injected as environment variables
    by the operator (e.g., OPENAI_API_KEY, ANTHROPIC_API_KEY) and are not
    included in this configuration file.
    """

    provider: ModelProvider = Field(
        ...,
        description="The LLM provider type.",
    )
    model: str = Field(
        ...,
        description="The model identifier (e.g., 'gpt-4o', 'claude-sonnet-4-20250514').",
    )
    config: dict[str, Any] | None = Field(
        default=None,
        description="Provider-specific connection configuration (baseURL, timeout, headers, etc.).",
    )
    settings: ModelSettings | None = Field(
        default=None,
        description="Model settings compatible with PydanticAI's ModelSettings.",
    )

    @property
    def base_url(self) -> str | None:
        """Get the base URL from config if present."""
        if self.config is None:
            return None
        return self.config.get("baseURL")

    @property
    def timeout_seconds(self) -> int | None:
        """Get the timeout in seconds from config if present."""
        if self.config is None:
            return None
        return self.config.get("timeoutSeconds")

    @property
    def default_headers(self) -> dict[str, str] | None:
        """Get the default headers from config if present."""
        if self.config is None:
            return None
        return self.config.get("defaultHeaders")

    def get_model_name(self) -> str:
        """Get the full model name in PydanticAI format (provider:model).

        Returns a model name string that can be used directly with PydanticAI Agent:

            from pydantic_ai import Agent
            config = load_model_config()
            agent = Agent(config.get_model_name(), model_settings=config.settings)
        """
        # Map our provider enum to PydanticAI model name prefixes
        provider_map = {
            ModelProvider.OPENAI: "openai",
            ModelProvider.ANTHROPIC: "anthropic",
            ModelProvider.AZURE_OPENAI: "azure",
            ModelProvider.OLLAMA: "ollama",
            ModelProvider.GEMINI: "gemini",
            ModelProvider.GEMINI_VERTEX: "vertexai",
            ModelProvider.ANTHROPIC_VERTEX: "vertexai",
            ModelProvider.BEDROCK: "bedrock",
        }
        prefix = provider_map.get(self.provider, self.provider.value)
        return f"{prefix}:{self.model}"
