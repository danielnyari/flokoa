import pytest

from flokoa.utils.agent_card_builder import (
    AgentCardBuilder,
    _coerce_text,
    _get_agent_name,
    _get_generic_agent_description,
)

pytestmark = pytest.mark.anyio


# ---------------------------------------------------------------------------
# AgentCardBuilder — init validation
# ---------------------------------------------------------------------------


class TestAgentCardBuilderInit:
    def test_none_agent_raises(self):
        with pytest.raises(ValueError, match="Agent cannot be None"):
            AgentCardBuilder(agent=None)

    def test_defaults(self):
        class A:
            name = "a"

        builder = AgentCardBuilder(agent=A())
        assert builder._rpc_url == "http://localhost:80/a2a"
        assert builder._agent_version == "0.0.1"


# ---------------------------------------------------------------------------
# AgentCardBuilder — generic agents (non-PydanticAI)
# ---------------------------------------------------------------------------


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


async def test_generic_agent_without_name_uses_class_name():
    class UnnamedAgent:
        description = "No name"

    builder = AgentCardBuilder(agent=UnnamedAgent())
    card = await builder.build()
    assert card.name == "UnnamedAgent"


async def test_generic_agent_uses_system_prompt_as_description():
    class AgentWithPrompt:
        name = "prompter"
        system_prompt = "I help with math"

    builder = AgentCardBuilder(agent=AgentWithPrompt())
    card = await builder.build()
    assert card.description == "I help with math"


async def test_generic_agent_falls_back_to_default_description():
    class BareAgent:
        name = "bare"

    builder = AgentCardBuilder(agent=BareAgent())
    card = await builder.build()
    assert card.description == "A Flokoa agent"


# ---------------------------------------------------------------------------
# AgentCardBuilder — PydanticAI agents
# ---------------------------------------------------------------------------


async def test_builds_agent_card_for_pydantic_ai_agent():
    pydantic_ai = pytest.importorskip("pydantic_ai")
    agent = pydantic_ai.Agent(
        "openai:gpt-4o",
        defer_model_check=True,
        name="pydantic_agent",
        instructions="You are a helpful assistant",
    )

    builder = AgentCardBuilder(agent=agent, rpc_url="http://localhost:9000/")
    card = await builder.build()

    assert card.name == "pydantic_agent"
    # PydanticAI agent should get "llm" tag, "model" skill name
    assert len(card.skills) == 1
    assert card.skills[0].tags == ["llm"]
    assert card.skills[0].name == "model"
    # Description should be pulled from _instructions
    assert "You are a helpful assistant" in card.description


# ---------------------------------------------------------------------------
# _get_agent_name
# ---------------------------------------------------------------------------


class TestGetAgentName:
    def test_uses_name_attribute(self):
        class A:
            name = "my_agent"

        assert _get_agent_name(A()) == "my_agent"

    def test_uses_dunder_name(self):
        def my_func():
            pass

        assert _get_agent_name(my_func) == "my_func"

    def test_falls_back_to_class_name(self):
        class MySpecialAgent:
            pass

        assert _get_agent_name(MySpecialAgent()) == "MySpecialAgent"


# ---------------------------------------------------------------------------
# _coerce_text
# ---------------------------------------------------------------------------


class TestCoerceText:
    def test_string(self):
        assert _coerce_text("hello") == "hello"

    def test_empty_string(self):
        assert _coerce_text("") is None

    def test_whitespace_string(self):
        assert _coerce_text("   ") is None

    def test_list_of_strings(self):
        assert _coerce_text(["hello", "world"]) == "hello world"

    def test_empty_list(self):
        assert _coerce_text([]) is None

    def test_list_with_text_attr(self):
        class TextObj:
            text = "from attr"

        assert _coerce_text([TextObj()]) == "from attr"

    def test_list_with_non_string(self):
        assert _coerce_text([42]) == "42"

    def test_none(self):
        assert _coerce_text(None) is None

    def test_non_string_non_list(self):
        assert _coerce_text(42) is None

    def test_tuple_of_strings(self):
        assert _coerce_text(("a", "b")) == "a b"


# ---------------------------------------------------------------------------
# _get_generic_agent_description
# ---------------------------------------------------------------------------


class TestGetGenericAgentDescription:
    def test_uses_description(self):
        class A:
            description = "I am an agent"

        assert _get_generic_agent_description(A()) == "I am an agent"

    def test_uses_system_prompt(self):
        class A:
            description = None
            system_prompt = "System prompt text"

        assert _get_generic_agent_description(A()) == "System prompt text"

    def test_uses_instruction(self):
        class A:
            description = None
            system_prompt = None
            instruction = "Instruction text"

        assert _get_generic_agent_description(A()) == "Instruction text"

    def test_uses_private_instructions(self):
        class A:
            description = None
            system_prompt = None
            instruction = None
            _instructions = ["First instruction", "Second instruction"]

        assert "First instruction" in _get_generic_agent_description(A())

    def test_falls_back_to_default(self):
        class A:
            pass

        assert _get_generic_agent_description(A()) == "A Flokoa agent"
