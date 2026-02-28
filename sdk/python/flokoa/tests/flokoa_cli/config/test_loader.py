"""Tests for flokoa.config.loader — config file loading."""

import json

import pytest

from flokoa.config.agent_config import LlmAgentConfig
from flokoa.config.loader import (
    load_agent_config,
    load_agent_config_from_dict,
    load_legacy_llm_config,
)


class TestLoadAgentConfig:
    def test_loads_from_json_file(self, tmp_path):
        config_file = tmp_path / "agent-config.json"
        config_file.write_text(json.dumps({
            "name": "test_agent",
            "instruction": "Be helpful.",
            "model": {"provider": {"type": "openai"}, "model": "gpt-4o"},
        }))
        config = load_agent_config(str(config_file))
        assert isinstance(config.root, LlmAgentConfig)
        assert config.root.name == "test_agent"

    def test_file_not_found(self, tmp_path):
        with pytest.raises(FileNotFoundError, match="Agent config file not found"):
            load_agent_config(str(tmp_path / "missing.json"))

    def test_uses_env_var(self, tmp_path, monkeypatch):
        config_file = tmp_path / "custom.json"
        config_file.write_text(json.dumps({"name": "env_agent"}))
        monkeypatch.setenv("FLOKOA_AGENT_CONFIG_PATH", str(config_file))
        config = load_agent_config()
        assert config.root.name == "env_agent"


class TestLoadAgentConfigFromDict:
    def test_loads_llm_config(self):
        config = load_agent_config_from_dict({
            "name": "test",
            "instruction": "Hello",
        })
        assert isinstance(config.root, LlmAgentConfig)

class TestLoadLegacyLlmConfig:
    def test_loads_from_files(self, tmp_path, monkeypatch):
        # Template config
        template_path = tmp_path / "template-config.json"
        template_path.write_text(json.dumps({
            "outputSchema": {
                "name": "Answer",
                "description": "An answer",
                "jsonSchema": {"type": "object"},
            },
        }))
        monkeypatch.setenv("FLOKOA_TEMPLATE_CONFIG_PATH", str(template_path))

        # Instruction
        instruction_path = tmp_path / "instruction.txt"
        instruction_path.write_text("Be helpful.")
        monkeypatch.setenv("FLOKOA_INSTRUCTION_PATH", str(instruction_path))

        # Model config
        model_path = tmp_path / "model.json"
        model_path.write_text(json.dumps({
            "provider": {"type": "openai"},
            "model": "gpt-4o",
        }))

        config = load_legacy_llm_config(
            template_config_path=str(template_path),
            instruction_path=str(instruction_path),
            model_config_path=str(model_path),
        )
        inner = config.root
        assert isinstance(inner, LlmAgentConfig)
        assert inner.instruction == "Be helpful."
        assert inner.model is not None
        assert inner.model.model == "gpt-4o"
        assert inner.output_schema is not None
        assert inner.output_schema.name == "Answer"

    def test_handles_missing_files_gracefully(self, tmp_path):
        config = load_legacy_llm_config(
            template_config_path=str(tmp_path / "missing.json"),
            instruction_path=str(tmp_path / "missing.txt"),
            model_config_path=str(tmp_path / "missing.json"),
        )
        inner = config.root
        assert isinstance(inner, LlmAgentConfig)
        assert inner.instruction is None
        assert inner.model is None
        assert inner.output_schema is None


