"""Tests for flokoa.serving: the A2A surface shared by the runner and the CLI."""

import uuid

import httpx
import pytest
from a2a.types import AgentCard
from pydantic_ai import Agent

from flokoa.serving import build_app

CARD = AgentCard.model_validate(
    {
        "name": "serving-test",
        "description": "serving test agent",
        "version": "0.1.0",
        "url": "http://localhost:10001/",
        "protocolVersion": "0.3.0",
        "capabilities": {"streaming": False},
        "defaultInputModes": ["application/json"],
        "defaultOutputModes": ["application/json"],
        "skills": [],
    }
)


@pytest.fixture
def anyio_backend():
    return "asyncio"


@pytest.fixture
async def client():
    agent = Agent("test", instructions="Answer tersely.")
    transport = httpx.ASGITransport(app=build_app(agent, CARD))
    async with httpx.AsyncClient(transport=transport, base_url="http://testserver") as c:
        yield c


def rpc_message(text: str) -> dict:
    return {
        "jsonrpc": "2.0",
        "id": str(uuid.uuid4()),
        "method": "message/send",
        "params": {
            "message": {
                "role": "user",
                "parts": [{"kind": "text", "text": text}],
                "messageId": str(uuid.uuid4()),
                "kind": "message",
            }
        },
    }


@pytest.mark.anyio
async def test_card_and_health(client):
    assert (await client.get("/.well-known/agent-card.json")).status_code == 200
    assert (await client.get("/health")).status_code == 200


@pytest.mark.anyio
async def test_message_send_completes(client):
    response = await client.post("/", json=rpc_message("hello"))
    body = response.json()
    assert body.get("error") is None, body
    assert body["result"]["status"]["state"] == "completed"
    assert body["result"].get("artifacts")
