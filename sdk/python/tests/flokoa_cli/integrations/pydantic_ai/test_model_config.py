"""Tests for model configuration features in PydanticAI integration.

This test module covers:
1. Loading model config from local files (following tool loading pattern)
2. Base class model config integration (FlokoaAgentExecutor)
3. PydanticAI model config integration (model settings mapping)
4. End-to-end tests: file -> executor -> agent -> verify settings
"""

import json
from unittest.mock import AsyncMock, MagicMock

import pytest
from pydantic_ai import Agent, models
from pydantic_ai.messages import ModelMessage, ModelResponse, TextPart
from pydantic_ai.models.function import AgentInfo, FunctionModel
from pydantic_ai.models.test import TestModel

import flokoa.utils as utils_module
from flokoa.integrations.pydantic_ai.agent_executor import PydanticAIAgentExecutor
from flokoa.integrations.pydantic_ai.models import PROVIDER_MODEL_MAP
from flokoa.types import ModelConfig
from flokoa.types.modelconfig import ProviderType
from flokoa.utils import load_model_config

# Block real model requests during testing
models.ALLOW_MODEL_REQUESTS = False

pytestmark = pytest.mark.anyio


# =============================================================================
# Test: Loading Model Config from Local Files
# =============================================================================


class TestLoadModelConfigFromFile:
    """Tests for loading model config from local JSON files.

    These tests follow the same pattern as tool loading tests,
    verifying the complete flow from file to ModelConfig object.
    """

    def test_loads_openai_config_from_file(self, openai_model_config_json, tmp_path, monkeypatch):
        """Verify OpenAI config loads correctly from JSON file."""
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(openai_model_config_json))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result is not None
        assert isinstance(result, ModelConfig)
        assert result.provider.type == ProviderType.openai
        assert result.model == "gpt-4o"
        assert result.provider.openai is not None
        assert result.provider.openai.base_url == "https://api.openai.com/v1"

    def test_loads_anthropic_config_from_file(self, anthropic_model_config_json, tmp_path, monkeypatch):
        """Verify Anthropic config loads correctly from JSON file."""
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(anthropic_model_config_json))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result is not None
        assert result.provider.type == ProviderType.anthropic
        assert result.model == "claude-sonnet-4-20250514"
        assert result.provider.anthropic is not None
        assert result.provider.anthropic.base_url == "https://api.anthropic.com"

    def test_loads_google_config_from_file(self, google_model_config_json, tmp_path, monkeypatch):
        """Verify Google/Gemini config loads correctly from JSON file."""
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(google_model_config_json))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result is not None
        assert result.provider.type == ProviderType.google
        assert result.model == "gemini-1.5-pro"
        assert result.provider.google is not None
        assert result.provider.google.location == "us-central1"
        assert result.provider.google.project == "test-project"

    def test_loads_bedrock_config_from_file(self, bedrock_model_config_json, tmp_path, monkeypatch):
        """Verify AWS Bedrock config loads correctly from JSON file."""
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(bedrock_model_config_json))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result is not None
        assert result.provider.type == ProviderType.bedrock
        assert result.model == "anthropic.claude-3-sonnet-20240229-v1:0"
        assert result.provider.bedrock is not None
        assert result.provider.bedrock.region == "us-east-1"

    def test_loads_minimal_config_from_file(self, minimal_model_config_json, tmp_path, monkeypatch):
        """Verify minimal config with only required fields loads correctly."""
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(minimal_model_config_json))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result is not None
        assert result.provider.type == ProviderType.openai
        assert result.model == "gpt-4o-mini"
        assert result.parameters is None

    def test_returns_none_when_file_not_exists(self, tmp_path, monkeypatch):
        """Verify None is returned when model config file doesn't exist."""
        nonexistent_path = str(tmp_path / "nonexistent" / "model.json")
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", nonexistent_path)

        result = load_model_config()

        assert result is None


class TestLoadModelConfigParameters:
    """Tests for loading model parameters from config files."""

    def test_loads_common_parameters(self, openai_model_config_json, tmp_path, monkeypatch):
        """Verify common model parameters are loaded correctly."""
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(openai_model_config_json))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.parameters is not None
        assert result.parameters.temperature == "0.7"
        assert result.parameters.max_tokens == 4096
        assert result.parameters.top_p == "0.9"
        assert result.parameters.frequency_penalty == "0.5"
        assert result.parameters.presence_penalty == "0.3"
        assert result.parameters.seed == 42

    def test_loads_provider_specific_parameters(self, openai_model_config_json, tmp_path, monkeypatch):
        """Verify provider-specific parameters are loaded correctly."""
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(openai_model_config_json))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.parameters is not None
        assert result.parameters.openai is not None
        assert result.parameters.openai.reasoning_effort.value == "medium"
        assert result.parameters.openai.service_tier.value == "default"

    def test_loads_anthropic_thinking_config(self, model_config_with_thinking_json, tmp_path, monkeypatch):
        """Verify Anthropic extended thinking configuration loads correctly."""
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(model_config_with_thinking_json))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.parameters is not None
        assert result.parameters.anthropic is not None
        assert result.parameters.anthropic.thinking is not None
        assert result.parameters.anthropic.thinking.type.value == "enabled"
        assert result.parameters.anthropic.thinking.budget_tokens == 8192

    def test_loads_default_headers(self, model_config_with_default_headers_json, tmp_path, monkeypatch):
        """Verify default headers are loaded correctly."""
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(model_config_with_default_headers_json))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.provider.default_headers is not None
        assert result.provider.default_headers["X-Request-Source"] == "flokoa-test"
        assert result.provider.default_headers["X-Tenant-ID"] == "tenant-test-123"


# =============================================================================
# Test: Provider Model Map
# =============================================================================


class TestProviderModelMap:
    """Tests for PROVIDER_MODEL_MAP mappings."""

    def test_all_providers_have_mappings(self):
        """Verify all provider types have entries in PROVIDER_MODEL_MAP."""
        for provider_type in [ProviderType.openai, ProviderType.anthropic, ProviderType.google, ProviderType.bedrock]:
            assert provider_type in PROVIDER_MODEL_MAP
            entry = PROVIDER_MODEL_MAP[provider_type]
            assert "model_class" in entry
            assert "settings_class" in entry
            assert "provider_class" in entry

    def test_openai_mapping_is_correct(self):
        """Verify OpenAI provider maps to correct pydantic-ai classes."""
        from pydantic_ai.models.openai import OpenAIResponsesModel, OpenAIResponsesModelSettings
        from pydantic_ai.providers.openai import OpenAIProvider

        entry = PROVIDER_MODEL_MAP[ProviderType.openai]
        assert entry["model_class"] == OpenAIResponsesModel
        assert entry["settings_class"] == OpenAIResponsesModelSettings
        assert entry["provider_class"] == OpenAIProvider

    def test_anthropic_mapping_is_correct(self):
        """Verify Anthropic provider maps to correct pydantic-ai classes."""
        from pydantic_ai.models.anthropic import AnthropicModel, AnthropicModelSettings
        from pydantic_ai.providers.anthropic import AnthropicProvider

        entry = PROVIDER_MODEL_MAP[ProviderType.anthropic]
        assert entry["model_class"] == AnthropicModel
        assert entry["settings_class"] == AnthropicModelSettings
        assert entry["provider_class"] == AnthropicProvider

    def test_google_mapping_is_correct(self):
        """Verify Google provider maps to correct pydantic-ai classes."""
        from pydantic_ai.models.google import GoogleModel, GoogleModelSettings
        from pydantic_ai.providers.google import GoogleProvider

        entry = PROVIDER_MODEL_MAP[ProviderType.google]
        assert entry["model_class"] == GoogleModel
        assert entry["settings_class"] == GoogleModelSettings
        assert entry["provider_class"] == GoogleProvider

    def test_bedrock_mapping_is_correct(self):
        """Verify Bedrock provider maps to correct pydantic-ai classes."""
        from pydantic_ai.models.bedrock import BedrockConverseModel, BedrockModelSettings
        from pydantic_ai.providers.bedrock import BedrockProvider

        entry = PROVIDER_MODEL_MAP[ProviderType.bedrock]
        assert entry["model_class"] == BedrockConverseModel
        assert entry["settings_class"] == BedrockModelSettings
        assert entry["provider_class"] == BedrockProvider


# =============================================================================
# Test: Base Class Model Config Integration
# =============================================================================


class TestFlokoaAgentExecutorModelConfig:
    """Tests for model config integration in FlokoaAgentExecutor base class."""

    @pytest.fixture
    def pydantic_agent(self):
        """Create a PydanticAI agent for testing."""
        return Agent("test", system_prompt="You are a helpful assistant.")

    @pytest.fixture
    def executor_with_model_config(
        self, pydantic_agent, openai_model_config_json, tmp_path, monkeypatch, mock_http_api
    ):
        """Create executor with model config file set up."""
        # Set up model config file
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(openai_model_config_json))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        # Patch load_tools to return empty list
        monkeypatch.setattr("flokoa.agent_executor.load_tools", lambda use_cache=True, cache=None: [])

        return PydanticAIAgentExecutor(pydantic_agent)

    def test_model_config_property_returns_loaded_config(self, executor_with_model_config):
        """Verify model_config property returns loaded configuration."""
        config = executor_with_model_config.model_config

        assert config is not None
        assert isinstance(config, ModelConfig)
        assert config.provider.type == ProviderType.openai
        assert config.model == "gpt-4o"

    def test_model_config_is_cached(self, executor_with_model_config):
        """Verify model config is cached between accesses."""
        config1 = executor_with_model_config.model_config
        config2 = executor_with_model_config.model_config

        # Same object should be returned (cached)
        assert config1 is config2

    def test_model_config_is_none_when_file_missing(self, pydantic_agent, tmp_path, monkeypatch, mock_http_api):
        """Verify model_config returns None when file doesn't exist."""
        nonexistent_path = str(tmp_path / "nonexistent" / "model.json")
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", nonexistent_path)
        monkeypatch.setattr("flokoa.agent_executor.load_tools", lambda use_cache=True, cache=None: [])

        executor = PydanticAIAgentExecutor(pydantic_agent)
        config = executor.model_config

        assert config is None

    def test_is_model_config_changed_detects_changes(self, executor_with_model_config, tmp_path, monkeypatch):
        """Verify is_model_config_changed detects file changes."""
        # Access model_config to populate cache
        _ = executor_with_model_config.model_config

        # Invalidate cache
        executor_with_model_config.invalidate_caches()

        # Should report changed after invalidation
        assert executor_with_model_config.is_model_config_changed() is True

    def test_invalidate_caches_clears_model_config(self, executor_with_model_config):
        """Verify invalidate_caches clears model config cache."""
        # Access model_config to populate cache
        config1 = executor_with_model_config.model_config

        # Invalidate caches
        executor_with_model_config.invalidate_caches()

        # Next access should reload
        config2 = executor_with_model_config.model_config

        # Should be equal but potentially different objects after reload
        assert config2 is not None
        assert config2.model == config1.model


# =============================================================================
# Test: PydanticAI Agent Executor Model Config Integration
# =============================================================================


class TestPydanticAIAgentExecutorModelConfig:
    """Tests for model config in PydanticAIAgentExecutor."""

    @pytest.fixture
    def pydantic_agent(self):
        """Create a PydanticAI agent for testing."""
        return Agent("test", system_prompt="You are a helpful assistant.")

    @pytest.fixture
    def executor_with_full_config(
        self, pydantic_agent, openai_model_config_json, multiple_tool_definitions, tmp_path, monkeypatch, mock_http_api
    ):
        """Create executor with both model config and tools set up."""
        # Set up model config file
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(openai_model_config_json))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        # Patch load_tools
        def patched_load_tools(use_cache=True, cache=None):
            return multiple_tool_definitions

        monkeypatch.setattr("flokoa.agent_executor.load_tools", patched_load_tools)

        return PydanticAIAgentExecutor(pydantic_agent)

    def test_get_model_config_returns_config(self, executor_with_full_config):
        """Verify model_config property returns the loaded configuration."""
        config = executor_with_full_config.model_config

        assert config is not None
        assert config.provider.type == ProviderType.openai
        assert config.model == "gpt-4o"

    def test_executor_has_both_tools_and_model_config(self, executor_with_full_config):
        """Verify executor can access both tools and model config."""
        tools = executor_with_full_config.tool_definitions
        config = executor_with_full_config.model_config

        assert len(tools) == 2
        assert config is not None
        assert config.model == "gpt-4o"


# =============================================================================
# Test: End-to-End Model Config Verification with FunctionModel
# =============================================================================


class TestEndToEndModelConfigVerification:
    """End-to-end tests verifying model config is available during agent execution.

    These tests use FunctionModel to capture and verify that model configuration
    is accessible throughout the execution flow.
    """

    @pytest.fixture
    def pydantic_agent(self):
        """Create a PydanticAI agent for testing."""
        agent = Agent("test", system_prompt="You are a helpful assistant.")

        @agent.tool_plain
        def echo_message(message: str) -> str:
            """Echo the input message."""
            return f"Echo: {message}"

        return agent

    @pytest.fixture
    def executor_with_config(
        self, pydantic_agent, openai_model_config_json, tmp_path, monkeypatch, mock_http_api
    ):
        """Create executor with model config for end-to-end testing."""
        # Set up model config file
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(openai_model_config_json))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        # Patch load_tools to return empty list
        monkeypatch.setattr("flokoa.agent_executor.load_tools", lambda use_cache=True, cache=None: [])

        return PydanticAIAgentExecutor(pydantic_agent)

    async def test_model_config_accessible_during_execution(
        self, pydantic_agent, executor_with_config
    ):
        """Verify model config is accessible during agent execution."""
        captured_info = None

        def model_function(messages: list[ModelMessage], info: AgentInfo) -> ModelResponse:
            nonlocal captured_info
            captured_info = info
            return ModelResponse(parts=[TextPart("Response")])

        # Verify model config is loaded before execution
        config = executor_with_config.model_config
        assert config is not None
        assert config.model == "gpt-4o"
        assert config.parameters is not None
        assert config.parameters.temperature == "0.7"

        # Run agent with FunctionModel
        with pydantic_agent.override(model=FunctionModel(model_function)):
            toolset = executor_with_config._get_toolset()
            result = await pydantic_agent.run("Test message", toolsets=[toolset])

        assert result.output == "Response"
        assert captured_info is not None

    async def test_execute_with_model_config_available(
        self, pydantic_agent, executor_with_config, mock_http_api
    ):
        """Verify execute method works with model config loaded."""
        test_model = TestModel(custom_output_text="Test response")

        mock_context = MagicMock()
        mock_context.get_user_input.return_value = "Hello"

        mock_event_queue = AsyncMock()
        mock_event_queue.enqueue_event = AsyncMock()

        # Verify config is available
        config = executor_with_config.model_config
        assert config is not None

        with pydantic_agent.override(model=test_model):
            await executor_with_config.execute(mock_context, mock_event_queue)

        mock_event_queue.enqueue_event.assert_called_once()


class TestModelConfigWithDifferentProviders:
    """Test model config loading with different provider configurations."""

    @pytest.fixture
    def pydantic_agent(self):
        """Create a PydanticAI agent for testing."""
        return Agent("test", system_prompt="Test agent")

    def test_anthropic_config_parameters_accessible(
        self, pydantic_agent, anthropic_model_config_json, tmp_path, monkeypatch, mock_http_api
    ):
        """Verify Anthropic-specific parameters are accessible."""
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(anthropic_model_config_json))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))
        monkeypatch.setattr("flokoa.agent_executor.load_tools", lambda use_cache=True, cache=None: [])

        executor = PydanticAIAgentExecutor(pydantic_agent)
        config = executor.model_config

        assert config is not None
        assert config.provider.type == ProviderType.anthropic
        assert config.parameters is not None
        assert config.parameters.anthropic is not None
        assert config.parameters.anthropic.cache_instructions == "5m"
        assert config.parameters.anthropic.cache_messages == "5m"

    def test_google_config_parameters_accessible(
        self, pydantic_agent, google_model_config_json, tmp_path, monkeypatch, mock_http_api
    ):
        """Verify Google-specific parameters are accessible."""
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(google_model_config_json))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))
        monkeypatch.setattr("flokoa.agent_executor.load_tools", lambda use_cache=True, cache=None: [])

        executor = PydanticAIAgentExecutor(pydantic_agent)
        config = executor.model_config

        assert config is not None
        assert config.provider.type == ProviderType.google
        assert config.provider.google is not None
        assert config.provider.google.location == "us-central1"
        assert config.provider.google.project == "test-project"

    def test_bedrock_config_parameters_accessible(
        self, pydantic_agent, bedrock_model_config_json, tmp_path, monkeypatch, mock_http_api
    ):
        """Verify Bedrock-specific parameters are accessible."""
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(bedrock_model_config_json))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))
        monkeypatch.setattr("flokoa.agent_executor.load_tools", lambda use_cache=True, cache=None: [])

        executor = PydanticAIAgentExecutor(pydantic_agent)
        config = executor.model_config

        assert config is not None
        assert config.provider.type == ProviderType.bedrock
        assert config.provider.bedrock is not None
        assert config.provider.bedrock.region == "us-east-1"
        assert config.parameters is not None
        assert config.parameters.bedrock is not None
        assert config.parameters.bedrock.cache_instructions is True


class TestModelConfigCaching:
    """Tests for model config caching behavior."""

    @pytest.fixture
    def pydantic_agent(self):
        """Create a PydanticAI agent for testing."""
        return Agent("test", system_prompt="Test agent")

    def test_model_config_uses_cache_by_default(
        self, pydantic_agent, openai_model_config_json, tmp_path, monkeypatch, mock_http_api
    ):
        """Verify model config is cached and reused."""
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(openai_model_config_json))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))
        monkeypatch.setattr("flokoa.agent_executor.load_tools", lambda use_cache=True, cache=None: [])

        executor = PydanticAIAgentExecutor(pydantic_agent)

        # Access config multiple times
        config1 = executor.model_config
        config2 = executor.model_config
        config3 = executor.model_config

        # All should be the same cached object
        assert config1 is config2
        assert config2 is config3

    def test_model_config_cache_invalidation(
        self, pydantic_agent, openai_model_config_json, tmp_path, monkeypatch, mock_http_api
    ):
        """Verify cache invalidation forces reload."""
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(openai_model_config_json))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))
        monkeypatch.setattr("flokoa.agent_executor.load_tools", lambda use_cache=True, cache=None: [])

        executor = PydanticAIAgentExecutor(pydantic_agent)

        config1 = executor.model_config
        executor.invalidate_caches()
        config2 = executor.model_config

        # After invalidation, config should be reloaded (may be different object)
        assert config1 is not config2
        # But content should be the same
        assert config1.model == config2.model
        assert config1.provider.type == config2.provider.type


class TestModelConfigValidation:
    """Tests for model config validation errors."""

    def test_raises_error_for_invalid_provider(self, tmp_path, monkeypatch):
        """Verify validation error for invalid provider type."""
        config_data = {
            "provider": {"type": "invalid-provider"},
            "model": "some-model",
        }
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        with pytest.raises(ValueError):
            load_model_config()

    def test_raises_error_for_missing_model(self, tmp_path, monkeypatch):
        """Verify validation error for missing model field."""
        config_data = {
            "provider": {"type": "openai"},
        }
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        with pytest.raises(ValueError):
            load_model_config()

    def test_raises_error_for_invalid_json(self, tmp_path, monkeypatch):
        """Verify JSON decode error for invalid JSON file."""
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text("not valid json {")
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        with pytest.raises(json.JSONDecodeError):
            load_model_config()
