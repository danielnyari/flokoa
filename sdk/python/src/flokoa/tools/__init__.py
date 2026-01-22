from typing import Callable

from flokoa.types import ToolType

from .api import call_api_tool

TOOL_CALLABLES: dict[ToolType, Callable] = {
    ToolType.API: call_api_tool,
}

__all__ = ["TOOL_CALLABLES", "call_api_tool"]