"""Entry point for the flokoa-managed-agent runtime.

Supports two configuration modes:

1. **Unified config** — a single ``agent-config.json`` containing the full
   :class:`AgentConfig` (name, instruction, model, tools, output schema).
2. **Legacy config** — scattered files (``template-config.json``,
   ``instruction.txt``, ``model.json``) for backward compatibility.

Usage:
    python -m flokoa_managed_agent
"""

import asyncio
import logging
import os

import uvicorn
from a2a.server.apps import A2AFastAPIApplication
from a2a.server.request_handlers import DefaultRequestHandler
from a2a.server.tasks import InMemoryTaskStore
from fastapi import FastAPI
from flokoa.config import LlmAgentConfig
from flokoa.utils import load_agent_card
from flokoa.utils.agent_card_builder import AgentCardBuilder
from flokoa.utils.router import router as health_router

from flokoa_managed_agent.agent_executor import TemplatedPydanticAIAgentExecutor
from flokoa_managed_agent.config import load_managed_agent_config, load_templated_config

logger = logging.getLogger(__name__)


def main() -> None:
    """Start the managed agent from operator-mounted configuration."""
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(name)s %(levelname)s %(message)s")

    host = os.environ.get("FLOKOA_HOST", "0.0.0.0")  # noqa: S104
    port = int(os.environ.get("FLOKOA_PORT", "8080"))
    public_url = os.environ.get("FLOKOA_PUBLIC_URL", f"http://{host}:{port}/")

    # Try unified config first, then fall back to legacy
    agent_config = load_managed_agent_config()

    if agent_config is not None and isinstance(agent_config.root, LlmAgentConfig):
        logger.info("Using unified AgentConfig (framework=%s)", agent_config.root.framework.value)
        executor = TemplatedPydanticAIAgentExecutor(agent_config=agent_config)
        # Validate instruction is available (raises if missing)
        _ = executor.instruction
    else:
        # Legacy path: scattered files
        logger.info("Using legacy config (template-config.json + instruction.txt + model.json)")
        templated_config = load_templated_config()

        from flokoa.utils import load_instruction

        instruction = load_instruction()
        if instruction is None:
            raise RuntimeError("No instruction found. Templated agents require spec.instruction to be set.")

        executor = TemplatedPydanticAIAgentExecutor(config=templated_config)

    # Use operator-mounted cardOverride if available, otherwise generate from agent
    agent_card = load_agent_card(url=public_url)
    if agent_card is None:
        card_builder = AgentCardBuilder(agent=executor.agent, rpc_url=public_url)
        agent_card = asyncio.run(card_builder.build())

    request_handler = DefaultRequestHandler(
        agent_executor=executor,
        task_store=InMemoryTaskStore(),
    )

    server = A2AFastAPIApplication(
        agent_card=agent_card,
        http_handler=request_handler,
    )

    app: FastAPI = server.build()
    app.include_router(health_router)

    uvicorn.run(app, host=host, port=port)


if __name__ == "__main__":
    main()
