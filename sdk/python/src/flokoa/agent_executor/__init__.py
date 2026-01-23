from typing import TYPE_CHECKING, Callable

from a2a.server.agent_execution import AgentExecutor

from flokoa.tools import TOOL_CALLABLES
from flokoa.types import ToolDefinition as FlokoaToolDefinition
from flokoa.utils import load_tools

if TYPE_CHECKING:
    from pydantic_ai import Agent


class FlokoaAgentExecutor(AgentExecutor):
    """Base class for Flokoa AgentExecutors."""

    def __init__(self, agent: "Agent"):
        self._agent = agent
        self._tool_definitions = load_tools()

    @property
    def tool_definitions(self) -> list[FlokoaToolDefinition]:
        return self._tool_definitions

    @property
    def agent(self) -> "Agent":
        return self._agent

    def _get_tool_callable(self, tool_definition: FlokoaToolDefinition) -> Callable:
        return TOOL_CALLABLES[tool_definition.type]
