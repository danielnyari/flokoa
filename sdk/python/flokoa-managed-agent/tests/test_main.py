"""Tests for flokoa_managed_agent.__main__ entry point.

Tests error paths that are reachable without starting uvicorn.
"""

import json

import flokoa.utils as utils_module
import pytest

from flokoa_managed_agent.__main__ import main

from .conftest import INSTRUCTION_TEXT, MODEL_CONFIG_DATA, TEMPLATE_CONFIG_DATA


def test_main_raises_without_template_config(tmp_path, monkeypatch):
    """main() raises FileNotFoundError when template config is missing."""
    monkeypatch.setenv("FLOKOA_TEMPLATE_CONFIG_PATH", str(tmp_path / "missing.json"))
    # Ensure unified config path doesn't exist either
    monkeypatch.setenv("FLOKOA_AGENT_CONFIG_PATH", str(tmp_path / "no-unified.json"))
    with pytest.raises(FileNotFoundError, match="Templated config file not found"):
        main()


def test_main_raises_without_instruction(tmp_path, monkeypatch):
    """main() raises RuntimeError when instruction file is missing."""
    # Ensure unified config path doesn't exist
    monkeypatch.setenv("FLOKOA_AGENT_CONFIG_PATH", str(tmp_path / "no-unified.json"))

    # Provide valid template config
    config_path = tmp_path / "template-config.json"
    config_path.write_text(json.dumps(TEMPLATE_CONFIG_DATA))
    monkeypatch.setenv("FLOKOA_TEMPLATE_CONFIG_PATH", str(config_path))

    # Provide valid model config and tools dir
    model_path = tmp_path / "model.json"
    model_path.write_text(json.dumps(MODEL_CONFIG_DATA))
    monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_path))
    tools_path = tmp_path / "tools"
    tools_path.mkdir()
    monkeypatch.setattr(utils_module, "TOOLS_PATH", str(tools_path) + "/")

    # No instruction file
    monkeypatch.setattr(utils_module, "INSTRUCTION_PATH", str(tmp_path / "missing.txt"))

    with pytest.raises(RuntimeError, match="No instruction found"):
        main()


def test_main_builds_app(tmp_path, monkeypatch):
    """main() builds the FastAPI app and calls uvicorn.run when all config is present."""
    # Provide all config files
    config_path = tmp_path / "template-config.json"
    config_path.write_text(json.dumps(TEMPLATE_CONFIG_DATA))
    monkeypatch.setenv("FLOKOA_TEMPLATE_CONFIG_PATH", str(config_path))

    model_path = tmp_path / "model.json"
    model_path.write_text(json.dumps(MODEL_CONFIG_DATA))
    monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_path))

    instruction_path = tmp_path / "instruction.txt"
    instruction_path.write_text(INSTRUCTION_TEXT)
    monkeypatch.setattr(utils_module, "INSTRUCTION_PATH", str(instruction_path))

    tools_path = tmp_path / "tools"
    tools_path.mkdir()
    monkeypatch.setattr(utils_module, "TOOLS_PATH", str(tools_path) + "/")

    # No agent card file → triggers auto-generation path
    monkeypatch.setattr(utils_module, "AGENT_CARD_PATH", str(tmp_path / "no-card.json"))

    # Capture the uvicorn.run call instead of blocking
    captured = {}

    def fake_uvicorn_run(app, **kwargs):
        captured["app"] = app
        captured["kwargs"] = kwargs

    import flokoa_managed_agent.__main__ as main_module

    monkeypatch.setattr(main_module.uvicorn, "run", fake_uvicorn_run)

    main()

    assert "app" in captured
    assert captured["kwargs"]["host"] == "0.0.0.0"
    assert captured["kwargs"]["port"] == 8080


def test_main_respects_host_port_env(tmp_path, monkeypatch):
    """main() uses FLOKOA_HOST and FLOKOA_PORT env vars."""
    config_path = tmp_path / "template-config.json"
    config_path.write_text(json.dumps(TEMPLATE_CONFIG_DATA))
    monkeypatch.setenv("FLOKOA_TEMPLATE_CONFIG_PATH", str(config_path))

    model_path = tmp_path / "model.json"
    model_path.write_text(json.dumps(MODEL_CONFIG_DATA))
    monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_path))

    instruction_path = tmp_path / "instruction.txt"
    instruction_path.write_text(INSTRUCTION_TEXT)
    monkeypatch.setattr(utils_module, "INSTRUCTION_PATH", str(instruction_path))

    tools_path = tmp_path / "tools"
    tools_path.mkdir()
    monkeypatch.setattr(utils_module, "TOOLS_PATH", str(tools_path) + "/")

    monkeypatch.setattr(utils_module, "AGENT_CARD_PATH", str(tmp_path / "no-card.json"))
    monkeypatch.setenv("FLOKOA_HOST", "127.0.0.1")
    monkeypatch.setenv("FLOKOA_PORT", "9090")

    captured = {}

    def fake_uvicorn_run(app, **kwargs):
        captured["kwargs"] = kwargs

    import flokoa_managed_agent.__main__ as main_module

    monkeypatch.setattr(main_module.uvicorn, "run", fake_uvicorn_run)

    main()

    assert captured["kwargs"]["host"] == "127.0.0.1"
    assert captured["kwargs"]["port"] == 9090
