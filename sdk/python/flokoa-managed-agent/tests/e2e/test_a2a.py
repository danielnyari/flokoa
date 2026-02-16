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


async def test_send_message_artifact_has_data_part(client):
    """Response artifact contains a data part with structured output."""
    body = _make_send_message_request("Give me data", request_id="test-3")
    response = await client.post(A2A_RPC_PATH, json=body)
    assert response.status_code == 200

    result = response.json()["result"]
    artifact = result["artifacts"][0]
    assert artifact["name"] == "TestOutput"
    assert artifact["description"] == "A test output schema"
    # Artifact should have parts with kind=data
    data_parts = [p for p in artifact["parts"] if p["kind"] == "data"]
    assert len(data_parts) > 0


async def test_send_message_has_task_id(client):
    """Response result includes a task ID."""
    body = _make_send_message_request("Task check", request_id="test-4")
    response = await client.post(A2A_RPC_PATH, json=body)
    result = response.json()["result"]
    assert "id" in result
    assert isinstance(result["id"], str)


async def test_invalid_jsonrpc_method(client):
    """Invalid JSON-RPC method returns an error."""
    body = {
        "jsonrpc": "2.0",
        "method": "invalid/method",
        "id": "err-1",
        "params": {},
    }
    response = await client.post(A2A_RPC_PATH, json=body)
    data = response.json()
    assert "error" in data


async def test_multiple_sequential_messages(client):
    """Multiple messages can be sent sequentially."""
    for i in range(3):
        body = _make_send_message_request(f"Message {i}", request_id=f"seq-{i}")
        response = await client.post(A2A_RPC_PATH, json=body)
        assert response.status_code == 200
        data = response.json()
        assert data["id"] == f"seq-{i}"
        assert data["result"]["status"]["state"] == "completed"
