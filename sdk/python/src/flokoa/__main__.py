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
from a2a.types import AgentCapabilities, AgentCard

from flokoa.integrations import IntegrationType, get_executor_cls
from flokoa.utils import load_agent_card, load_templated_config
from flokoa.utils.agent_card_builder import AgentCardBuilder

logger = logging.getLogger(__name__)


class _MutuallyExclusiveOption(click.Option):
    """Click Option subclass that enforces mutual exclusion with another option."""

    def __init__(self, *args, **kwargs):
        self.mutually_exclusive = kwargs.pop("mutually_exclusive", [])
        super().__init__(*args, **kwargs)

    def handle_parse_result(self, ctx, opts, args):
        for mutex_name in self.mutually_exclusive:
            if self.name in opts and mutex_name in opts:
                raise click.UsageError(f"--{self.name} and --{mutex_name} are mutually exclusive.")
        return super().handle_parse_result(ctx, opts, args)


def _validate_mode(ctx, param, value):
    """Callback to ensure exactly one of --module or --templated is provided."""
    if ctx.resilient_parsing:
        return value
    # This runs after all params are parsed (attached to the last of the two).
    # At this point ctx.params has 'module' already.
    module = ctx.params.get("module")
    templated = value
    if not module and not templated:
        raise click.UsageError("Either --module/-m or --templated/-t is required.")
    return value


@click.group()
def cli() -> None:
    """Flokoa - AI Agent platform CLI."""


@cli.command()
@click.option(
    "--module",
    "-m",
    type=str,
    default=None,
    cls=_MutuallyExclusiveOption,
    mutually_exclusive=["templated"],
    help="Module path to the agent (e.g. my_module:my_agent).",
)
@click.option(
    "--templated",
    "-t",
    is_flag=True,
    default=False,
    callback=_validate_mode,
    is_eager=False,
    cls=_MutuallyExclusiveOption,
    mutually_exclusive=["module"],
    help="Run a templated agent from operator-mounted configuration.",
)
@click.option("--host", default=None, help="Host to bind the server to.")
@click.option("--port", default=None, type=int, help="Port to bind the server to.")
@click.option("--framework", type=click.Choice(IntegrationType, case_sensitive=False))
def run(module: str | None, templated: bool, host: str | None, port: int | None, framework: str) -> None:
    """Run a Flokoa agent server.

    \b
    Integration mode:
      flokoa run -m my_module:my_agent --framework pydantic-ai

    \b
    Templated mode (operator runtime):
      flokoa run --templated
    """
    if templated:
        _start_templated(host=host or "0.0.0.0", port=port or 8080)  # noqa: S104
    else:
        _start_integration(module=module, host=host or "localhost", port=port or 10001, framework=framework)


def _start_integration(module: str, host: str, port: int, framework: str) -> None:
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

    _run_server(agent_executor=agent_executor, agent_card=agent_card, host=host, port=port)


def _start_templated(host: str, port: int) -> None:
    """Start a templated agent from operator-mounted configuration."""
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(name)s %(levelname)s %(message)s")

    from flokoa.templated.agent import TemplatedAgentBuilder
    from flokoa.templated.agent_executor import TemplatedPydanticAIAgentExecutor
    from flokoa.utils import load_instruction

    templated_config = load_templated_config()
    instruction = load_instruction()

    if instruction is None:
        raise click.ClickException("No instruction found. Templated agents require spec.instruction to be set.")

    builder = TemplatedAgentBuilder(config=templated_config)

    logger.info("Building templated pydantic-ai agent")
    executor = TemplatedPydanticAIAgentExecutor(builder=builder, instruction=instruction)

    # Use operator-mounted cardOverride if available, otherwise generate a default card
    agent_card = load_agent_card(url=f"http://{host}:{port}/")
    if agent_card is None:
        agent_card = _build_default_agent_card(url=f"http://{host}:{port}/", instruction=instruction)

    _run_server(agent_executor=executor, agent_card=agent_card, host=host, port=port)


def _build_default_agent_card(url: str, instruction: str) -> AgentCard:
    """Build a minimal default A2A AgentCard for a templated agent."""
    description = instruction.split("\n", 1)[0].strip()[:200] if instruction else "Flokoa templated agent"

    return AgentCard(
        name="flokoa-templated-agent",
        description=description,
        version="0.0.1",
        url=url,
        capabilities=AgentCapabilities(streaming=False),
        skills=[],
    )


def _run_server(agent_executor, agent_card, host: str, port: int) -> None:
    """Create and run the A2A server."""
    request_handler = DefaultRequestHandler(
        agent_executor=agent_executor,
        task_store=InMemoryTaskStore(),
    )

    server = A2AFastAPIApplication(
        agent_card=agent_card,
        http_handler=request_handler,
    )

    logger.info("Starting agent server on %s:%d", host, port)
    uvicorn.run(server.build(), host=host, port=port)


def main() -> None:
    """Entry point for the flokoa CLI."""
    cli()


if __name__ == "__main__":
    main()
