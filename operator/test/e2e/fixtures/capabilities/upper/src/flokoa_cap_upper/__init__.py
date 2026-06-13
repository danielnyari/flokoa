"""Reference e2e fixture capability: upper (roadmap 09).

Like the echo fixture, but with exactly one small non-baseline dependency
(``inflection``) so the pinned-closure machinery — wheelhouse assembly,
manifest ``dependencies`` pins, and the runner's explicit-pin install — is
genuinely exercised end to end.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import inflection
from pydantic_ai.capabilities.abstract import AbstractCapability
from pydantic_ai.toolsets import FunctionToolset


@dataclass
class UpperCapability(AbstractCapability[Any]):
    """Shout messages back in humanized upper case."""

    exclaim: bool = False

    def get_toolset(self) -> FunctionToolset[Any]:
        toolset: FunctionToolset[Any] = FunctionToolset()

        @toolset.tool_plain
        def shout(message: str) -> str:
            """Return the message humanized and upper-cased."""
            shouted = inflection.humanize(message).upper()
            return f"{shouted}!" if self.exclaim else shouted

        return toolset
