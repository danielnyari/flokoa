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
from flokoa.config import AgentConfig, LlmAgentConfig
from flokoa.exceptions import ProviderNotConfiguredError
from flokoa.integrations.pydantic_ai.agent_executor import PydanticAIAgentExecutor
from flokoa_types import TemplateConfig
from flokoa_managed_agent.bootstrap import TemplatedAgentBuilder, build_agent_from_config

logger = logging.getLogger(__name__)


class TemplatedPydanticAIAgentExecutor(PydanticAIAgentExecutor):
    """Agent executor for the templated pydantic-ai runtime.

    Supports both the unified :class:`AgentConfig` and the legacy
    :class:`TemplateConfig`.

    Unlike the integration executor which wraps a user-provided agent,
    this executor creates a bare pydantic-ai Agent internally and drives
    it entirely from operator-mounted configuration:

    - Instruction from config or /etc/flokoa/instruction.txt
    - Model config from config or /etc/flokoa/model.json (via parent)
    - Tools from /etc/flokoa/tools/ (via parent)
    - Output schema from config
    """

    def __init__(
        self,
        config: TemplateConfig | None = None,
        agent_config: AgentConfig | None = None,
        cache: ConfigCache | None = None,
    ):
        self._template_config = config
        self._agent_config = agent_config
        self._llm_config: LlmAgentConfig | None = None

        if agent_config is not None:
            inner = agent_config.root
            if not isinstance(inner, LlmAgentConfig):
                raise TypeError(
                    f"TemplatedPydanticAIAgentExecutor requires LlmAgentConfig, "
                    f"got {type(inner).__name__}"
                )
            self._llm_config = inner
            agent = build_agent_from_config(agent_config)
        elif config is not None:
            # Legacy path: build from TemplateConfig
            agent = TemplatedAgentBuilder.from_config(config=config)
        else:
            raise ValueError("Either 'config' or 'agent_config' must be provided.")

        super().__init__(agent=agent, cache=cache)

    @property
    def config(self) -> TemplateConfig | None:
        return self._template_config

    @property
    def llm_config(self) -> LlmAgentConfig | None:
        return self._llm_config

    @property
    @override
    def instruction(self) -> str:
        """Return instruction from unified config or mounted file."""
        # Unified config instruction takes precedence
        if self._llm_config and self._llm_config.instruction:
            return self._llm_config.instruction

        # Fall back to file-based instruction
        instruction = super().instruction
        if instruction is None:
            raise ProviderNotConfiguredError("Instruction is required for templated agents")
        return instruction

    @property
    def _output_schema_name(self) -> str:
        if self._llm_config and self._llm_config.output_schema:
            return self._llm_config.output_schema.name
        if self._template_config:
            return self._template_config.output_schema.name
        return "result"

    @property
    def _output_schema_description(self) -> str | None:
        if self._llm_config and self._llm_config.output_schema:
            return self._llm_config.output_schema.description
        if self._template_config:
            return self._template_config.output_schema.description
        return None

    @override
    async def execute(self, context: RequestContext, event_queue: EventQueue) -> None:
        request = context.get_user_input()
        task = context.current_task
        if not task:
            task = new_task(context.message)
            await event_queue.enqueue_event(task)

        # Use model from unified config or fall back to mounted file
        if self._llm_config and self._llm_config.model:
            from flokoa.integrations.pydantic_ai.model_factory import create_model_from_config
            model = create_model_from_config(self._llm_config.model)
        elif self.model_config:
            model = self._create_model(self._create_provider(self.model_config.provider.type))
        else:
            raise ProviderNotConfiguredError("Model configuration is required for templated agents")

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
                    name=self._output_schema_name,
                    description=self._output_schema_description,
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
