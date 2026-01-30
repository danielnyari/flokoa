"""Model configuration types for PydanticAI integration.

These types represent the model configuration that is mounted at /etc/flokoa/model.json
by the Flokoa operator when an Agent references a Model or ModelConfig resource.

The configuration maps to PydanticAI's provider/model architecture:
- provider: identifies which PydanticAI provider to use (OpenAIProvider, AnthropicProvider, etc.)
- config: provider-specific connection kwargs (base_url, timeout, etc.)
- model: the model name to pass to the model class
- settings: model settings compatible with pydantic_ai.settings.ModelSettings
"""

from __future__ import annotations

from enum import Enum
from typing import Any

from pydantic import BaseModel, Field


class ModelProvider(str, Enum):
    """Model provider enum matching the Kubernetes CRD ModelProvider enum.

    These map to PydanticAI providers:
    - OPENAI -> pydantic_ai.providers.openai.OpenAIProvider
    - ANTHROPIC -> pydantic_ai.providers.anthropic.AnthropicProvider
    - AZURE_OPENAI -> pydantic_ai.providers.azure.AzureOpenAIProvider
    - OLLAMA -> pydantic_ai.providers.ollama.OllamaProvider
    - GEMINI -> pydantic_ai.providers.google.GoogleProvider
    - GEMINI_VERTEX -> pydantic_ai.providers.google_vertex.GoogleVertexProvider
    - ANTHROPIC_VERTEX -> pydantic_ai.providers.anthropic (with Vertex config)
    - BEDROCK -> pydantic_ai.providers.bedrock.BedrockProvider
    """

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

    This maps to PydanticAI's provider/model architecture:

    1. Using with PydanticAI Agent (simple):

        from pydantic_ai import Agent
        from flokoa.utils import load_model_config

        config = load_model_config()
        agent = Agent(config.get_model_name(), model_settings=config.settings)

    2. Using with PydanticAI Provider/Model (full control):

        from pydantic_ai.providers.openai import OpenAIProvider
        from pydantic_ai.models.openai import OpenAIModel

        config = load_model_config()

        # Provider handles connection config (base_url, api_key from env, etc.)
        provider = OpenAIProvider(base_url=config.base_url)

        # Model uses the provider and settings
        model = OpenAIModel(
            model_name=config.model,
            provider=provider,
            settings=config.settings  # Compatible with pydantic_ai.settings.ModelSettings
        )
        agent = Agent(model)

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

    Note: API keys are injected as environment variables by the operator
    (e.g., OPENAI_API_KEY, ANTHROPIC_API_KEY) and read automatically by
    PydanticAI providers.
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
    settings: dict[str, Any] | None = Field(
        default=None,
        description="Model settings compatible with pydantic_ai.settings.ModelSettings.",
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
        """Get the model name in PydanticAI format (provider:model).

        Returns a string like 'openai:gpt-4o' that can be used directly with
        pydantic_ai.Agent or to construct model instances.
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
