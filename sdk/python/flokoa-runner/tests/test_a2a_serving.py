"""In-process A2A serving tests: the same JSON-RPC surface the platform
(playground, trigger invoke, push gateway, Argo plugin) speaks today."""

import json
import uuid

import httpx
import pytest
from flokoa_runner.agent import build_agent
from flokoa_runner.serve import build_app, load_card

CARD = {
    "name": "golden-agent",
    "description": "runner serving test",
    "version": "0.1.0",
    "capabilities": {"streaming": False},
    "defaultInputModes": ["application/json"],
    "defaultOutputModes": ["application/json"],
    "skills": [],
}


@pytest.fixture
def anyio_backend():
    return "asyncio"


@pytest.fixture
def app(etc_flokoa):
    (etc_flokoa / "agent-card.json").write_text(json.dumps(CARD))
    agent = build_agent({"model": "test", "instructions": "Answer tersely."})
    card = load_card("http://golden-agent.default.svc.cluster.local:80/", etc_flokoa / "agent-card.json")
    return build_app(agent, card)


@pytest.fixture
async def client(app):
    transport = httpx.ASGITransport(app=app)
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
async def test_agent_card_is_served(client):
    response = await client.get("/.well-known/agent-card.json")
    assert response.status_code == 200
    card = response.json()
    assert card["name"] == "golden-agent"
    assert card["url"] == "http://golden-agent.default.svc.cluster.local:80/"


@pytest.mark.anyio
async def test_health_endpoints(client):
    assert (await client.get("/health")).status_code == 200


@pytest.mark.anyio
async def test_message_send_completes_with_artifact(client):
    response = await client.post("/", json=rpc_message("hello"))
    assert response.status_code == 200
    body = response.json()
    assert body.get("error") is None, body
    result = body["result"]
    assert result["status"]["state"] == "completed"
    artifacts = result.get("artifacts") or []
    assert artifacts, f"expected artifacts in {result}"
    parts = artifacts[0]["parts"]
    assert parts and parts[0]["kind"] in ("text", "data")


@pytest.mark.anyio
async def test_invalid_method_returns_jsonrpc_error(client):
    request = rpc_message("x")
    request["method"] = "nonsense/method"
    response = await client.post("/", json=request)
    body = response.json()
    assert body.get("error") is not None
