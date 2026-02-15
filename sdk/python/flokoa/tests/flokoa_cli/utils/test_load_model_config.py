"""Tests for load_model_config utility function.

These tests verify the model config loading functionality with the CRD-based
ModelConfig schema (provider with type field, parameters structure, etc.).
"""

import json

import pytest

import flokoa.utils as utils_module
from flokoa_types import ModelConfig
from flokoa_types.modelconfig import ProviderType
from flokoa.utils import load_model_config


class TestLoadModelConfigFileNotExists:
    """Tests for when the model config file does not exist."""

    def test_returns_none_when_file_not_exists(self, tmp_path, monkeypatch):
        nonexistent_path = str(tmp_path / "nonexistent" / "model.json")
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", nonexistent_path)

        result = load_model_config()

        assert result is None

    def test_returns_none_for_empty_directory(self, tmp_path, monkeypatch):
        empty_dir = tmp_path / "empty"
        empty_dir.mkdir()
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(empty_dir / "model.json"))

        result = load_model_config()

        assert result is None


class TestLoadModelConfigBasic:
    """Tests for basic model config loading functionality."""

    def test_loads_minimal_model_config(self, minimal_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(minimal_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result is not None
        assert isinstance(result, ModelConfig)
        assert result.provider.type == ProviderType.openai
        assert result.model == "gpt-4o"
        assert result.parameters is None

    def test_returns_model_config_type(self, minimal_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(minimal_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert isinstance(result, ModelConfig)


class TestLoadModelConfigProviders:
    """Tests for loading configs with different providers."""

    def test_loads_openai_config(self, openai_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(openai_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result is not None
        assert result.provider.type == ProviderType.openai
        assert result.model == "gpt-4o"
        assert result.provider.openai is not None
        assert result.provider.openai.base_url == "https://api.openai.com/v1"

    def test_loads_anthropic_config(self, anthropic_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(anthropic_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result is not None
        assert result.provider.type == ProviderType.anthropic
        assert result.model == "claude-sonnet-4-20250514"
        assert result.provider.anthropic is not None
        assert result.provider.anthropic.base_url == "https://api.anthropic.com"

    def test_loads_google_config(self, google_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(google_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result is not None
        assert result.provider.type == ProviderType.google
        assert result.model == "gemini-1.5-pro"
        assert result.provider.google is not None
        assert result.provider.google.location == "us-central1"

    def test_loads_bedrock_config(self, bedrock_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(bedrock_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result is not None
        assert result.provider.type == ProviderType.bedrock
        assert result.model == "anthropic.claude-3-sonnet"
        assert result.provider.bedrock is not None
        assert result.provider.bedrock.region == "us-east-1"


class TestLoadModelConfigParameters:
    """Tests for loading model parameters."""

    def test_loads_common_parameters(self, openai_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(openai_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.parameters is not None
        assert result.parameters.temperature == "0.7"
        assert result.parameters.max_tokens == 4096
        assert result.parameters.top_p == "0.9"

    def test_loads_penalty_parameters(self, openai_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(openai_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.parameters is not None
        assert result.parameters.frequency_penalty == "0.5"
        assert result.parameters.presence_penalty == "0.3"

    def test_loads_stop_sequences_and_seed(self, model_config_with_settings, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(model_config_with_settings))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.parameters is not None
        assert result.parameters.stop_sequences == ["END", "STOP"]
        assert result.parameters.seed == 42


class TestLoadModelConfigProviderSpecificParameters:
    """Tests for provider-specific parameters."""

    def test_loads_openai_specific_parameters(self, openai_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(openai_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.parameters is not None
        assert result.parameters.openai is not None
        assert result.parameters.openai.service_tier.value == "default"

    def test_loads_anthropic_cache_settings(self, anthropic_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(anthropic_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.parameters is not None
        assert result.parameters.anthropic is not None
        assert result.parameters.anthropic.cache_instructions == "5m"


class TestLoadModelConfigDefaultHeaders:
    """Tests for default headers configuration."""

    def test_loads_default_headers(self, model_config_with_default_headers, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(model_config_with_default_headers))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.provider.default_headers is not None
        assert result.provider.default_headers["X-Request-Source"] == "flokoa"
        assert result.provider.default_headers["X-Tenant-ID"] == "tenant-123"

    def test_default_headers_is_none_when_not_specified(
        self, minimal_model_config_data, tmp_path, monkeypatch
    ):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(minimal_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.provider.default_headers is None


class TestLoadModelConfigAllProviders:
    """Tests that all provider enum values are valid."""

    @pytest.mark.parametrize(
        "provider_type,model",
        [
            ("openai", "gpt-4o"),
            ("anthropic", "claude-sonnet-4-20250514"),
            ("google", "gemini-1.5-pro"),
            ("bedrock", "anthropic.claude-3-sonnet"),
        ],
    )
    def test_loads_all_provider_types(self, provider_type, model, tmp_path, monkeypatch):
        config_data = {
            "provider": {"type": provider_type},
            "model": model,
        }
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result is not None
        assert result.provider.type.value == provider_type
        assert result.model == model


class TestLoadModelConfigValidation:
    """Tests for validation errors."""

    def test_raises_error_for_invalid_provider(self, tmp_path, monkeypatch):
        config_data = {
            "provider": {"type": "invalid-provider"},
            "model": "some-model",
        }
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        with pytest.raises(ValueError):
            load_model_config()

    def test_raises_error_for_missing_provider(self, tmp_path, monkeypatch):
        config_data = {
            "model": "gpt-4o",
        }
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        with pytest.raises(ValueError):
            load_model_config()

    def test_raises_error_for_missing_model(self, tmp_path, monkeypatch):
        config_data = {
            "provider": {"type": "openai"},
        }
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        with pytest.raises(ValueError):
            load_model_config()

    def test_raises_error_for_invalid_json(self, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text("not valid json {")
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        with pytest.raises(json.JSONDecodeError):
            load_model_config()
