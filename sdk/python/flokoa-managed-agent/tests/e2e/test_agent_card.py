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
