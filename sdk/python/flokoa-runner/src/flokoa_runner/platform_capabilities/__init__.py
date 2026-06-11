"""Platform capabilities injected by the flokoa operator (runtime contract §6).

These ship in the runner baseline — never as OCI capability artifacts — and
version with the runner. They are real pydantic-ai ``AbstractCapability``
subclasses registered under ``flokoa.platform/<name>`` serialization names, so
they appear as ordinary capability entries in compiled AgentSpecs and in the
AgentSpec JSON schema the operator validates against.

Reserved names (contract v1):

- ``flokoa.platform/telemetry`` — traces + GenAI token metrics (roadmap 07)
- ``flokoa.platform/session-persistence`` — reserved, implemented in P1 (13)
- ``flokoa.platform/budget-guardrail`` — reserved, implemented in P1 (14)
"""

from typing import TYPE_CHECKING, Any

from flokoa_runner.platform_capabilities.telemetry import FlokoaTelemetry

if TYPE_CHECKING:
    from pydantic_ai.capabilities.abstract import AbstractCapability

PLATFORM_CAPABILITY_TYPES: dict[str, type["AbstractCapability[Any]"]] = {
    "flokoa.platform/telemetry": FlokoaTelemetry,
}
"""Registry of implemented platform capabilities, keyed by serialization name.

Passed as ``custom_capability_types`` both to ``Agent.from_spec`` at bootstrap
and to the AgentSpec schema generator, so the compiled-spec schema and the
runner accept exactly the same entries.
"""


def platform_capability_types() -> list[type["AbstractCapability[Any]"]]:
    """The platform capability classes, for ``custom_capability_types=`` call sites."""
    return list(PLATFORM_CAPABILITY_TYPES.values())
