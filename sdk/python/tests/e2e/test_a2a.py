"""E2E tests for the A2A JSON-RPC message endpoints."""

import uuid

import pytest

pytestmark = pytest.mark.anyio

A2A_RPC_PATH = "/"


def _make_send_message_request(text: str, request_id: str | None = None):
    """Build a JSON-RPC 2.0 message/send request body."""
    return {
        "jsonrpc": "2.0",
        "method": "message/send",
        "id": request_id or str(uuid.uuid4()),
        "params": {
            "message": {
                "messageId": str(uuid.uuid4()),
                "role": "user",
                "parts": [{"kind": "text", "text": text}],
            },
        },
    }


async def test_send_message(client):
    body = _make_send_message_request("Hello agent", request_id="test-1")
    response = await client.post(A2A_RPC_PATH, json=body)
    assert response.status_code == 200

    data = response.json()
    assert data["jsonrpc"] == "2.0"
    assert data["id"] == "test-1"
    assert "result" in data


async def test_send_message_returns_agent_response(client):
    body = _make_send_message_request("What is 2+2?", request_id="test-2")
    response = await client.post(A2A_RPC_PATH, json=body)
    assert response.status_code == 200

    data = response.json()
    result = data["result"]
    assert result["status"]["state"] == "completed"
    assert len(result["artifacts"]) > 0
