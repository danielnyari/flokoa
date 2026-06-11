"""Stage: serve — A2A serving of the hydrated agent."""

from __future__ import annotations

import json
import logging
import os
from pathlib import Path
from typing import TYPE_CHECKING, Any

from a2a.types import AgentCard

from flokoa_runner.errors import BootstrapError

if TYPE_CHECKING:
    from fastapi import FastAPI
    from pydantic_ai import Agent

logger = logging.getLogger(__name__)

DEFAULT_CARD_PATH = "/etc/flokoa/agent-card.json"


def load_card(public_url: str, path: str | Path | None = None) -> AgentCard:
    """Load the operator-rendered card and stamp the published endpoint."""
    card_path = Path(path or os.environ.get("FLOKOA_AGENT_CARD_PATH", DEFAULT_CARD_PATH))
    try:
        data: dict[str, Any] = json.loads(card_path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        raise BootstrapError("serve", "agent card not found", path=str(card_path)) from None
    except json.JSONDecodeError as exc:
        raise BootstrapError("serve", f"agent card invalid: {exc}", path=str(card_path)) from exc

    data.setdefault("url", public_url)
    data.setdefault("protocolVersion", "0.3.0")
    try:
        return AgentCard.model_validate(data)
    except Exception as exc:
        raise BootstrapError("serve", f"agent card failed validation: {exc}", path=str(card_path)) from exc


def build_app(agent: Agent[Any, Any], card: AgentCard) -> FastAPI:
    """Assemble the A2A FastAPI app around the spec-built agent."""
    from flokoa.serving import build_app as build_serving_app

    return build_serving_app(agent, card)


def serve(agent: Agent[Any, Any]) -> None:
    import uvicorn

    host = os.environ.get("FLOKOA_HOST", "0.0.0.0")  # noqa: S104
    port = int(os.environ.get("FLOKOA_PORT", "8080"))
    public_url = os.environ.get("FLOKOA_PUBLIC_URL", f"http://{host}:{port}/")

    card = load_card(public_url)
    app = build_app(agent, card)

    logger.info("Serving A2A on %s:%d (public url %s)", host, port, public_url)
    uvicorn.run(app, host=host, port=port)
