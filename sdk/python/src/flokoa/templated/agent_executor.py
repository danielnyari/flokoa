import logging
from typing import Any, override

from a2a.server.agent_execution import RequestContext
from a2a.server.events import EventQueue
from a2a.types import (
    TaskArtifactUpdateEvent,
    TaskState,
    TaskStatus,
    TaskStatusUpdateEvent,
)
from a2a.utils import new_data_artifact, new_task
from pydantic_ai import AgentRunResult

from flokoa.cache import ConfigCache
from flokoa.exceptions import ProviderNotConfiguredError
from flokoa.integrations.pydantic_ai.agent_executor import PydanticAIAgentExecutor
from flokoa.templated.agent import TemplatedAgentBuilder
from flokoa.types import TemplateConfig

logger = logging.getLogger(__name__)


class TemplatedPydanticAIAgentExecutor(PydanticAIAgentExecutor):
    """Agent executor for the templated pydantic-ai runtime.

    Unlike the integration executor which wraps a user-provided agent,
    this executor creates a bare pydantic-ai Agent internally and drives
    it entirely from operator-mounted configuration:

    - Instruction from /etc/flokoa/instruction.txt (passed at construction)
    - Model config from /etc/flokoa/model.json (via parent)
    - Tools from /etc/flokoa/tools/ (via parent)
    - Templated config from /etc/flokoa/managed-config.json (via builder)
    """

    def __init__(
        self,
        config: TemplateConfig,
        cache: ConfigCache | None = None,
    ):
        agent = TemplatedAgentBuilder.from_config(config=config)
        self._config = config
        super().__init__(agent=agent, cache=cache)

    @property
    def config(self) -> TemplateConfig:
        return self._config

    @property
    @override
    def instruction(self) -> str:
        """Templated agents always use the instruction passed at construction."""
        instruction = super().instruction
        if instruction is None:
            raise ProviderNotConfiguredError("Instruction is required for templated agents")
        return instruction

    @override
    async def execute(self, context: RequestContext, event_queue: EventQueue) -> None:
        request = context.get_user_input()
        task = context.current_task
        if not task:
            task = new_task(context.message)
            await event_queue.enqueue_event(task)

        if not self.model_config:
            raise ProviderNotConfiguredError("Model configuration is required for templated agents")

        model = self._create_model(self._create_provider(self.model_config.provider.type))

        result: AgentRunResult[dict[str, Any]] = await self.agent.run(
            request,
            model=model,
            toolsets=[self._get_toolset()],
            instructions=self.instruction,
        )

        await event_queue.enqueue_event(
            TaskArtifactUpdateEvent(
                append=False,
                context_id=task.context_id,
                task_id=task.id,
                last_chunk=True,
                artifact=new_data_artifact(
                    name=self.config.output_schema.name,
                    description=self.config.output_schema.description,
                    data=result.output,
                ),
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
