"""Shared test fixtures for flokoa-managed-agent.

Provides real file-based fixtures for config, model, instruction, and tools.
Only LLM calls are mocked (via pydantic-ai TestModel).
"""

import json

import flokoa.utils as utils_module
import pytest
from flokoa.cache import ConfigCache
from flokoa_types.templateconfig import OutputSchema, TemplateConfig
from pydantic_ai import models
from pydantic_ai.models.test import TestModel

from flokoa_managed_agent.agent_executor import TemplatedPydanticAIAgentExecutor

models.ALLOW_MODEL_REQUESTS = False

TEMPLATE_CONFIG_DATA = {
    "outputSchema": {
        "name": "TestOutput",
        "description": "A test output schema",
        "jsonSchema": {
            "type": "object",
            "properties": {
                "answer": {"type": "string"},
            },
            "required": ["answer"],
        },
    },
}

MODEL_CONFIG_DATA = {
    "provider": {"type": "openai"},
    "model": "gpt-4o-mini",
}

INSTRUCTION_TEXT = "You are a helpful test agent. Answer questions concisely."


@pytest.fixture
def template_config():
    """Create a TemplateConfig object directly."""
    return TemplateConfig(
        output_schema=OutputSchema(
            name="TestOutput",
            description="A test output schema",
            json_schema={
                "type": "object",
                "properties": {
                    "answer": {"type": "string"},
                },
                "required": ["answer"],
            },
        ),
    )


@pytest.fixture
def template_config_file(tmp_path, monkeypatch):
    """Write template config JSON and set env var to point to it."""
    path = tmp_path / "template-config.json"
    path.write_text(json.dumps(TEMPLATE_CONFIG_DATA))
    monkeypatch.setenv("FLOKOA_TEMPLATE_CONFIG_PATH", str(path))
    return path


@pytest.fixture
def tools_dir(tmp_path, monkeypatch):
    """Create an empty tools directory and patch the path."""
    tools_path = tmp_path / "tools"
    tools_path.mkdir()
    monkeypatch.setattr(utils_module, "TOOLS_PATH", str(tools_path) + "/")
    return tools_path


@pytest.fixture
def model_config_file(tmp_path, monkeypatch):
    """Write model config JSON and patch MODEL_CONFIG_PATH."""
    path = tmp_path / "model.json"
    path.write_text(json.dumps(MODEL_CONFIG_DATA))
    monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(path))
    return path


@pytest.fixture
def instruction_file(tmp_path, monkeypatch):
    """Write instruction text and patch INSTRUCTION_PATH."""
    path = tmp_path / "instruction.txt"
    path.write_text(INSTRUCTION_TEXT)
    monkeypatch.setattr(utils_module, "INSTRUCTION_PATH", str(path))
    return path


@pytest.fixture
def no_instruction_file(tmp_path, monkeypatch):
    """Ensure no instruction file exists."""
    monkeypatch.setattr(utils_module, "INSTRUCTION_PATH", str(tmp_path / "nonexistent.txt"))


class TestTemplatedExecutor(TemplatedPydanticAIAgentExecutor):
    """Executor subclass that returns TestModel instead of a real LLM provider."""

    def _create_provider(self, provider_type):
        return None

    def _create_model(self, provider):
        return TestModel()


@pytest.fixture
def executor(template_config, tools_dir, model_config_file, instruction_file):
    """Create a TemplatedPydanticAIAgentExecutor with TestModel for LLM."""
    return TestTemplatedExecutor(config=template_config, cache=ConfigCache())


@pytest.fixture
def executor_no_instruction(template_config, tools_dir, model_config_file, no_instruction_file):
    """Create an executor without an instruction file mounted."""
    return TestTemplatedExecutor(config=template_config, cache=ConfigCache())


@pytest.fixture
def executor_no_model(template_config, tools_dir, instruction_file, tmp_path, monkeypatch):
    """Create an executor without a model config file."""
    monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(tmp_path / "nonexistent.json"))
    return TestTemplatedExecutor(config=template_config, cache=ConfigCache())
