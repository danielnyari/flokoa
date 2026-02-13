"""E2E tests for health and readiness endpoints."""

import pytest

pytestmark = pytest.mark.anyio


async def test_health_endpoint(client):
    response = await client.get("/health")
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


async def test_ready_endpoint(client):
    response = await client.get("/ready")
    assert response.status_code == 200
    assert response.json() == {"status": "ready"}
