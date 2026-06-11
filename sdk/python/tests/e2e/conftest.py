"""E2E test fixtures — Docker container lifecycle for managed-agent."""

import os
import subprocess
import time

import httpx
import pytest

AGENT_IMAGE = "flokoa-managed-agent:local"
AGENT_CONTAINER = "flokoa-e2e-agent"
AGENT_PORT = 18080  # non-default to avoid clashes with manually-run containers

SAMPLES_DIR = os.path.join(os.path.dirname(__file__), os.pardir, os.pardir, "samples")


def _agent_config_path() -> str:
    return os.path.join(SAMPLES_DIR, "agent-config.json")


def _require_openai_key() -> str:
    key = os.environ.get("OPENAI_API_KEY")
    if not key:
        pytest.skip("OPENAI_API_KEY not set")
    return key


# ---------------------------------------------------------------------------
# managed-agent: long-running A2A server
# ---------------------------------------------------------------------------


@pytest.fixture(scope="session")
def agent_url() -> str:
    """Start managed-agent in Docker and return its base URL.

    The container is torn down after the test session.
    """
    api_key = _require_openai_key()

    # Stop stale container if present
    subprocess.run(
        ["docker", "rm", "-f", AGENT_CONTAINER],
        capture_output=True,
    )

    url = f"http://localhost:{AGENT_PORT}"

    subprocess.run(
        [
            "docker",
            "run",
            "-d",
            "--name",
            AGENT_CONTAINER,
            "-p",
            f"{AGENT_PORT}:8080",
            "-e",
            f"OPENAI_API_KEY={api_key}",
            "-e",
            f"FLOKOA_PUBLIC_URL={url}",
            "-v",
            f"{os.path.abspath(_agent_config_path())}:/etc/flokoa/agent-config.json:ro",
            AGENT_IMAGE,
        ],
        check=True,
        capture_output=True,
    )

    # Wait for the server to become healthy
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        try:
            r = httpx.get(f"{url}/health", timeout=2)
            if r.status_code == 200:
                break
        except (httpx.ConnectError, httpx.ReadError, httpx.TimeoutException):
            pass
        time.sleep(1)
    else:
        logs = subprocess.run(["docker", "logs", AGENT_CONTAINER], capture_output=True, text=True)
        pytest.fail(f"managed-agent not healthy after 60s.\n{logs.stdout}\n{logs.stderr}")

    yield url

    subprocess.run(["docker", "rm", "-f", AGENT_CONTAINER], capture_output=True)
