import json

import flokoa.utils as utils_module
from flokoa.utils import load_agent_card


class TestLoadAgentCardFileNotExists:
    """Tests for when the agent card file does not exist."""

    def test_returns_none_when_file_not_exists(self, tmp_path, monkeypatch):
        nonexistent_path = str(tmp_path / "nonexistent" / "agent-card.json")
        monkeypatch.setattr(utils_module, "AGENT_CARD_PATH", nonexistent_path)

        result = load_agent_card()

        assert result is None

    def test_returns_none_with_url_param_when_file_not_exists(self, tmp_path, monkeypatch):
        nonexistent_path = str(tmp_path / "nonexistent" / "agent-card.json")
        monkeypatch.setattr(utils_module, "AGENT_CARD_PATH", nonexistent_path)

        result = load_agent_card(url="http://localhost:8080/")

        assert result is None


class TestLoadAgentCardBasic:
    """Tests for basic agent card loading functionality."""

    def test_loads_minimal_agent_card(self, minimal_agent_card_data, tmp_path, monkeypatch):
        agent_card_path = tmp_path / "agent-card.json"
        agent_card_path.write_text(json.dumps(minimal_agent_card_data))
        monkeypatch.setattr(utils_module, "AGENT_CARD_PATH", str(agent_card_path))

        result = load_agent_card(url="http://test.local/")

        assert result is not None
        assert result.name == "Test Agent"
        assert result.description == "A test agent for unit testing"
        assert result.version == "1.0.0"
        assert result.url == "http://test.local/"
        assert result.default_input_modes == ["application/json"]
        assert result.default_output_modes == ["application/json"]

    def test_loads_agent_card_with_all_fields(self, full_agent_card_data, tmp_path, monkeypatch):
        agent_card_path = tmp_path / "agent-card.json"
        agent_card_path.write_text(json.dumps(full_agent_card_data))
        monkeypatch.setattr(utils_module, "AGENT_CARD_PATH", str(agent_card_path))

        result = load_agent_card(url="http://full.test/")

        assert result is not None
        assert result.name == "Full Test Agent"
        assert result.description == "A fully configured test agent"
        assert result.version == "2.0.0"
        assert result.default_input_modes == ["application/json", "text/plain"]
        assert result.default_output_modes == ["application/json", "text/plain"]


class TestLoadAgentCardCapabilities:
    """Tests for capabilities conversion."""

    def test_converts_capabilities_correctly(self, full_agent_card_data, tmp_path, monkeypatch):
        agent_card_path = tmp_path / "agent-card.json"
        agent_card_path.write_text(json.dumps(full_agent_card_data))
        monkeypatch.setattr(utils_module, "AGENT_CARD_PATH", str(agent_card_path))

        result = load_agent_card(url="http://test/")

        assert result.capabilities.streaming is True
        assert result.capabilities.push_notifications is True
        assert result.capabilities.state_transition_history is True

    def test_handles_false_capabilities(self, minimal_agent_card_data, tmp_path, monkeypatch):
        agent_card_path = tmp_path / "agent-card.json"
        agent_card_path.write_text(json.dumps(minimal_agent_card_data))
        monkeypatch.setattr(utils_module, "AGENT_CARD_PATH", str(agent_card_path))

        result = load_agent_card(url="http://test/")

        assert result.capabilities.streaming is False
        assert result.capabilities.push_notifications is False
        assert result.capabilities.state_transition_history is False

    def test_handles_none_capabilities_as_false(
        self, agent_card_with_none_capabilities, tmp_path, monkeypatch
    ):
        agent_card_path = tmp_path / "agent-card.json"
        agent_card_path.write_text(json.dumps(agent_card_with_none_capabilities))
        monkeypatch.setattr(utils_module, "AGENT_CARD_PATH", str(agent_card_path))

        result = load_agent_card(url="http://test/")

        assert result.capabilities.streaming is False
        assert result.capabilities.push_notifications is False
        assert result.capabilities.state_transition_history is False


class TestLoadAgentCardSkills:
    """Tests for skills conversion."""

    def test_converts_single_skill(self, minimal_agent_card_data, tmp_path, monkeypatch):
        agent_card_path = tmp_path / "agent-card.json"
        agent_card_path.write_text(json.dumps(minimal_agent_card_data))
        monkeypatch.setattr(utils_module, "AGENT_CARD_PATH", str(agent_card_path))

        result = load_agent_card(url="http://test/")

        assert len(result.skills) == 1
        skill = result.skills[0]
        assert skill.id == "test-skill"
        assert skill.name == "Test Skill"
        assert skill.description == "A test skill"
        assert skill.tags == ["test", "example"]
        assert skill.examples is None
        assert skill.input_modes is None
        assert skill.output_modes is None

    def test_converts_multiple_skills(self, full_agent_card_data, tmp_path, monkeypatch):
        agent_card_path = tmp_path / "agent-card.json"
        agent_card_path.write_text(json.dumps(full_agent_card_data))
        monkeypatch.setattr(utils_module, "AGENT_CARD_PATH", str(agent_card_path))

        result = load_agent_card(url="http://test/")

        assert len(result.skills) == 2

        skill1 = result.skills[0]
        assert skill1.id == "skill-1"
        assert skill1.name == "Primary Skill"
        assert skill1.examples == ["Example prompt 1", "Example prompt 2"]
        assert skill1.input_modes == ["application/json"]
        assert skill1.output_modes == ["text/plain"]

        skill2 = result.skills[1]
        assert skill2.id == "skill-2"
        assert skill2.name == "Secondary Skill"
        assert skill2.examples is None


class TestLoadAgentCardUrl:
    """Tests for URL handling."""

    def test_uses_url_parameter(self, minimal_agent_card_data, tmp_path, monkeypatch):
        agent_card_path = tmp_path / "agent-card.json"
        agent_card_path.write_text(json.dumps(minimal_agent_card_data))
        monkeypatch.setattr(utils_module, "AGENT_CARD_PATH", str(agent_card_path))

        result = load_agent_card(url="http://custom-url.example.com:8080/")

        assert result.url == "http://custom-url.example.com:8080/"

    def test_uses_env_var_when_no_url_param(self, minimal_agent_card_data, tmp_path, monkeypatch):
        agent_card_path = tmp_path / "agent-card.json"
        agent_card_path.write_text(json.dumps(minimal_agent_card_data))
        monkeypatch.setattr(utils_module, "AGENT_CARD_PATH", str(agent_card_path))
        monkeypatch.setenv("FLOKOA_AGENT_URL", "http://from-env-var.example.com/")

        result = load_agent_card()

        assert result.url == "http://from-env-var.example.com/"

    def test_url_param_takes_precedence_over_env_var(
        self, minimal_agent_card_data, tmp_path, monkeypatch
    ):
        agent_card_path = tmp_path / "agent-card.json"
        agent_card_path.write_text(json.dumps(minimal_agent_card_data))
        monkeypatch.setattr(utils_module, "AGENT_CARD_PATH", str(agent_card_path))
        monkeypatch.setenv("FLOKOA_AGENT_URL", "http://from-env-var.example.com/")

        result = load_agent_card(url="http://param-url.example.com/")

        assert result.url == "http://param-url.example.com/"

    def test_defaults_to_empty_string_when_no_url(
        self, minimal_agent_card_data, tmp_path, monkeypatch
    ):
        agent_card_path = tmp_path / "agent-card.json"
        agent_card_path.write_text(json.dumps(minimal_agent_card_data))
        monkeypatch.setattr(utils_module, "AGENT_CARD_PATH", str(agent_card_path))
        monkeypatch.delenv("FLOKOA_AGENT_URL", raising=False)

        result = load_agent_card()

        assert result.url == ""
