from typing import Callable

from flokoa.types import ToolType

from .http_api import call_http_api_tool

TOOL_CALLABLES: dict[ToolType, Callable] = {
    ToolType.HTTP_API: call_http_api_tool,
}

__all__ = ["TOOL_CALLABLES", "call_http_api_tool"]
