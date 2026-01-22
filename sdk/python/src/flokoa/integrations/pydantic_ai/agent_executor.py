import logging
from typing import Callable, override

from pydantic_ai import Agent, FunctionToolset, Tool
from a2a.server.agent_execution import AgentExecutor, RequestContext
from a2a.server.events import EventQueue
from a2a.utils import new_data_artifact

from flokoa.utils import load_tools
from flokoa.tools import TOOL_CALLABLES
from flokoa.types import ToolDefinition as FlokoaToolDefinition

logger = logging.getLogger(__name__)

class PydanticAIAgentExecutor(AgentExecutor):
    """
    A2A AgentExecutor that wraps a PydanticAI agent with automatic 
    flokoa tool injection from /etc/flokoa/tools.json.
    """
    
    def __init__(self, agent: Agent):
        self._agent = agent
        self._tool_definitions = load_tools()
        self._function_toolset = FunctionToolset(tools=[])

    @property
    def tool_definitions(self) -> list[FlokoaToolDefinition]:
        return self._tool_definitions
    
    @property
    def agent(self) -> Agent:
        return self._agent
    
    def _get_tool_callable(self, tool_definition: FlokoaToolDefinition) -> Callable:
        return TOOL_CALLABLES[tool_definition.type]
    
    def _create_tool(self, tool_definition: FlokoaToolDefinition) -> Tool:
        tool_callable = self._get_tool_callable(tool_definition)

        tool = Tool.from_schema(
            function=tool_callable,
            name=tool_definition.name,
            description=tool_definition.description,
            json_schema=tool_definition.input_json_schema,
            takes_ctx=False,
            sequential=False
        )
        return tool
        

    def _add_tools_to_toolset(self):
        for tool_definition in self._tool_definitions:
            tool = self._create_tool(tool_definition)
            self._function_toolset.add_tool(tool)
            logger.info(f"Added tool '{tool_definition.name}' to agent toolset.")
    
    @override
    async def execute(self, context: RequestContext, event_queue: EventQueue) -> None:
        request = context.get_user_input()
        logger.info(f"Executing PydanticAI agent with request: {request}")
