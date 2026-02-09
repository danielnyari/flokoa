import logging

import click
import uvicorn
from a2a.server.apps import A2AFastAPIApplication
from a2a.server.request_handlers import DefaultRequestHandler
from a2a.server.tasks import InMemoryTaskStore

from flokoa.managed.agent import ManagedAgentBuilder
from flokoa.managed.agent_executor import ManagedPydanticAIAgentExecutor
from flokoa.utils import load_agent_card, load_instruction, load_managed_config

logger = logging.getLogger(__name__)


@click.command()
@click.option("--host", default="0.0.0.0", help="Host to bind the server to.")  # noqa: S104
@click.option("--port", default=8080, type=int, help="Port to bind the server to.")
def managed(host: str, port: int) -> None:
    """Run a Flokoa managed agent server.

    This is the entrypoint for the managed agent runtime container.
    All configuration is loaded from operator-mounted files:

    \b
      /etc/flokoa/managed-config.json  - Output schema and managed config
      /etc/flokoa/instruction.txt      - System instruction
      /etc/flokoa/model.json           - Model and provider configuration
      /etc/flokoa/agent-card.json      - A2A agent card
      /etc/flokoa/tools/               - Tool definitions
    """
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(name)s %(levelname)s %(message)s")

    # Load managed configuration
    managed_config = load_managed_config()
    instruction = load_instruction()

    if instruction is None:
        raise click.ClickException(
            "No instruction found. Managed agents require an instruction at /etc/flokoa/instruction.txt"
        )

    if managed_config is not None:
        builder = ManagedAgentBuilder.from_managed_config(managed_config, instruction=instruction)
    else:
        builder = ManagedAgentBuilder()
        builder.set_instruction(instruction)

    logger.info("Building managed pydantic-ai agent")
    executor = ManagedPydanticAIAgentExecutor(builder=builder, managed_config=managed_config)

    # Load agent card (always expected to be present in managed mode)
    agent_card = load_agent_card(url=f"http://{host}:{port}/")
    if agent_card is None:
        raise click.ClickException(
            "No agent card found at /etc/flokoa/agent-card.json. Managed agents require an agent card."
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


if __name__ == "__main__":
    managed()
