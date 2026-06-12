from __future__ import annotations

import logging
from typing import Any

from a2a.types import AgentCapabilities, AgentCard, AgentSkill

logger = logging.getLogger(__name__)

try:  # pragma: no cover - optional dependency
    from pydantic_ai import Agent as _PydanticAgent

    PydanticAIAgent: type[Any] | None = _PydanticAgent
    _PYDANTIC_AI_AVAILABLE = True
except ImportError:  # pragma: no cover - optional dependency
    PydanticAIAgent = None
    _PYDANTIC_AI_AVAILABLE = False


class AgentCardBuilder:
    """Builder for creating agent cards from Flokoa agent instances."""

    def __init__(
        self,
        *,
        agent: Any,
        rpc_url: str | None = None,
        capabilities: AgentCapabilities | None = None,
        agent_version: str | None = None,
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
            return _build_generic_agent_card(
                agent=self._agent,
                agent_name=agent_name,
                rpc_url=self._rpc_url,
                capabilities=self._capabilities,
                agent_version=self._agent_version,
            )
        except Exception as exc:  # pragma: no cover - defensive
            raise RuntimeError(f"Failed to build agent card for {agent_name}: {exc}") from exc


def _get_agent_name(agent: Any) -> str:
    """Resolve an agent's display name."""
    name = getattr(agent, "name", None)
    if name:
        return str(name)
    name = getattr(agent, "__name__", None)
    if name:
        return str(name)
    return agent.__class__.__name__


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


def _coerce_text(value: Any) -> str | None:
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
