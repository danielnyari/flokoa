"""Model configuration types matching the Kubernetes operator's ModelProviderConfig.

These types represent the model configuration that is mounted at /etc/flokoa/model.json
by the Flokoa operator when an Agent references a Model or ModelConfig resource.
"""

from __future__ import annotations

from enum import Enum
from typing import Any

from pydantic import BaseModel, Field


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


class ModelParameters(BaseModel):
    """Common model parameters from ModelConfig.

    These are provider-agnostic parameters that can be applied to most LLM providers.
    """

    temperature: str | None = Field(
        default=None,
        description="Temperature controls randomness (0.0 to 2.0). Specified as string to avoid floating point issues.",
    )
    maxTokens: int | None = Field(
        default=None,
        description="Maximum number of tokens to generate.",
        alias="maxTokens",
    )
    topP: str | None = Field(
        default=None,
        description="Top-p (nucleus) sampling parameter (0.0 to 1.0). Specified as string.",
        alias="topP",
    )
    topK: int | None = Field(
        default=None,
        description="Limits the number of tokens to consider for each step.",
        alias="topK",
    )
    stopSequences: list[str] | None = Field(
        default=None,
        description="Sequences where the model will stop generating.",
        alias="stopSequences",
    )
    seed: int | None = Field(
        default=None,
        description="Seed for deterministic generation (where supported).",
    )

    # Provider-specific parameters
    openai: dict[str, Any] | None = Field(
        default=None,
        description="OpenAI-specific parameters (frequencyPenalty, presencePenalty, reasoningEffort, etc.)",
    )
    anthropic: dict[str, Any] | None = Field(
        default=None,
        description="Anthropic-specific parameters (thinking configuration).",
    )
    gemini: dict[str, Any] | None = Field(
        default=None,
        description="Gemini-specific parameters (candidateCount, safetySettings, etc.)",
    )


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
        "parameters": {
            "temperature": "0.7",
            "maxTokens": 4096,
            "openai": {
                "frequencyPenalty": "0.5"
            }
        }
    }

    Note: Secrets like API keys are injected as environment variables,
    not included in this configuration file.
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
        description="Provider-specific non-sensitive configuration (baseURL, timeout, headers, etc.).",
    )
    parameters: ModelParameters | None = Field(
        default=None,
        description="Model parameters from ModelConfig (temperature, maxTokens, etc.).",
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
