from __future__ import annotations

import logging
import re
from typing import Any, Dict, List, Optional

from a2a.types import AgentCapabilities, AgentCard, AgentSkill

logger = logging.getLogger(__name__)

_PRONOUN_MAP = {
    "you are": "I am",
    "you were": "I was",
    "you're": "I am",
    "you've": "I have",
    "yours": "mine",
    "your": "my",
    "you": "I",
}
_PRONOUN_KEYS = sorted(_PRONOUN_MAP.keys(), key=len, reverse=True)
_PRONOUN_PATTERN = re.compile(r"\b(" + "|".join(re.escape(key) for key in _PRONOUN_KEYS) + r")\b", re.IGNORECASE)

try:  # pragma: no cover - optional dependency
    from google.adk.agents import BaseAgent, LlmAgent, LoopAgent, ParallelAgent, SequentialAgent
    from google.adk.tools.example_tool import ExampleTool

    _ADK_AVAILABLE = True
except ImportError:  # pragma: no cover - optional dependency
    BaseAgent = None  # type: ignore[assignment]
    LlmAgent = None  # type: ignore[assignment]
    LoopAgent = None  # type: ignore[assignment]
    ParallelAgent = None  # type: ignore[assignment]
    SequentialAgent = None  # type: ignore[assignment]
    ExampleTool = None  # type: ignore[assignment]
    _ADK_AVAILABLE = False

try:  # pragma: no cover - optional dependency
    from pydantic_ai import Agent as PydanticAIAgent

    _PYDANTIC_AI_AVAILABLE = True
except ImportError:  # pragma: no cover - optional dependency
    PydanticAIAgent = None  # type: ignore[assignment]
    _PYDANTIC_AI_AVAILABLE = False


class AgentCardBuilder:
    """Builder for creating agent cards from Flokoa agent instances."""

    def __init__(
        self,
        *,
        agent: Any,
        rpc_url: Optional[str] = None,
        capabilities: Optional[AgentCapabilities] = None,
        agent_version: Optional[str] = None,
    ) -> None:
        if agent is None:
            raise ValueError("Agent cannot be None.")

        self._agent = agent
        self._rpc_url = rpc_url or "http://localhost:80/a2a"
        self._capabilities = capabilities or AgentCapabilities()
        self._agent_version = agent_version or "0.0.1"

    async def build(self) -> AgentCard:
        """Build and return the complete agent card."""
        agent_name = _get_agent_name(self._agent)
        try:
            if _is_adk_agent(self._agent):
                primary_skills = await _build_primary_skills(self._agent)
                sub_agent_skills = await _build_sub_agent_skills(self._agent)
                all_skills = primary_skills + sub_agent_skills

                return AgentCard(
                    name=agent_name,
                    description=getattr(self._agent, "description", None) or "An ADK Agent",
                    url=self._rpc_url.rstrip("/"),
                    version=self._agent_version,
                    capabilities=self._capabilities,
                    skills=all_skills,
                    default_input_modes=["text/plain"],
                    default_output_modes=["text/plain"],
                )

            return _build_generic_agent_card(
                agent=self._agent,
                agent_name=agent_name,
                rpc_url=self._rpc_url,
                capabilities=self._capabilities,
                agent_version=self._agent_version,
            )
        except Exception as exc:  # pragma: no cover - defensive
            raise RuntimeError(f"Failed to build agent card for {agent_name}: {exc}") from exc


def _is_adk_agent(agent: Any) -> bool:
    return _ADK_AVAILABLE and BaseAgent is not None and isinstance(agent, BaseAgent)


async def _build_primary_skills(agent: Any) -> List[AgentSkill]:
    """Build skills for any agent type."""
    if _is_llm_agent(agent):
        return await _build_llm_agent_skills(agent)
    return await _build_non_llm_agent_skills(agent)


async def _build_llm_agent_skills(agent: Any) -> List[AgentSkill]:
    """Build skills for LLM agent."""
    skills = []
    agent_name = _get_agent_name(agent)

    agent_description = _build_llm_agent_description_with_instructions(agent)
    agent_examples = await _extract_examples_from_agent(agent)

    skills.append(
        AgentSkill(
            id=agent_name,
            name="model",
            description=agent_description,
            examples=_extract_inputs_from_examples(agent_examples),
            input_modes=_get_input_modes(agent),
            output_modes=_get_output_modes(agent),
            tags=["llm"],
        )
    )

    if getattr(agent, "tools", None):
        tool_skills = await _build_tool_skills(agent)
        skills.extend(tool_skills)

    if getattr(agent, "planner", None):
        skills.append(_build_planner_skill(agent))

    if getattr(agent, "code_executor", None):
        skills.append(_build_code_executor_skill(agent))

    return skills


async def _build_sub_agent_skills(agent: Any) -> List[AgentSkill]:
    """Build skills for all sub-agents."""
    sub_agent_skills: list[AgentSkill] = []
    for sub_agent in _get_sub_agents(agent):
        try:
            sub_skills = await _build_primary_skills(sub_agent)
            for skill in sub_skills:
                sub_agent_name = _get_agent_name(sub_agent)
                aggregated_skill = AgentSkill(
                    id=f"{sub_agent_name}_{skill.id}",
                    name=f"{sub_agent_name}: {skill.name}",
                    description=skill.description,
                    examples=skill.examples,
                    input_modes=skill.input_modes,
                    output_modes=skill.output_modes,
                    tags=[f"sub_agent:{sub_agent_name}"] + (skill.tags or []),
                )
                sub_agent_skills.append(aggregated_skill)
        except Exception as exc:  # pragma: no cover - defensive
            logger.warning("Failed to build skills for sub-agent %s: %s", sub_agent, exc)
            continue

    return sub_agent_skills


async def _build_tool_skills(agent: Any) -> List[AgentSkill]:
    """Build skills for agent tools."""
    tool_skills: list[AgentSkill] = []
    canonical_tools = await agent.canonical_tools()

    for tool in canonical_tools or []:
        if ExampleTool is not None and isinstance(tool, ExampleTool):
            continue

        tool_name = getattr(tool, "name", None) or tool.__class__.__name__

        tool_skills.append(
            AgentSkill(
                id=f"{_get_agent_name(agent)}-{tool_name}",
                name=tool_name,
                description=getattr(tool, "description", f"Tool: {tool_name}"),
                examples=None,
                input_modes=None,
                output_modes=None,
                tags=["llm", "tools"],
            )
        )

    return tool_skills


def _build_planner_skill(agent: Any) -> AgentSkill:
    """Build planner skill for LLM agent."""
    return AgentSkill(
        id=f"{_get_agent_name(agent)}-planner",
        name="planning",
        description="Can think about the tasks to do and make plans",
        examples=None,
        input_modes=None,
        output_modes=None,
        tags=["llm", "planning"],
    )


def _build_code_executor_skill(agent: Any) -> AgentSkill:
    """Build code executor skill for LLM agent."""
    return AgentSkill(
        id=f"{_get_agent_name(agent)}-code-executor",
        name="code-execution",
        description="Can execute code",
        examples=None,
        input_modes=None,
        output_modes=None,
        tags=["llm", "code_execution"],
    )


async def _build_non_llm_agent_skills(agent: Any) -> List[AgentSkill]:
    """Build skills for non-LLM agents."""
    agent_description = _build_agent_description(agent)
    agent_examples = await _extract_examples_from_agent(agent)

    agent_type = _get_agent_type(agent)
    agent_name = _get_agent_name(agent)
    agent_skill_name = _get_agent_skill_name(agent)

    skills = [
        AgentSkill(
            id=agent_name,
            name=agent_skill_name,
            description=agent_description,
            examples=_extract_inputs_from_examples(agent_examples),
            input_modes=_get_input_modes(agent),
            output_modes=_get_output_modes(agent),
            tags=[agent_type],
        )
    ]

    if _get_sub_agents(agent):
        orchestration_skill = _build_orchestration_skill(agent, agent_type)
        if orchestration_skill:
            skills.append(orchestration_skill)

    return skills


def _build_orchestration_skill(agent: Any, agent_type: str) -> Optional[AgentSkill]:
    """Build orchestration skill for agents with sub-agents."""
    sub_agent_descriptions = []
    for sub_agent in _get_sub_agents(agent):
        description = getattr(sub_agent, "description", None) or "No description"
        sub_agent_descriptions.append(f"{_get_agent_name(sub_agent)}: {description}")

    if not sub_agent_descriptions:
        return None

    return AgentSkill(
        id=f"{_get_agent_name(agent)}-sub-agents",
        name="sub-agents",
        description="Orchestrates: " + "; ".join(sub_agent_descriptions),
        examples=None,
        input_modes=None,
        output_modes=None,
        tags=[agent_type, "orchestration"],
    )


def _get_agent_type(agent: Any) -> str:
    """Get the agent type for tagging."""
    if _is_llm_agent(agent):
        return "llm"
    if _ADK_AVAILABLE and SequentialAgent is not None and isinstance(agent, SequentialAgent):
        return "sequential_workflow"
    if _ADK_AVAILABLE and ParallelAgent is not None and isinstance(agent, ParallelAgent):
        return "parallel_workflow"
    if _ADK_AVAILABLE and LoopAgent is not None and isinstance(agent, LoopAgent):
        return "loop_workflow"
    return "custom_agent"


def _get_agent_skill_name(agent: Any) -> str:
    """Get the skill name based on agent type."""
    if _is_llm_agent(agent):
        return "model"
    if _ADK_AVAILABLE and (
        (SequentialAgent is not None and isinstance(agent, SequentialAgent))
        or (ParallelAgent is not None and isinstance(agent, ParallelAgent))
        or (LoopAgent is not None and isinstance(agent, LoopAgent))
    ):
        return "workflow"
    return "custom"


def _build_agent_description(agent: Any) -> str:
    """Build agent description from agent.description and workflow-specific descriptions."""
    description_parts = []

    agent_description = getattr(agent, "description", None)
    if agent_description:
        description_parts.append(agent_description)

    if not _is_llm_agent(agent):
        workflow_description = _get_workflow_description(agent)
        if workflow_description:
            description_parts.append(workflow_description)

    return " ".join(description_parts) if description_parts else _get_default_description(agent)


def _build_llm_agent_description_with_instructions(agent: Any) -> str:
    """Build agent description including instructions for LLM agents."""
    description_parts = []

    agent_description = getattr(agent, "description", None)
    if agent_description:
        description_parts.append(agent_description)

    instruction = getattr(agent, "instruction", None)
    if instruction:
        description_parts.append(_replace_pronouns(instruction))

    global_instruction = getattr(agent, "global_instruction", None)
    if global_instruction:
        description_parts.append(_replace_pronouns(global_instruction))

    return " ".join(description_parts) if description_parts else _get_default_description(agent)


def _replace_pronouns(text: str) -> str:
    """Replace pronouns and conjugate common verbs for agent description."""
    def _replacement(match: re.Match[str]) -> str:
        replacement = _PRONOUN_MAP[match.group(1).lower()]
        if match.group(0)[0].isupper():
            return replacement[:1].upper() + replacement[1:]
        return replacement

    return _PRONOUN_PATTERN.sub(_replacement, text)


def _get_workflow_description(agent: Any) -> Optional[str]:
    """Get workflow-specific description for non-LLM agents."""
    if not _get_sub_agents(agent):
        return None

    if _ADK_AVAILABLE and SequentialAgent is not None and isinstance(agent, SequentialAgent):
        return _build_sequential_description(agent)
    if _ADK_AVAILABLE and ParallelAgent is not None and isinstance(agent, ParallelAgent):
        return _build_parallel_description(agent)
    if _ADK_AVAILABLE and LoopAgent is not None and isinstance(agent, LoopAgent):
        return _build_loop_description(agent)

    return None


def _build_sequential_description(agent: Any) -> str:
    """Build description for sequential workflow agent."""
    descriptions = []
    sub_agents = _get_sub_agents(agent)
    for i, sub_agent in enumerate(sub_agents, 1):
        sub_description = getattr(sub_agent, "description", None) or f"execute the {_get_agent_name(sub_agent)} agent"
        if i == 1:
            descriptions.append(f"First, this agent will {sub_description}")
        elif i == len(sub_agents):
            descriptions.append(f"Finally, this agent will {sub_description}")
        else:
            descriptions.append(f"Then, this agent will {sub_description}")
    return " ".join(descriptions) + "."


def _build_parallel_description(agent: Any) -> str:
    """Build description for parallel workflow agent."""
    descriptions = []
    sub_agents = _get_sub_agents(agent)
    for i, sub_agent in enumerate(sub_agents):
        sub_description = getattr(sub_agent, "description", None) or f"execute the {_get_agent_name(sub_agent)} agent"
        if i == 0:
            descriptions.append(f"This agent will {sub_description}")
        elif i == len(sub_agents) - 1:
            descriptions.append(f"and {sub_description}")
        else:
            descriptions.append(f", {sub_description}")
    return " ".join(descriptions) + " simultaneously."


def _build_loop_description(agent: Any) -> str:
    """Build description for loop workflow agent."""
    descriptions = []
    sub_agents = _get_sub_agents(agent)
    for i, sub_agent in enumerate(sub_agents):
        sub_description = getattr(sub_agent, "description", None) or f"execute the {_get_agent_name(sub_agent)} agent"
        if i == 0:
            descriptions.append(f"This agent will {sub_description}")
        elif i == len(sub_agents) - 1:
            descriptions.append(f"and {sub_description}")
        else:
            descriptions.append(f", {sub_description}")
    description_text = " ".join(descriptions)
    max_iterations = getattr(agent, "max_iterations", None)
    if max_iterations is None:
        return f"{description_text} in a loop (no iteration limit)."
    return f"{description_text} in a loop (max {max_iterations} iterations)."


def _get_default_description(agent: Any) -> str:
    """Get default description based on agent type."""
    if _is_llm_agent(agent):
        return "An LLM-based agent"
    if _ADK_AVAILABLE and SequentialAgent is not None and isinstance(agent, SequentialAgent):
        return "A sequential workflow agent"
    if _ADK_AVAILABLE and ParallelAgent is not None and isinstance(agent, ParallelAgent):
        return "A parallel workflow agent"
    if _ADK_AVAILABLE and LoopAgent is not None and isinstance(agent, LoopAgent):
        return "A loop workflow agent"

    return "A custom agent"


def _extract_inputs_from_examples(examples: Optional[list[dict]]) -> list[str]:
    """Extract only the input strings so they can be added to an AgentSkill."""
    if examples is None:
        return []

    extracted_inputs: list[str] = []
    for example in examples:
        example_input = example.get("input")
        if not example_input:
            continue

        if not isinstance(example_input, dict):
            continue

        parts = example_input.get("parts")
        if parts is not None:
            part_texts = [part.get("text") for part in parts if part.get("text") is not None]
            if part_texts:
                extracted_inputs.append("\n".join(part_texts))
            continue

        text = example_input.get("text")
        if text is not None:
            extracted_inputs.append(text)

    return extracted_inputs


async def _extract_examples_from_agent(agent: Any) -> Optional[List[Dict]]:
    """Extract examples from example_tool if configured; otherwise, from agent instruction."""
    if not _is_llm_agent(agent):
        return None

    try:
        canonical_tools = await agent.canonical_tools()
        for tool in canonical_tools or []:
            if ExampleTool is not None and isinstance(tool, ExampleTool):
                return _convert_example_tool_examples(tool)
    except Exception as exc:
        logger.warning("Failed to extract examples from tools: %s", exc)

    instruction = getattr(agent, "instruction", None)
    if instruction:
        return _extract_examples_from_instruction(instruction)

    return None


def _convert_example_tool_examples(tool: Any) -> List[Dict]:
    """Convert ExampleTool examples to the expected format."""
    examples = []
    for example in getattr(tool, "examples", []):
        examples.append(
            {
                "input": example.input.model_dump() if hasattr(example.input, "model_dump") else example.input,
                "output": [
                    output.model_dump() if hasattr(output, "model_dump") else output for output in example.output
                ],
            }
        )
    return examples


def _extract_examples_from_instruction(instruction: str) -> Optional[List[Dict]]:
    """Extract examples from agent instruction text using regex patterns."""
    examples = []
    example_patterns = [
        r'Example Query:\s*["\']((?:[^"\'\\\\]|\\\\.)+)["\']\s*Example Response:\s*["\']((?:[^"\'\\\\]|\\\\.)+)["\']',
        r'Example:\s*["\']((?:[^"\'\\\\]|\\\\.)+)["\']\s*Response:\s*["\']((?:[^"\'\\\\]|\\\\.)+)["\']',
    ]

    for pattern in example_patterns:
        matches = re.findall(pattern, instruction, re.IGNORECASE | re.DOTALL)
        for query, response in matches:
            examples.append({"input": {"text": query}, "output": [{"text": response}]})

    return examples if examples else None


def _get_input_modes(agent: Any) -> Optional[List[str]]:
    """Get input modes based on agent model."""
    if not _is_llm_agent(agent):
        return None

    input_modes = getattr(agent, "input_modes", None) or getattr(agent, "input_modalities", None)
    return input_modes


def _get_output_modes(agent: Any) -> Optional[List[str]]:
    """Get output modes from agent configuration."""
    if not _is_llm_agent(agent):
        return None

    generate_config = getattr(agent, "generate_content_config", None)
    response_modalities = getattr(generate_config, "response_modalities", None)
    return response_modalities


def _get_agent_name(agent: Any) -> str:
    """Resolve an agent's display name."""
    name = getattr(agent, "name", None)
    if name:
        return str(name)
    name = getattr(agent, "__name__", None)
    if name:
        return str(name)
    return agent.__class__.__name__


def _get_sub_agents(agent: Any) -> list[Any]:
    sub_agents = getattr(agent, "sub_agents", None) or []
    return sub_agents if isinstance(sub_agents, list) else list(sub_agents)


def _is_llm_agent(agent: Any) -> bool:
    return _ADK_AVAILABLE and LlmAgent is not None and isinstance(agent, LlmAgent)


def _is_pydantic_ai_agent(agent: Any) -> bool:
    return _PYDANTIC_AI_AVAILABLE and PydanticAIAgent is not None and isinstance(agent, PydanticAIAgent)


def _get_generic_agent_description(agent: Any) -> str:
    description = _coerce_text(getattr(agent, "description", None))
    if description:
        return description

    description = _coerce_text(getattr(agent, "system_prompt", None))
    if description:
        return description

    description = _coerce_text(getattr(agent, "instruction", None))
    if description:
        return description

    # pydantic-ai stores instructions in _instructions (list of strings)
    description = _coerce_text(getattr(agent, "_instructions", None))
    if description:
        return description

    return "A Flokoa agent"


def _coerce_text(value: Any) -> Optional[str]:
    if isinstance(value, str):
        return value.strip() or None
    if isinstance(value, (list, tuple)):
        parts = []
        for part in value:
            if isinstance(part, str):
                part_text = part.strip()
            elif hasattr(part, "text"):
                part_text = str(part.text).strip()
            else:
                part_text = str(part).strip()
            if part_text:
                parts.append(part_text)
        return " ".join(parts) if parts else None
    return None


def _build_generic_agent_card(
    *,
    agent: Any,
    agent_name: str,
    rpc_url: str,
    capabilities: AgentCapabilities,
    agent_version: str,
) -> AgentCard:
    description = _get_generic_agent_description(agent)
    tag = "llm" if _is_pydantic_ai_agent(agent) else "custom_agent"
    skill_name = "model" if tag == "llm" else "agent"

    skills = [
        AgentSkill(
            id=agent_name,
            name=skill_name,
            description=description,
            tags=[tag],
            examples=None,
            input_modes=None,
            output_modes=None,
        )
    ]

    return AgentCard(
        name=agent_name,
        description=description,
        url=rpc_url.rstrip("/"),
        version=agent_version,
        capabilities=capabilities,
        skills=skills,
        default_input_modes=["text/plain"],
        default_output_modes=["text/plain"],
    )
