"""E2E test fixtures for the flokoa-managed-agent runtime.

Provides fixtures to build a full FastAPI app backed by a
TemplatedPydanticAIAgentExecutor with mocked LLM calls (TestModel).
"""

import json

import pytest
from a2a.server.apps import A2AFastAPIApplication
from a2a.server.request_handlers import DefaultRequestHandler
from a2a.server.tasks import InMemoryTaskStore
from a2a.types import AgentCapabilities, AgentCard, AgentSkill
from httpx import ASGITransport, AsyncClient
from pydantic_ai import models
from pydantic_ai.models.test import TestModel

import flokoa.utils as utils_module
from flokoa.utils.router import router as health_router
from flokoa_managed_agent.agent_executor import TemplatedPydanticAIAgentExecutor
from flokoa_types.templateconfig import OutputSchema, TemplateConfig

models.ALLOW_MODEL_REQUESTS = False

# =============================================================================
# Monkeypatched /etc/flokoa/ path fixtures
# =============================================================================

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

AGENT_CARD_DATA = {
    "name": "test-agent",
    "description": "A test agent for e2e testing",
    "version": "0.0.1",
    "defaultInputModes": ["text/plain"],
    "defaultOutputModes": ["text/plain"],
    "capabilities": {"streaming": False},
    "skills": [
        {
            "id": "test-skill",
            "name": "test",
            "description": "A test skill",
            "tags": ["test"],
        }
    ],
}


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
def agent_card_file(tmp_path, monkeypatch):
    """Write agent card JSON and patch AGENT_CARD_PATH."""
    path = tmp_path / "agent-card.json"
    path.write_text(json.dumps(AGENT_CARD_DATA))
    monkeypatch.setattr(utils_module, "AGENT_CARD_PATH", str(path))
    return path


# =============================================================================
# Direct construction fixtures
# =============================================================================


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
def agent_card():
    """Create an a2a AgentCard directly."""
    return AgentCard(
        name="test-agent",
        description="A test agent for e2e testing",
        url="http://test",
        version="0.0.1",
        capabilities=AgentCapabilities(streaming=False),
        skills=[
            AgentSkill(
                id="test-skill",
                name="test",
                description="A test skill",
                tags=["test"],
            )
        ],
        default_input_modes=["text/plain"],
        default_output_modes=["text/plain"],
    )


class _TestTemplatedExecutor(TemplatedPydanticAIAgentExecutor):
    """Executor subclass that returns TestModel instead of a real provider/model."""

    def _create_provider(self, provider_type):
        return None

    def _create_model(self, provider):
        return TestModel()


@pytest.fixture
def agent_executor(template_config, tools_dir, model_config_file, instruction_file):
    """Create a TemplatedPydanticAIAgentExecutor backed by test config files."""
    return _TestTemplatedExecutor(config=template_config)


@pytest.fixture
def app(agent_executor, agent_card):
    """Build the full FastAPI app."""
    request_handler = DefaultRequestHandler(
        agent_executor=agent_executor,
        task_store=InMemoryTaskStore(),
    )
    server = A2AFastAPIApplication(
        agent_card=agent_card,
        http_handler=request_handler,
    )
    app = server.build()
    app.include_router(health_router)
    return app


@pytest.fixture
async def client(app):
    """Create an httpx AsyncClient for testing the FastAPI app."""
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        yield ac
