from flokoa_types.modelconfig import ProviderType
from pydantic_ai.models import Model
from pydantic_ai.models.anthropic import AnthropicModel, AnthropicModelSettings
from pydantic_ai.models.bedrock import BedrockConverseModel, BedrockModelSettings
from pydantic_ai.models.google import GoogleModel, GoogleModelSettings
from pydantic_ai.models.openai import OpenAIResponsesModel, OpenAIResponsesModelSettings
from pydantic_ai.providers import Provider
from pydantic_ai.providers.anthropic import AnthropicProvider
from pydantic_ai.providers.bedrock import BedrockProvider
from pydantic_ai.providers.google import GoogleProvider
from pydantic_ai.providers.openai import OpenAIProvider
from pydantic_ai.settings import ModelSettings
from typing_extensions import TypedDict


class ProviderModelMapEntry(TypedDict):
    model_class: type[Model]
    settings_class: type[ModelSettings]
    provider_class: type[Provider]


PROVIDER_MODEL_MAP: dict[ProviderType, ProviderModelMapEntry] = {
    ProviderType.anthropic: {
        "model_class": AnthropicModel,
        "settings_class": AnthropicModelSettings,
        "provider_class": AnthropicProvider,
    },
    ProviderType.openai: {
        "model_class": OpenAIResponsesModel,
        "settings_class": OpenAIResponsesModelSettings,
        "provider_class": OpenAIProvider,
    },
    ProviderType.google: {
        "model_class": GoogleModel,
        "settings_class": GoogleModelSettings,
        "provider_class": GoogleProvider,
    },
    ProviderType.bedrock: {
        "model_class": BedrockConverseModel,
        "settings_class": BedrockModelSettings,
        "provider_class": BedrockProvider,
    },
}
