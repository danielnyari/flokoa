import logging
from typing import Any, override

from a2a.server.agent_execution import RequestContext
from a2a.server.events import EventQueue
from a2a.utils import new_agent_text_message
from pydantic_ai import Agent

from flokoa.cache import ConfigCache
from flokoa.exceptions import ProviderNotConfiguredError
from flokoa.integrations.pydantic_ai.agent_executor import PydanticAIAgentExecutor
from flokoa.templated.agent import TemplatedAgentBuilder

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
        builder: TemplatedAgentBuilder,
        instruction: str,
        cache: ConfigCache | None = None,
    ):
        agent: Agent[None, str] = Agent()
        super().__init__(agent=agent, cache=cache)
        self._builder = builder
        self._instruction = instruction

    @property
    def builder(self) -> TemplatedAgentBuilder:
        return self._builder

    @property
    @override
    def instruction(self) -> str:
        """Templated agents always use the instruction passed at construction."""
        return self._instruction

    @override
    async def execute(self, context: RequestContext, event_queue: EventQueue) -> None:
        request = context.get_user_input()
        logger.info("Executing templated PydanticAI agent with request: %s", request)

        if not self.model_config:
            raise ProviderNotConfiguredError("Model configuration is required for templated agents")

        model = self._create_model(self._create_provider(self.model_config.provider.type))

        run_kwargs: dict[str, Any] = {
            "toolsets": [self._get_toolset()],
            "model": model,
            "instructions": self.instruction,
        }

        result = await self.agent.run(request, **run_kwargs)
        await event_queue.enqueue_event(new_agent_text_message(str(result.output)))
