"""Shared factory functions for creating pydantic-ai models, providers, and settings.

This module centralises the model/provider/settings construction logic
that is shared between the flokoa SDK executor and the managed-task runtime.
"""

from __future__ import annotations

from enum import Enum
from typing import Any

from pydantic import BaseModel
from pydantic_ai.models import Model
from pydantic_ai.providers import Provider, infer_provider_class
from pydantic_ai.settings import ModelSettings, merge_model_settings

from flokoa_types import ModelConfig, ModelParameters
from flokoa_types.modelconfig import ProviderType

from .models import PROVIDER_MODEL_MAP

# Common parameter → ModelSettings key mappings with optional type transforms.
_PARAM_MAPPINGS: list[tuple[str, str, type | None]] = [
    ("max_tokens", "max_tokens", None),
    ("temperature", "temperature", float),
    ("top_p", "top_p", float),
    ("seed", "seed", None),
    ("presence_penalty", "presence_penalty", float),
    ("frequency_penalty", "frequency_penalty", float),
    ("logit_bias", "logit_bias", None),
    ("stop_sequences", "stop_sequences", None),
    ("extra_headers", "extra_headers", None),
    ("extra_body", "extra_body", None),
    ("parallel_tool_calls", "parallel_tool_calls", None),
    ("time_out", "timeout", float),
]


def create_provider(model_config: ModelConfig) -> Provider:
    """Create a pydantic-ai Provider from ModelConfig.

    Args:
        model_config: The model configuration containing provider type and settings.

    Returns:
        A configured pydantic-ai Provider instance.
    """
    provider_type = model_config.provider.type
    provider_config = getattr(model_config.provider, provider_type.value, None)

    provider_cls = infer_provider_class(provider=provider_type.value)
    if provider_config:
        return provider_cls(**provider_config.model_dump())
    return provider_cls()


def create_model(model_config: ModelConfig, provider: Provider) -> Model:
    """Create a pydantic-ai Model from ModelConfig and a Provider.

    Args:
        model_config: The model configuration.
        provider: A pre-constructed pydantic-ai provider.

    Returns:
        A configured pydantic-ai Model instance.

    Raises:
        ValueError: If no model mapping exists for the given provider type.
    """
    provider_type = model_config.provider.type

    provider_entry = PROVIDER_MODEL_MAP.get(provider_type)
    if not provider_entry:
        raise ValueError(f"No model mapping found for provider '{provider_type.value}'")

    model_cls = provider_entry["model_class"]
    model_settings = build_model_settings(model_config)

    return model_cls(model_config.model, provider=provider, settings=model_settings)


def build_model_settings(model_config: ModelConfig) -> ModelSettings | None:
    """Build pydantic-ai ModelSettings from a ModelConfig.

    Merges common parameter settings with provider-specific settings.

    Args:
        model_config: The model configuration containing parameters.

    Returns:
        Merged ModelSettings, or None if no parameters are configured.
    """
    if not model_config.parameters:
        return None

    common = params_to_settings(model_config.parameters)
    provider_specific = provider_params_to_settings(model_config)

    return merge_model_settings(common, provider_specific)


def params_to_settings(params: ModelParameters) -> ModelSettings:
    """Convert common ModelParameters to a ModelSettings dict.

    Maps standard parameter fields (temperature, max_tokens, etc.) from
    the Flokoa ModelParameters type to pydantic-ai's ModelSettings.

    Args:
        params: The common model parameters.

    Returns:
        A ModelSettings dict with the mapped values.
    """
    settings: ModelSettings = {}
    for attr, key, transform in _PARAM_MAPPINGS:
        value = getattr(params, attr)
        if value is not None:
            settings[key] = transform(value) if transform else value  # type: ignore[literal-required]

    return settings


def provider_params_to_settings(model_config: ModelConfig) -> ModelSettings | None:
    """Convert provider-specific parameters to prefixed ModelSettings.

    Provider-specific parameters are stored under a sub-object keyed by
    the provider type (e.g. ``model_config.parameters.openai``). Each
    field is mapped to a prefixed ModelSettings key (e.g. ``openai_reasoning_effort``).

    Args:
        model_config: The model configuration containing provider-specific parameters.

    Returns:
        A ModelSettings dict with prefixed keys, or None if no provider parameters exist.
    """
    if not model_config.parameters:
        return None

    provider_type = model_config.provider.type
    provider_params = getattr(model_config.parameters, provider_type.value, None)
    if not provider_params:
        return None

    prefix = provider_type.value + "_"
    settings: ModelSettings = {}

    for field_name in provider_params.__pydantic_fields__:
        value = getattr(provider_params, field_name)
        if value is not None:
            if isinstance(value, Enum):
                value = value.value
            elif isinstance(value, BaseModel):
                value = value.model_dump(exclude_none=True)
            settings[prefix + field_name] = value  # type: ignore[literal-required]

    return settings if settings else None


def create_model_from_config(model_config: ModelConfig) -> Model:
    """Convenience: create both provider and model from a ModelConfig in one call.

    Args:
        model_config: The full model configuration.

    Returns:
        A configured pydantic-ai Model instance.
    """
    provider = create_provider(model_config)
    return create_model(model_config, provider)
