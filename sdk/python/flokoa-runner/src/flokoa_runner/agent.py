"""Stage: build_agent — hydrate the compiled spec into a pydantic-ai Agent."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

from flokoa_runner.errors import BootstrapError
from flokoa_runner.platform_capabilities import platform_capability_types

if TYPE_CHECKING:
    from pydantic_ai import Agent
    from pydantic_ai.capabilities.abstract import AbstractCapability


def build_agent(
    doc: dict[str, Any],
    capability_types: list[type[AbstractCapability[Any]]] | None = None,
) -> Agent[Any, Any]:
    """Construct the agent from the resolved spec document.

    Custom capability types are the platform capabilities (runner baseline)
    plus the entrypoint classes installed from capability wheelhouses. Native
    and platform entries hydrate by construction (the AgentSpec schema was
    generated with them); Capability-CR entries hydrate only if their
    wheelhouse delivered a class registered under the compiled entry name —
    the operator's admission checks (digest pin, requires tuple, entry-name
    uniqueness) keep that contract, but the class itself is resolved here.
    """
    from pydantic_ai import Agent
    from pydantic_ai.agent.spec import AgentSpec

    custom_types = platform_capability_types() + list(capability_types or [])

    try:
        spec = AgentSpec.from_dict(doc)
        return Agent.from_spec(spec, custom_capability_types=custom_types)
    except Exception as exc:
        # A schema-valid spec should always hydrate; reaching this means an
        # environment problem (e.g. a provider SDK missing its API key env).
        raise BootstrapError("build_agent", f"{type(exc).__name__}: {exc}") from exc
