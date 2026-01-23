from typing import override
from a2a.server.agent_execution.context import RequestContext
from a2a.server.events.event_queue import EventQueue
import pytest

from src.flokoa.agent_executor import FlokoaAgentExecutor


@pytest.fixture
def dummy_agent():
    class DummyAgent:
        def __init__(self):
            self._tools = ["tool1", "tool2"]

        def execute(self, command: str) -> str:
            return f"Executed: {command}"

        def add_tool(self, tool: str) -> None:
            self._tools.append(tool)

        @property
        def tools(self) -> list:
            return self._tools

    return DummyAgent()


@pytest.fixture
def dummy_agent_executor(dummy_agent):
    class DummyAgentExecutor(FlokoaAgentExecutor):
        @override
        async def execute(self, context: RequestContext, event_queue: EventQueue) -> None:
            return self.agent.execute(context.get_user_input())

        @override
        async def cancel(self, context: RequestContext, event_queue: EventQueue) -> None:
            raise Exception("cancel not supported")

        def add_tools(self) -> None:
            for tool in self.tool_definitions:
                self.agent.add_tool(tool.name)

    return DummyAgentExecutor(dummy_agent)
