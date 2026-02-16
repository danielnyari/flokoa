"""Unit tests for flokoa_managed_agent.config."""

import json

import pytest
from flokoa_types.templateconfig import TemplateConfig
from pydantic import ValidationError

from flokoa_managed_agent.config import load_templated_config

from .conftest import TEMPLATE_CONFIG_DATA


def test_load_from_env_path(template_config_file):
    """Loads valid config from FLOKOA_TEMPLATE_CONFIG_PATH."""
    config = load_templated_config()
    assert isinstance(config, TemplateConfig)
    assert config.output_schema.name == "TestOutput"
    assert config.output_schema.description == "A test output schema"
    assert config.output_schema.json_schema["type"] == "object"


def test_load_file_not_found(tmp_path, monkeypatch):
    """Raises FileNotFoundError when config file does not exist."""
    monkeypatch.setenv("FLOKOA_TEMPLATE_CONFIG_PATH", str(tmp_path / "missing.json"))
    with pytest.raises(FileNotFoundError, match="Templated config file not found"):
        load_templated_config()


def test_load_invalid_json(tmp_path, monkeypatch):
    """Raises json.JSONDecodeError for malformed JSON."""
    bad_file = tmp_path / "bad.json"
    bad_file.write_text("not json {{{")
    monkeypatch.setenv("FLOKOA_TEMPLATE_CONFIG_PATH", str(bad_file))
    with pytest.raises(json.JSONDecodeError):
        load_templated_config()


def test_load_missing_required_fields(tmp_path, monkeypatch):
    """Raises ValidationError when required fields are missing."""
    incomplete = tmp_path / "incomplete.json"
    incomplete.write_text(json.dumps({"outputSchema": {"name": "Test"}}))
    monkeypatch.setenv("FLOKOA_TEMPLATE_CONFIG_PATH", str(incomplete))
    with pytest.raises(ValidationError):
        load_templated_config()


def test_load_with_camel_case_keys(tmp_path, monkeypatch):
    """Parses camelCase keys from the operator-mounted JSON."""
    path = tmp_path / "config.json"
    path.write_text(json.dumps(TEMPLATE_CONFIG_DATA))
    monkeypatch.setenv("FLOKOA_TEMPLATE_CONFIG_PATH", str(path))
    config = load_templated_config()
    assert config.output_schema.name == "TestOutput"


def test_load_preserves_json_schema(template_config_file):
    """The full JSON Schema dict is preserved on output_schema."""
    config = load_templated_config()
    schema = config.output_schema.json_schema
    assert schema["properties"]["answer"]["type"] == "string"
    assert "answer" in schema["required"]


def test_load_with_optional_input_schema(tmp_path, monkeypatch):
    """Config with optional inputSchema parses correctly."""
    data = {
        **TEMPLATE_CONFIG_DATA,
        "inputSchema": {"type": "object", "properties": {"query": {"type": "string"}}},
    }
    path = tmp_path / "config.json"
    path.write_text(json.dumps(data))
    monkeypatch.setenv("FLOKOA_TEMPLATE_CONFIG_PATH", str(path))
    config = load_templated_config()
    assert config.input_schema is not None
    assert config.input_schema["type"] == "object"
