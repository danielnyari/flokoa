import asyncio
import importlib
import os
import sys

import click
import uvicorn
from a2a.server.apps import A2AFastAPIApplication
from a2a.server.request_handlers import DefaultRequestHandler
from a2a.server.tasks import InMemoryTaskStore
from fastapi import FastAPI

from flokoa.integrations import IntegrationType, get_executor_cls
from flokoa.utils import load_agent_card
from flokoa.utils.agent_card_builder import AgentCardBuilder
from flokoa.utils.router import router as health_router


@click.group()
def cli() -> None:
    """Flokoa - AI Agent platform CLI."""


@cli.command()
@click.option(
    "--module",
    "-m",
    type=str,
    required=True,
    help="Module path to the agent (e.g. my_module:my_agent).",
)
@click.option("--host", default=None, help="Host to bind the server to.")
@click.option("--port", default=None, type=int, help="Port to bind the server to.")
@click.option("--framework", type=click.Choice(IntegrationType, case_sensitive=False))
def run(
    module: str,
    host: str | None,
    port: int | None,
    framework: str,
) -> None:
    """Run a Flokoa agent server.

    \b
    Usage:
      flokoa run -m my_module:my_agent --framework pydantic-ai
    """
    _start_integration(
        module=module,
        host=host or "localhost",
        port=port or 10001,
        framework=framework,
    )


def _start_integration(module: str, host: str, port: int, framework: IntegrationType) -> None:
    """Start a user-provided agent with an A2A server."""
    cwd = os.getcwd()
    if cwd not in sys.path:
        sys.path.insert(0, cwd)

    executor_cls = get_executor_cls(framework)
    agent_parts = module.split(":")
    agent_module_name = agent_parts[0]
    agent_cls_name = agent_parts[1]

    try:
        agent_module = importlib.import_module(agent_module_name)
        try:
            agent_obj = getattr(agent_module, agent_cls_name)
            agent_executor = executor_cls(agent=agent_obj)
        except AttributeError as e:
            raise ImportError(f"Agent '{agent_cls_name}' not found in module '{agent_module_name}': {e}") from e
    except ImportError as e:
        raise ImportError(f"Could not import agent module '{module}': {e}") from e

    agent_card = load_agent_card(url=f"http://{host}:{port}/")

    if agent_card is None:
        builder = AgentCardBuilder(agent=agent_obj, rpc_url=f"http://{host}:{port}/")
        agent_card = asyncio.run(builder.build())

    app = _get_app(agent_executor=agent_executor, agent_card=agent_card)
    _run_server(app=app, host=host, port=port)


def _get_app(agent_executor, agent_card) -> FastAPI:
    """Create and run the A2A server."""
    request_handler = DefaultRequestHandler(
        agent_executor=agent_executor,
        task_store=InMemoryTaskStore(),
    )

    server = A2AFastAPIApplication(
        agent_card=agent_card,
        http_handler=request_handler,
    )

    app = server.build()

    app.include_router(health_router)
    return app


def _run_server(app: FastAPI, host: str, port: int) -> None:
    """Run the FastAPI app with Uvicorn."""
    uvicorn.run(app, host=host, port=port)


def main() -> None:
    """Entry point for the flokoa CLI."""
    cli()


if __name__ == "__main__":
    main()
