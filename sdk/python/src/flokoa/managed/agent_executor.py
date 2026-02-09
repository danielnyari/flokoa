import logging
from typing import override

from a2a.server.agent_execution import RequestContext
from a2a.server.events import EventQueue
from a2a.utils import new_agent_text_message
from pydantic import BaseModel

from flokoa.cache import ConfigCache
from flokoa.exceptions import ProviderNotConfiguredError
from flokoa.integrations.pydantic_ai.agent_executor import PydanticAIAgentExecutor
from flokoa.managed.agent import ManagedAgentBuilder
from flokoa.types import ManagedConfig

logger = logging.getLogger(__name__)


class ManagedPydanticAIAgentExecutor(PydanticAIAgentExecutor):
    """Agent executor for the managed pydantic-ai runtime.

    Unlike the integration executor which wraps a user-provided agent,
    this executor builds the agent from operator-mounted configuration:
    - Instruction from /etc/flokoa/instruction.txt
    - Output schema from /etc/flokoa/managed-config.json
    - Model config from /etc/flokoa/model.json
    - Tools from /etc/flokoa/tools/

    The pydantic-ai agent is generated via ManagedAgentBuilder.
    """

    def __init__(
        self,
        builder: ManagedAgentBuilder,
        managed_config: ManagedConfig | None = None,
        cache: ConfigCache | None = None,
    ):
        agent = builder.build()
        super().__init__(agent=agent, cache=cache)
        self._builder = builder
        self._managed_config = managed_config

    @property
    def managed_config(self) -> ManagedConfig | None:
        return self._managed_config

    @override
    async def execute(self, context: RequestContext, event_queue: EventQueue) -> None:
        request = context.get_user_input()
        logger.info("Executing managed PydanticAI agent with request: %s", request)

        if not self.model_config:
            raise ProviderNotConfiguredError("Model configuration is required for managed agents")

        model = self._create_model(self._create_provider(self.model_config.provider.type))

        result = await self.agent.run(
            request,
            toolsets=[self._get_toolset()],
            model=model,
        )

        if isinstance(result.output, BaseModel):
            await event_queue.enqueue_event(new_agent_text_message(result.output.model_dump_json()))
        else:
            await event_queue.enqueue_event(new_agent_text_message(str(result.output)))
