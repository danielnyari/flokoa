import asyncio
import importlib
import os
import sys

import click
import uvicorn
from a2a.server.apps import A2AFastAPIApplication
from a2a.server.request_handlers import DefaultRequestHandler
from a2a.server.tasks import InMemoryTaskStore

from flokoa.integrations import IntegrationType, get_executor_cls
from flokoa.utils import load_agent_card
from flokoa.utils.agent_card_builder import AgentCardBuilder


@click.command()
@click.argument("agent")
@click.option("--host", "host", default="localhost")
@click.option("--port", "port", default=10001)
@click.option("--framework", type=click.Choice(IntegrationType, case_sensitive=False))
def main(agent: str, host: str, port: int, framework: str) -> None:
    """Run a Flokoa agent server."""
    # Add current working directory to path for local module imports
    cwd = os.getcwd()
    if cwd not in sys.path:
        sys.path.insert(0, cwd)

    executor_cls = get_executor_cls(framework)
    agent_parts = agent.split(":")
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
        raise ImportError(f"Could not import agent module '{agent}': {e}") from e

    # Try to load agent card from mounted ConfigMap (Kubernetes deployment)
    # Falls back to building from the provided agent for local development
    agent_card = load_agent_card(url=f"http://{host}:{port}/")

    if agent_card is None:
        builder = AgentCardBuilder(agent=agent_obj, rpc_url=f"http://{host}:{port}/")
        agent_card = asyncio.run(builder.build())

    request_handler = DefaultRequestHandler(
        agent_executor=agent_executor,
        task_store=InMemoryTaskStore(),
    )

    server = A2AFastAPIApplication(
        agent_card=agent_card,
        http_handler=request_handler,
    )

    uvicorn.run(server.build(), host=host, port=port)


if __name__ == "__main__":
    main()
