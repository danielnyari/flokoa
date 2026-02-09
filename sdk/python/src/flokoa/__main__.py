import asyncio
import importlib
import logging
import os
import sys

import click
import uvicorn
from a2a.server.apps import A2AFastAPIApplication
from a2a.server.request_handlers import DefaultRequestHandler
from a2a.server.tasks import InMemoryTaskStore

from flokoa.integrations import IntegrationType, get_executor_cls
from flokoa.utils import load_agent_card, load_managed_config
from flokoa.utils.agent_card_builder import AgentCardBuilder

logger = logging.getLogger(__name__)


@click.group()
def cli() -> None:
    """Flokoa - AI Agent platform CLI."""


@cli.command()
@click.argument("agent")
@click.option("--host", default="localhost", help="Host to bind the server to.")
@click.option("--port", default=10001, type=int, help="Port to bind the server to.")
@click.option("--framework", type=click.Choice(IntegrationType, case_sensitive=False))
def run(agent: str, host: str, port: int, framework: str) -> None:
    """Run a user-provided agent with an A2A server.

    AGENT is the module:object path to the agent instance (e.g. my_module:my_agent).
    """
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


@cli.command()
@click.option("--host", default="0.0.0.0", help="Host to bind the server to.")  # noqa: S104
@click.option("--port", default=8080, type=int, help="Port to bind the server to.")
def serve(host: str, port: int) -> None:
    """Serve a managed agent from operator-mounted configuration.

    This is the entrypoint for the managed agent runtime container.
    All configuration is read from mounted files:

    \b
      /etc/flokoa/managed-config.json  - Managed agent config (output schema)
      /etc/flokoa/instruction.txt      - System instruction
      /etc/flokoa/model.json           - Model and provider configuration
      /etc/flokoa/agent-card.json      - A2A agent card
      /etc/flokoa/tools/               - Tool definitions
    """
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(name)s %(levelname)s %(message)s")

    from flokoa.managed.agent import ManagedAgentBuilder
    from flokoa.managed.agent_executor import ManagedPydanticAIAgentExecutor
    from flokoa.utils import load_instruction

    managed_config = load_managed_config()
    instruction = load_instruction()

    if instruction is None:
        raise click.ClickException("No instruction found. Managed agents require spec.instruction to be set.")

    builder = ManagedAgentBuilder(config=managed_config)

    logger.info("Building managed pydantic-ai agent")
    executor = ManagedPydanticAIAgentExecutor(builder=builder, instruction=instruction)

    agent_card = load_agent_card(url=f"http://{host}:{port}/")
    if agent_card is None:
        raise click.ClickException(
            "No agent card found at /etc/flokoa/agent-card.json. Managed agents require spec.card to be set."
        )

    request_handler = DefaultRequestHandler(
        agent_executor=executor,
        task_store=InMemoryTaskStore(),
    )

    server = A2AFastAPIApplication(
        agent_card=agent_card,
        http_handler=request_handler,
    )

    logger.info("Starting managed agent server on %s:%d", host, port)
    uvicorn.run(server.build(), host=host, port=port)


def main() -> None:
    """Entry point for the flokoa CLI."""
    cli()


if __name__ == "__main__":
    main()
