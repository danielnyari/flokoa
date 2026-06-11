"""The contract test that binds roadmap units 03/04/05.

Every golden AgentSpec document the operator's compiler/validator treats as
valid must hydrate into a working pydantic-ai Agent here — same documents,
same pinned pydantic-ai, both sides of the contract.
"""

import json
from pathlib import Path

import pytest
from flokoa_runner.agent import build_agent
from flokoa_runner.secrets import resolve_secrets

SPEC_TESTDATA = Path(__file__).resolve().parents[4] / "operator" / "internal" / "spec" / "testdata"

GOLDEN_VALID = sorted((SPEC_TESTDATA / "valid").glob("*.json"))


@pytest.fixture(autouse=True)
def provider_env(monkeypatch):
    monkeypatch.setenv("OPENAI_API_KEY", "test")
    monkeypatch.setenv("ANTHROPIC_API_KEY", "test")
    monkeypatch.setenv("FLOKOA_SECRET_KB_TOKEN", "test-token")


@pytest.mark.parametrize("path", GOLDEN_VALID, ids=lambda p: p.name)
def test_golden_spec_hydrates(path):
    doc = resolve_secrets(json.loads(path.read_text()))
    agent = build_agent(doc)
    assert agent is not None


@pytest.mark.anyio
async def test_hydrated_agent_answers_a_test_model_run():
    doc = {
        "model": "test",
        "name": "golden",
        "instructions": ["You are a test agent."],
    }
    agent = build_agent(doc)
    result = await agent.run("hello")
    assert result.output


@pytest.mark.anyio
async def test_structured_output_spec_round_trips():
    doc = {
        "model": "test",
        "output_schema": {
            "type": "object",
            "properties": {"answer": {"type": "string"}},
            "required": ["answer"],
        },
    }
    agent = build_agent(doc)
    result = await agent.run("hello")
    assert isinstance(result.output, dict)
    assert "answer" in result.output


@pytest.fixture
def anyio_backend():
    return "asyncio"
