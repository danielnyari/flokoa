import importlib
import os
import sys

import click
import uvicorn
from a2a.server.apps import A2AFastAPIApplication
from a2a.server.request_handlers import DefaultRequestHandler
from a2a.server.tasks import InMemoryTaskStore
from a2a.types import (
    AgentCapabilities,
    AgentCard,
    AgentSkill,
)

from flokoa.integrations import IntegrationType, get_executor_cls


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

    skill = AgentSkill(
        id="hello_world",
        name="Returns hello world",
        description="just returns hello world",
        tags=["hello world"],
        examples=["hi", "hello world"],
    )

    agent_card = AgentCard(
        name="Hello World Agent",
        description="Just a hello world agent",
        url=f"http://{host}:{port}/",
        version="1.0.0",
        default_input_modes=["application/json"],
        default_output_modes=["application/json"],
        capabilities=AgentCapabilities(streaming=False),
        skills=[skill],
    )

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
