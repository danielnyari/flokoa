"""Flokoa toolset implementation for Google ADK."""

import inspect
import logging
from typing import TYPE_CHECKING, Any, Callable, Optional

_MISSING_ADK_MESSAGE = (
    "Failed to use BaseToolset: google-adk is not installed or unavailable at runtime. "
    "Install it with your package manager (e.g., uv pip install google-adk)."
)

try:
    from google.adk.tools import BaseToolset as _BaseToolset
except (ImportError, AttributeError):
    try:
        from google.adk.tools.base_toolset import BaseToolset as _BaseToolset
    except (ImportError, AttributeError):
        _BaseToolset = None

# In tests, google.adk.tools is a MagicMock, so BaseToolset resolves to a non-type.
if _BaseToolset is None or not inspect.isclass(_BaseToolset):
    class BaseToolset:  # type: ignore[no-redef]
        async def get_tools(self, readonly_context: Optional[Any] = None) -> list[Any]:
            """Return tools for the given readonly ADK context."""
            raise ImportError(_MISSING_ADK_MESSAGE)

        async def close(self) -> None:
            raise ImportError(_MISSING_ADK_MESSAGE)
else:
    BaseToolset = _BaseToolset

from flokoa.types import ToolDefinition as FlokoaToolDefinition

if TYPE_CHECKING:
    from google.adk.tools import BaseTool

logger = logging.getLogger(__name__)


class FlokoaToolset(BaseToolset):
    """A toolset that wraps Flokoa tool definitions for use with Google ADK agents.

    This implements the BaseToolset interface expected by ADK agents,
    providing FunctionTool instances from Flokoa's tool definitions.

    Example:
        toolset = FlokoaToolset(
            tool_definitions=[...],
            get_tool_callable=executor._get_tool_callable,
        )
        agent.tools.append(toolset)
    """

    def __init__(
        self,
        tool_definitions: list[FlokoaToolDefinition],
        get_tool_callable: Callable[[FlokoaToolDefinition], Callable[..., Any]],
    ):
        """Initialize the toolset and create FunctionTool instances.

        Args:
            tool_definitions: List of Flokoa tool definitions to wrap.
            get_tool_callable: Function to create callables from tool definitions.
        """
        super().__init__()
        from google.adk.tools import FunctionTool

        self._tool_definitions = tool_definitions
        self._get_tool_callable = get_tool_callable

        # Create FunctionTool instances once during init (following ADK pattern)
        self._tools: list[BaseTool] = []
        for tool_def in tool_definitions:
            callable_fn = get_tool_callable(tool_def)
            # FunctionTool extracts name from __name__ and description from __doc__
            callable_fn.__name__ = tool_def.name
            callable_fn.__doc__ = tool_def.description
            tool = FunctionTool(func=callable_fn)
            self._tools.append(tool)
            logger.info(f"Created Flokoa tool '{tool_def.name}' for ADK.")

        logger.info(f"FlokoaToolset initialized with {len(self._tools)} tools: {[t.name for t in self._tools]}")

    async def get_tools(self, readonly_context: Optional[Any] = None) -> list["BaseTool"]:
        """Get the list of tools provided by this toolset.

        Args:
            readonly_context: Optional readonly context (for dynamic tool selection).

        Returns:
            List of FunctionTool instances.
        """
        logger.debug(f"FlokoaToolset.get_tools() called, returning {len(self._tools)} tools.")
        return self._tools

    async def close(self) -> None:
        """Clean up resources.

        This toolset has no resources to clean up, but the method is required
        by the BaseToolset interface.
        """
        logger.debug("FlokoaToolset.close() called.")
