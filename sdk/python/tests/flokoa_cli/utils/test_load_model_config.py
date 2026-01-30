import json

import pytest

import flokoa.utils as utils_module
from flokoa.types import ModelConfig, ModelProvider
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
        assert result.provider == ModelProvider.OPENAI
        assert result.model == "gpt-4o"
        assert result.config is None
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
        assert result.provider == ModelProvider.OPENAI
        assert result.model == "gpt-4o"
        assert result.config is not None
        assert result.config["baseURL"] == "https://api.openai.com/v1"
        assert result.config["organizationID"] == "org-12345"
        assert result.config["timeoutSeconds"] == 120

    def test_loads_anthropic_config(self, anthropic_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(anthropic_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result is not None
        assert result.provider == ModelProvider.ANTHROPIC
        assert result.model == "claude-sonnet-4-20250514"
        assert result.config["baseURL"] == "https://api.anthropic.com"
        assert result.config["timeoutSeconds"] == 90

    def test_loads_ollama_config(self, ollama_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(ollama_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result is not None
        assert result.provider == ModelProvider.OLLAMA
        assert result.model == "llama3.2"
        assert result.config["host"] == "http://localhost:11434"

    def test_loads_azure_openai_config(self, azure_openai_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(azure_openai_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result is not None
        assert result.provider == ModelProvider.AZURE_OPENAI
        assert result.model == "gpt-4o"
        assert result.config["endpoint"] == "https://myresource.openai.azure.com"
        assert result.config["deploymentName"] == "my-gpt4o-deployment"

    def test_loads_gemini_config(self, gemini_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(gemini_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result is not None
        assert result.provider == ModelProvider.GEMINI
        assert result.model == "gemini-1.5-pro"


class TestLoadModelConfigParameters:
    """Tests for loading model parameters."""

    def test_loads_common_parameters(self, openai_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(openai_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.parameters is not None
        assert result.parameters.temperature == "0.7"
        assert result.parameters.maxTokens == 4096
        assert result.parameters.topP == "0.9"

    def test_loads_openai_specific_parameters(self, openai_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(openai_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.parameters is not None
        assert result.parameters.openai is not None
        assert result.parameters.openai["frequencyPenalty"] == "0.5"
        assert result.parameters.openai["presencePenalty"] == "0.3"

    def test_loads_anthropic_thinking_parameters(
        self, anthropic_model_config_data, tmp_path, monkeypatch
    ):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(anthropic_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.parameters is not None
        assert result.parameters.anthropic is not None
        assert result.parameters.anthropic["thinking"]["type"] == "enabled"
        assert result.parameters.anthropic["thinking"]["budgetTokens"] == 2048

    def test_loads_stop_sequences_and_seed(
        self, model_config_with_only_parameters, tmp_path, monkeypatch
    ):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(model_config_with_only_parameters))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.parameters is not None
        assert result.parameters.stopSequences == ["END", "STOP"]
        assert result.parameters.seed == 42

    def test_loads_topk_parameter(self, gemini_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(gemini_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.parameters is not None
        assert result.parameters.topK == 40


class TestLoadModelConfigProperties:
    """Tests for ModelConfig convenience properties."""

    def test_base_url_property(self, openai_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(openai_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.base_url == "https://api.openai.com/v1"

    def test_base_url_property_when_no_config(
        self, minimal_model_config_data, tmp_path, monkeypatch
    ):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(minimal_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.base_url is None

    def test_timeout_seconds_property(self, openai_model_config_data, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(openai_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.timeout_seconds == 120

    def test_timeout_seconds_property_when_no_config(
        self, minimal_model_config_data, tmp_path, monkeypatch
    ):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(minimal_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.timeout_seconds is None

    def test_default_headers_property(
        self, model_config_with_default_headers, tmp_path, monkeypatch
    ):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(model_config_with_default_headers))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.default_headers is not None
        assert result.default_headers["X-Request-Source"] == "flokoa"
        assert result.default_headers["X-Tenant-ID"] == "tenant-123"

    def test_default_headers_property_when_no_config(
        self, minimal_model_config_data, tmp_path, monkeypatch
    ):
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(minimal_model_config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result.default_headers is None


class TestLoadModelConfigAllProviders:
    """Tests that all provider enum values are valid."""

    @pytest.mark.parametrize(
        "provider,model",
        [
            ("openai", "gpt-4o"),
            ("anthropic", "claude-sonnet-4-20250514"),
            ("azure-openai", "gpt-4o"),
            ("ollama", "llama3.2"),
            ("gemini", "gemini-1.5-pro"),
            ("gemini-vertex", "gemini-1.5-pro"),
            ("anthropic-vertex", "claude-sonnet-4-20250514"),
            ("bedrock", "anthropic.claude-3-sonnet"),
        ],
    )
    def test_loads_all_provider_types(self, provider, model, tmp_path, monkeypatch):
        config_data = {
            "provider": provider,
            "model": model,
        }
        model_config_path = tmp_path / "model.json"
        model_config_path.write_text(json.dumps(config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        result = load_model_config()

        assert result is not None
        assert result.provider.value == provider
        assert result.model == model


class TestLoadModelConfigValidation:
    """Tests for validation errors."""

    def test_raises_error_for_invalid_provider(self, tmp_path, monkeypatch):
        config_data = {
            "provider": "invalid-provider",
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
            "provider": "openai",
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
