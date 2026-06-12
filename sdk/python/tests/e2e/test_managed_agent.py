"""E2E tests for the managed-agent Docker container via A2A protocol."""

import pytest
from a2a.client import ClientConfig, ClientFactory, create_text_message_object
from a2a.types import TaskState

pytestmark = pytest.mark.anyio


async def test_agent_card_reachable(agent_url: str) -> None:
    """The agent serves a valid agent card at the well-known path."""
    client = await ClientFactory.connect(
        agent=agent_url,
        client_config=ClientConfig(streaming=False),
    )
    card = await client.get_card()
    assert card.name, "Agent card should have a non-empty name"


async def test_send_message_returns_completed(agent_url: str) -> None:
    """Sending a simple message returns a completed task with an artifact."""
    client = await ClientFactory.connect(
        agent=agent_url,
        client_config=ClientConfig(streaming=False),
    )

    message = create_text_message_object(content="What is 2+2? Answer in one word.")

    async for event in client.send_message(message):
        assert isinstance(event, tuple), f"Expected ClientEvent tuple, got {type(event)}"
        task, _update = event
        assert task.status.state == TaskState.completed
        assert task.artifacts, "Expected at least one artifact"

        artifact = task.artifacts[0]
        assert artifact.name == "response"
        data_parts = [p for p in artifact.parts if p.root.kind == "data"]
        assert data_parts, "Expected a data part in the artifact"
        assert "answer" in data_parts[0].root.data
