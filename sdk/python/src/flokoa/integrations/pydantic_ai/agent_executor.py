import logging
from typing import Callable, override

from pydantic_ai import Agent, FunctionToolset, Tool
from a2a.server.agent_execution import AgentExecutor, RequestContext
from a2a.server.events import EventQueue
from a2a.utils import new_agent_text_message

from flokoa.agent_executor import FlokoaAgentExecutor
from flokoa.types import ToolDefinition as FlokoaToolDefinition

logger = logging.getLogger(__name__)


class PydanticAIAgentExecutor(FlokoaAgentExecutor):
    """
    A2A AgentExecutor that wraps a PydanticAI agent with automatic
    flokoa tool injection from /etc/flokoa/tools.json.
    """

    def _create_tool(self, tool_definition: FlokoaToolDefinition) -> Tool:
        tool_callable = self._get_tool_callable(tool_definition)

        tool = Tool.from_schema(
            function=tool_callable,
            name=tool_definition.name,
            description=tool_definition.description,
            json_schema=tool_definition.input_json_schema,
            takes_ctx=False,
            sequential=False,
        )
        return tool

    def _get_toolset(self) -> FunctionToolset:
        toolset = FunctionToolset()
        for tool_definition in self._tool_definitions:
            tool = self._create_tool(tool_definition)
            toolset.add_tool(tool)
            logger.info(f"Added tool '{tool_definition.name}' to agent toolset.")
        return toolset

    @override
    async def execute(self, context: RequestContext, event_queue: EventQueue) -> None:
        request = context.get_user_input()
        logger.info(f"Executing PydanticAI agent with request: {request}")
        result = await self.agent.run(request, toolsets=[self._get_toolset()])
        await event_queue.enqueue_event(new_agent_text_message(result.output))

    @override
    async def cancel(self, context: RequestContext, event_queue: EventQueue) -> None:
        raise Exception("cancel not supported")
