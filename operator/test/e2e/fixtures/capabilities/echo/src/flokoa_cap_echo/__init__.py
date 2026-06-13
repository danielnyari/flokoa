"""Reference e2e fixture capability: echo (roadmap 09).

The smallest real pydantic-ai capability that exercises the full artifact
path: one config field, one tool, zero non-baseline dependencies — the
wheelhouse holds exactly one wheel. Built into an OCI artifact by
``../build.sh`` (pre-CLI path; unit 10's ``flokoa capability build``
supersedes the script and CI diffs the manifests).
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from pydantic_ai.capabilities.abstract import AbstractCapability
from pydantic_ai.toolsets import FunctionToolset


@dataclass
class EchoCapability(AbstractCapability[Any]):
    """Echo messages back, prefixed.

    Spec entries hydrate via the default ``from_spec`` → ``cls(**config)``
    path, so the per-agent config schema is this dataclass's own field.
    """

    prefix: str = "echo"

    def get_toolset(self) -> FunctionToolset[Any]:
        toolset: FunctionToolset[Any] = FunctionToolset()

        @toolset.tool_plain
        def echo(message: str) -> str:
            """Echo the message back, prefixed with the configured prefix."""
            return f"{self.prefix}: {message}"

        return toolset
