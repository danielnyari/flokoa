import pytest

from flokoa.utils.agent_card_builder import (
    AgentCardBuilder,
    _build_orchestration_skill,
    _coerce_text,
    _extract_examples_from_instruction,
    _extract_inputs_from_examples,
    _get_agent_name,
    _get_generic_agent_description,
    _replace_pronouns,
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
# AgentCardBuilder — generic agents (non-ADK, non-PydanticAI)
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
# AgentCardBuilder — ADK agents
# ---------------------------------------------------------------------------


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


async def test_adk_llm_agent_with_description_and_instruction():
    adk_agents = pytest.importorskip("google.adk.agents")

    agent = adk_agents.LlmAgent(
        name="described_agent",
        model="gemini-2.0-flash",
        description="A smart agent",
        instruction="You help with tasks",
    )

    builder = AgentCardBuilder(agent=agent)
    card = await builder.build()

    assert card.description == "A smart agent"
    # The model skill description should include both description and instruction (pronoun-replaced)
    model_skill = next(s for s in card.skills if s.name == "model")
    assert "A smart agent" in model_skill.description
    assert "I help with tasks" in model_skill.description


async def test_adk_llm_agent_with_planner():
    adk_agents = pytest.importorskip("google.adk.agents")
    planners = pytest.importorskip("google.adk.planners")

    agent = adk_agents.LlmAgent(
        name="planning_agent",
        model="gemini-2.0-flash",
        planner=planners.PlanReActPlanner(),
    )

    builder = AgentCardBuilder(agent=agent)
    card = await builder.build()

    skill_names = {s.name for s in card.skills}
    assert "planning" in skill_names


async def test_adk_llm_agent_with_code_executor():
    adk_agents = pytest.importorskip("google.adk.agents")
    code_executors = pytest.importorskip("google.adk.code_executors")

    agent = adk_agents.LlmAgent(
        name="coding_agent",
        model="gemini-2.0-flash",
        code_executor=code_executors.BuiltInCodeExecutor(),
    )

    builder = AgentCardBuilder(agent=agent)
    card = await builder.build()

    skill_names = {s.name for s in card.skills}
    assert "code-execution" in skill_names


async def test_adk_llm_agent_with_example_tool():
    adk_agents = pytest.importorskip("google.adk.agents")
    example_tool_mod = pytest.importorskip("google.adk.tools.example_tool")
    genai_types = pytest.importorskip("google.genai.types")

    example = example_tool_mod.Example(
        input=genai_types.Content(parts=[genai_types.Part(text="What is 2+2?")]),
        output=[genai_types.Content(parts=[genai_types.Part(text="4")])],
    )
    example_tool = example_tool_mod.ExampleTool(examples=[example])

    agent = adk_agents.LlmAgent(
        name="example_agent",
        model="gemini-2.0-flash",
        tools=[example_tool],
    )

    builder = AgentCardBuilder(agent=agent)
    card = await builder.build()

    # ExampleTool should NOT appear as a separate tool skill
    skill_names = {s.name for s in card.skills}
    assert "ExampleTool" not in skill_names
    # But examples should be extracted into the model skill
    model_skill = next(s for s in card.skills if s.name == "model")
    assert "What is 2+2?" in (model_skill.examples or [])


async def test_adk_sequential_agent():
    adk_agents = pytest.importorskip("google.adk.agents")

    sub1 = adk_agents.LlmAgent(name="step1", model="gemini-2.0-flash", description="fetch data")
    sub2 = adk_agents.LlmAgent(name="step2", model="gemini-2.0-flash", description="process data")

    agent = adk_agents.SequentialAgent(
        name="seq_agent",
        description="A sequential workflow",
        sub_agents=[sub1, sub2],
    )

    builder = AgentCardBuilder(agent=agent)
    card = await builder.build()

    assert card.name == "seq_agent"
    # Should have workflow skill + sub-agent skills
    skill_ids = {s.id for s in card.skills}
    assert "seq_agent" in skill_ids

    # Check that workflow description contains sequential language
    workflow_skill = next(s for s in card.skills if s.id == "seq_agent")
    assert workflow_skill.name == "workflow"
    assert "sequential_workflow" in workflow_skill.tags

    # Check orchestration skill exists
    orchestration_skills = [s for s in card.skills if "orchestration" in (s.tags or [])]
    assert len(orchestration_skills) == 1
    assert "step1" in orchestration_skills[0].description
    assert "step2" in orchestration_skills[0].description


async def test_adk_parallel_agent():
    adk_agents = pytest.importorskip("google.adk.agents")

    sub1 = adk_agents.LlmAgent(name="worker1", model="gemini-2.0-flash", description="task A")
    sub2 = adk_agents.LlmAgent(name="worker2", model="gemini-2.0-flash", description="task B")

    agent = adk_agents.ParallelAgent(
        name="par_agent",
        description="Run tasks in parallel",
        sub_agents=[sub1, sub2],
    )

    builder = AgentCardBuilder(agent=agent)
    card = await builder.build()

    workflow_skill = next(s for s in card.skills if s.id == "par_agent")
    assert "parallel_workflow" in workflow_skill.tags
    assert "simultaneously" in workflow_skill.description


async def test_adk_loop_agent():
    adk_agents = pytest.importorskip("google.adk.agents")

    sub = adk_agents.LlmAgent(name="checker", model="gemini-2.0-flash", description="check quality")

    agent = adk_agents.LoopAgent(
        name="loop_agent",
        description="Repeat until done",
        sub_agents=[sub],
        max_iterations=5,
    )

    builder = AgentCardBuilder(agent=agent)
    card = await builder.build()

    workflow_skill = next(s for s in card.skills if s.id == "loop_agent")
    assert "loop_workflow" in workflow_skill.tags
    assert "max 5 iterations" in workflow_skill.description


async def test_adk_loop_agent_no_max_iterations():
    adk_agents = pytest.importorskip("google.adk.agents")

    sub = adk_agents.LlmAgent(name="checker", model="gemini-2.0-flash")

    agent = adk_agents.LoopAgent(
        name="infinite_loop",
        sub_agents=[sub],
    )

    builder = AgentCardBuilder(agent=agent)
    card = await builder.build()

    workflow_skill = next(s for s in card.skills if s.id == "infinite_loop")
    assert "no iteration limit" in workflow_skill.description


async def test_sub_agent_skills_are_prefixed():
    adk_agents = pytest.importorskip("google.adk.agents")

    sub = adk_agents.LlmAgent(name="helper", model="gemini-2.0-flash", description="helps out")

    agent = adk_agents.SequentialAgent(
        name="parent",
        sub_agents=[sub],
    )

    builder = AgentCardBuilder(agent=agent)
    card = await builder.build()

    # Sub-agent skills should be prefixed with sub-agent name
    sub_skills = [s for s in card.skills if "sub_agent:helper" in (s.tags or [])]
    assert len(sub_skills) >= 1
    assert sub_skills[0].id.startswith("helper_")


# ---------------------------------------------------------------------------
# _replace_pronouns
# ---------------------------------------------------------------------------


class TestReplacePronouns:
    def test_basic_replacement(self):
        assert "I am" in _replace_pronouns("You are a helpful assistant")

    def test_your_to_my(self):
        assert "my task" in _replace_pronouns("your task")

    def test_preserves_capitalization(self):
        result = _replace_pronouns("You are smart")
        assert result.startswith("I am")

    def test_no_pronouns_unchanged(self):
        text = "This agent processes data"
        assert _replace_pronouns(text) == text


# ---------------------------------------------------------------------------
# _extract_inputs_from_examples
# ---------------------------------------------------------------------------


class TestExtractInputsFromExamples:
    def test_none_returns_empty(self):
        assert _extract_inputs_from_examples(None) == []

    def test_empty_list_returns_empty(self):
        assert _extract_inputs_from_examples([]) == []

    def test_extracts_text_input(self):
        examples = [{"input": {"text": "hello"}}]
        assert _extract_inputs_from_examples(examples) == ["hello"]

    def test_extracts_parts_input(self):
        examples = [{"input": {"parts": [{"text": "part1"}, {"text": "part2"}]}}]
        result = _extract_inputs_from_examples(examples)
        assert len(result) == 1
        assert "part1" in result[0]
        assert "part2" in result[0]

    def test_skips_non_dict_input(self):
        examples = [{"input": "just a string"}]
        assert _extract_inputs_from_examples(examples) == []

    def test_skips_missing_input(self):
        examples = [{"output": "something"}]
        assert _extract_inputs_from_examples(examples) == []

    def test_skips_empty_parts(self):
        examples = [{"input": {"parts": []}}]
        assert _extract_inputs_from_examples(examples) == []


# ---------------------------------------------------------------------------
# _extract_examples_from_instruction
# ---------------------------------------------------------------------------


class TestExtractExamplesFromInstruction:
    def test_extracts_example_query_response(self):
        instruction = 'Example Query: "What is Python?" Example Response: "A programming language"'
        result = _extract_examples_from_instruction(instruction)
        assert result is not None
        assert len(result) == 1
        assert result[0]["input"]["text"] == "What is Python?"
        assert result[0]["output"][0]["text"] == "A programming language"

    def test_extracts_example_response_pattern(self):
        instruction = 'Example: "Hello" Response: "Hi there"'
        result = _extract_examples_from_instruction(instruction)
        assert result is not None
        assert len(result) == 1

    def test_no_examples_returns_none(self):
        result = _extract_examples_from_instruction("Just a regular instruction with no examples")
        assert result is None


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


# ---------------------------------------------------------------------------
# _build_orchestration_skill
# ---------------------------------------------------------------------------


class TestBuildOrchestrationSkill:
    def test_returns_none_when_no_sub_agents(self):
        class A:
            name = "a"
            sub_agents = []

        assert _build_orchestration_skill(A(), "custom") is None

    def test_builds_skill_with_sub_agents(self):
        class Sub:
            name = "sub1"
            description = "Does stuff"

        class A:
            name = "parent"
            sub_agents = [Sub()]

        skill = _build_orchestration_skill(A(), "workflow")
        assert skill is not None
        assert "sub1" in skill.description
        assert "Does stuff" in skill.description
        assert "orchestration" in skill.tags
