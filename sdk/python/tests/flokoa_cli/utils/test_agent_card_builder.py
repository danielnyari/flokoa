import pytest

from flokoa.utils.agent_card_builder import AgentCardBuilder

pytestmark = pytest.mark.anyio


class DummyAgent:
    name = "dummy_agent"
    description = "Dummy agent for testing"


async def test_builds_generic_agent_card_from_agent_attributes():
    builder = AgentCardBuilder(agent=DummyAgent(), rpc_url="http://localhost:10001/")

    card = await builder.build()

    assert card.name == "dummy_agent"
    assert card.description == "Dummy agent for testing"
    assert card.url == "http://localhost:10001"
    assert len(card.skills) == 1
    skill = card.skills[0]
    assert skill.id == "dummy_agent"
    assert skill.name == "agent"
    assert skill.description == "Dummy agent for testing"


async def test_builds_agent_card_for_adk_llm_agent():
    adk_agents = pytest.importorskip("google.adk.agents")
    adk_tools = pytest.importorskip("google.adk.tools")

    def dummy_tool_function(a: int, b: int) -> int:
        return a + b

    tool = adk_tools.FunctionTool(func=dummy_tool_function)
    agent = adk_agents.LlmAgent(name="test_agent", model="gemini-2.0-flash", tools=[tool])

    builder = AgentCardBuilder(agent=agent, rpc_url="http://localhost:10001/")
    card = await builder.build()

    skill_names = {skill.name for skill in card.skills}
    assert "model" in skill_names
    assert "dummy_tool_function" in skill_names
