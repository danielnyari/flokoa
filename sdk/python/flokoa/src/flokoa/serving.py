"""A2A serving for pydantic-ai agents.

Shared by the generic runner (``flokoa-runner``) and ``flokoa run``: a simple
executor around a fully-constructed Agent — model, tools, and instructions
all live inside it (for the runner, the compiled spec is immutable per pod
generation; spec changes roll the Deployment). No config caches, no
per-request overrides.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Any

from a2a.server.agent_execution import AgentExecutor, RequestContext
from a2a.server.apps import A2AFastAPIApplication
from a2a.server.events import EventQueue
from a2a.server.request_handlers import DefaultRequestHandler
from a2a.server.tasks import InMemoryTaskStore
from a2a.types import (
    AgentCard,
    TaskArtifactUpdateEvent,
    TaskState,
    TaskStatus,
    TaskStatusUpdateEvent,
)
from a2a.utils import new_data_artifact, new_task, new_text_artifact

from flokoa import context as flokoa_context

if TYPE_CHECKING:
    from fastapi import FastAPI
    from pydantic_ai import Agent

logger = logging.getLogger(__name__)

__all__ = ["SpecAgentExecutor", "build_app"]


class SpecAgentExecutor(AgentExecutor):
    """Runs a constructed pydantic-ai agent for each A2A request."""

    def __init__(self, agent: Agent[Any, Any]) -> None:
        self._agent = agent

    @property
    def agent(self) -> Agent[Any, Any]:
        return self._agent

    async def execute(self, context: RequestContext, event_queue: EventQueue) -> None:
        request = context.get_user_input()
        task = context.current_task
        if not task:
            task = new_task(context.message)
            await event_queue.enqueue_event(task)

        # Expose the session identity to capabilities (flokoa.context).
        flokoa_context.bind_request(task.context_id, task.id)

        logger.info("Running agent (task=%s context=%s)", task.id, task.context_id)
        result = await self._agent.run(request)

        output = result.output
        if isinstance(output, str):
            artifact = new_text_artifact(name="result", text=output)
        else:
            data = output if isinstance(output, dict) else _as_jsonable(output)
            artifact = new_data_artifact(name="result", data=data)

        await event_queue.enqueue_event(
            TaskArtifactUpdateEvent(
                append=False,
                context_id=task.context_id,
                task_id=task.id,
                last_chunk=True,
                artifact=artifact,
            )
        )
        await event_queue.enqueue_event(
            TaskStatusUpdateEvent(
                status=TaskStatus(state=TaskState.completed),
                final=True,
                context_id=task.context_id,
                task_id=task.id,
            )
        )

    async def cancel(self, context: RequestContext, event_queue: EventQueue) -> None:
        raise NotImplementedError("cancel is not supported")


def build_app(agent: Agent[Any, Any], card: AgentCard) -> FastAPI:
    """Assemble the A2A FastAPI app (JSON-RPC endpoint + card + health)."""
    from flokoa.telemetry import instrument_fastapi
    from flokoa.utils.router import router as health_router

    request_handler = DefaultRequestHandler(
        agent_executor=SpecAgentExecutor(agent),
        task_store=InMemoryTaskStore(),
    )
    server = A2AFastAPIApplication(agent_card=card, http_handler=request_handler)
    app = server.build()
    app.include_router(health_router)
    instrument_fastapi(app)
    return app


def _as_jsonable(output: Any) -> dict[str, Any]:
    from pydantic import BaseModel

    if isinstance(output, BaseModel):
        return output.model_dump(mode="json")
    return {"result": str(output)}
