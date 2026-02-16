"""E2E tests for the A2A agent card endpoint."""

import pytest

pytestmark = pytest.mark.anyio


async def test_agent_card_endpoint(client):
    response = await client.get("/.well-known/agent.json")
    assert response.status_code == 200

    data = response.json()
    assert data["name"] == "test-agent"
    assert data["description"] == "A test agent for e2e testing"
    assert data["version"] == "0.0.1"
    assert "skills" in data
    assert len(data["skills"]) == 1
    assert data["skills"][0]["id"] == "test-skill"


async def test_agent_card_capabilities(client):
    """Agent card includes capabilities structure."""
    response = await client.get("/.well-known/agent.json")
    data = response.json()
    assert "capabilities" in data
    assert data["capabilities"]["streaming"] is False


async def test_agent_card_default_modes(client):
    """Agent card includes default input/output modes."""
    response = await client.get("/.well-known/agent.json")
    data = response.json()
    assert "text/plain" in data["defaultInputModes"]
    assert "text/plain" in data["defaultOutputModes"]
