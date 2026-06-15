import asyncio
import importlib
import os
import sys
from pathlib import Path

import click
import uvicorn

from flokoa.capability_cli import capability


@click.group()
def cli() -> None:
    """Flokoa - AI Agent platform CLI."""


cli.add_command(capability)


@cli.command()
@click.option(
    "--module",
    "-m",
    type=str,
    default=None,
    help="Module path to the agent (e.g. my_module:my_agent).",
)
@click.option(
    "--file",
    "-f",
    "spec_file",
    type=click.Path(exists=True, dir_okay=False, path_type=Path),
    default=None,
    help="Serve a pydantic-ai AgentSpec file (YAML or JSON) — the local mirror of the cluster runner path.",
)
@click.option("--host", default=None, help="Host to bind the server to.")
@click.option("--port", default=None, type=int, help="Port to bind the server to.")
def run(
    module: str | None,
    spec_file: Path | None,
    host: str | None,
    port: int | None,
) -> None:
    """Run a Flokoa agent server.

    \b
    Usage:
      flokoa run -m my_module:my_agent
      flokoa run -f agentspec.yaml
    """
    host = host or "localhost"
    port = port or 10001

    if module is not None and spec_file is None:
        agent = _load_agent_from_module(module)
    elif spec_file is not None and module is None:
        agent = _load_agent_from_spec(spec_file)
    else:
        raise click.UsageError("exactly one of --module/-m or --file/-f is required")

    _serve(agent, host=host, port=port)


def _load_agent_from_module(module: str):
    """Import a user-constructed pydantic-ai agent from module:attr."""
    cwd = os.getcwd()
    if cwd not in sys.path:
        sys.path.insert(0, cwd)

    module_name, _, attr = module.partition(":")
    if not module_name or not attr:
        raise click.UsageError("--module must be of the form module:attr")

    try:
        agent_module = importlib.import_module(module_name)
    except ImportError as e:
        raise ImportError(f"Could not import agent module '{module_name}': {e}") from e
    try:
        return getattr(agent_module, attr)
    except AttributeError as e:
        raise ImportError(f"Agent '{attr}' not found in module '{module_name}': {e}") from e


def _load_agent_from_spec(spec_file: Path):
    """Hydrate a pydantic-ai AgentSpec file — what the cluster runner does."""
    try:
        from pydantic_ai import Agent
        from pydantic_ai.agent.spec import AgentSpec
    except ImportError as e:
        raise ImportError(
            "flokoa[pydantic-ai] is not installed. Install it with: pip install flokoa[pydantic-ai]"
        ) from e

    spec = AgentSpec.from_file(spec_file)
    return Agent.from_spec(spec)


def _serve(agent, host: str, port: int) -> None:
    """Serve a constructed agent over A2A."""
    from flokoa.serving import build_app
    from flokoa.telemetry import init_telemetry, instrument_pydantic_ai
    from flokoa.utils.agent_card_builder import AgentCardBuilder

    init_telemetry("flokoa-agent", restore_context_from_env=False)
    instrument_pydantic_ai()

    rpc_url = f"http://{host}:{port}/"
    card = asyncio.run(AgentCardBuilder(agent=agent, rpc_url=rpc_url).build())
    app = build_app(agent, card)
    uvicorn.run(app, host=host, port=port)


def main() -> None:
    """Entry point for the flokoa CLI."""
    cli()


if __name__ == "__main__":
    main()
