"""Flokoa toolset implementation for Google ADK."""

import inspect
import logging
from typing import TYPE_CHECKING, Any

_MISSING_ADK_MESSAGE = (
    "google-adk is not installed or unavailable at runtime. "
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
# This also handles any non-class sentinel returned by mocked imports.
if _BaseToolset is None or not inspect.isclass(_BaseToolset):

    class BaseToolset:  # type: ignore[no-redef]
        async def get_tools(self, readonly_context: Any | None = None) -> list[Any]:
            """Return tools for the given readonly ADK context."""
            raise ImportError(f"Failed to call get_tools: {_MISSING_ADK_MESSAGE}")

        async def close(self) -> None:
            raise ImportError(f"Failed to call close: {_MISSING_ADK_MESSAGE}")

else:
    BaseToolset = _BaseToolset

if TYPE_CHECKING:
    from google.adk.tools import BaseTool

logger = logging.getLogger(__name__)


class FlokoaToolset(BaseToolset):
    """A toolset that wraps pre-built ADK tools for use with Google ADK agents.

    This implements the BaseToolset interface expected by ADK agents.
    Tools are built externally via the ToolsetFactory and passed in.

    Example::

        tools = default_factory.build(
            tool_definitions, IntegrationType.GOOGLE_ADK
        )
        toolset = FlokoaToolset(tools)
        agent.tools.append(toolset)
    """

    def __init__(self, tools: list[Any]):
        """Initialize the toolset with pre-built ADK tools.

        Args:
            tools: List of ADK BaseTool instances produced by the factory.
        """
        super().__init__()
        self._tools: list[BaseTool] = tools
        logger.info("FlokoaToolset initialized with %d tools", len(self._tools))

    async def get_tools(self, readonly_context: Any | None = None) -> list["BaseTool"]:
        """Get the list of tools provided by this toolset.

        Args:
            readonly_context: Optional readonly context (for dynamic tool selection).

        Returns:
            List of BaseTool instances.
        """
        return self._tools

    async def close(self) -> None:
        """Clean up resources.

        This toolset has no resources to clean up, but the method is required
        by the BaseToolset interface.
        """
        logger.debug("FlokoaToolset.close() called.")
