import logging
from typing import TYPE_CHECKING, Any, Callable, override

from a2a.server.agent_execution import RequestContext
from a2a.server.events import EventQueue
from a2a.utils import new_agent_text_message
from pydantic_ai import FunctionToolset, Tool

from flokoa import tools as flokoa_tools
from flokoa.agent_executor import FlokoaAgentExecutor
from flokoa.cache import ConfigCache
from flokoa.exceptions import CancelNotSupportedError
from flokoa.types import ModelConfig, ToolType
from flokoa.types import ToolDefinition as FlokoaToolDefinition

if TYPE_CHECKING:
    from pydantic_ai import Agent

logger = logging.getLogger(__name__)


class PydanticAIAgentExecutor(FlokoaAgentExecutor):
    """A2A AgentExecutor that wraps a PydanticAI agent with automatic
    flokoa tool injection from /etc/flokoa/tools/.

    This executor provides:
    - Automatic tool injection from mounted ConfigMap
    - TTL-based caching of tools and model config
    - Automatic toolset reloading when files change
    - Model configuration loading with caching

    The toolset is lazily rebuilt when:
    - Tool files are modified (detected via mtime)
    - Cache TTL expires (default: 60 seconds)
    - invalidate_caches() is called explicitly

    Environment Variables:
        FLOKOA_CACHE_TTL_SECONDS: TTL for cached configs in seconds (default: 60)
        FLOKOA_CACHE_ENABLED: Enable/disable caching (default: true)
    """

    def __init__(self, agent: "Agent", cache: ConfigCache | None = None):
        """Initialize the executor.

        Args:
            agent: The PydanticAI agent to wrap.
            cache: Optional cache instance. Uses global cache if not provided.
        """
        super().__init__(agent, cache)
        self._cached_toolset: FunctionToolset | None = None
        self._cached_tool_definitions: list[FlokoaToolDefinition] | None = None

    def _get_tool_callable(self, tool_definition: FlokoaToolDefinition) -> Callable[..., Any]:
        """Create a callable that accepts schema parameters and calls the underlying tool.

        The wrapper function accepts **kwargs matching the tool's input schema,
        and passes them to the appropriate tool handler with the tool's configuration.
        """
        if tool_definition.type == ToolType.HTTP_API:
            http_api = tool_definition.spec.http_api
            if http_api is None:
                raise ValueError(f"Tool '{tool_definition.name}' has type http-api but no http_api configuration")
            endpoint = http_api.url or ""
            method = http_api.method.value

            async def api_tool_wrapper(**kwargs: Any) -> dict[str, Any]:
                # Dynamic lookup allows mocking in tests
                return await flokoa_tools.call_http_api_tool(endpoint=endpoint, method=method, params=kwargs)

            return api_tool_wrapper

        return super()._get_tool_callable(tool_definition)

    def _create_tool(self, tool_definition: FlokoaToolDefinition) -> Tool:
        tool_callable = self._get_tool_callable(tool_definition)

        tool = Tool.from_schema(
            function=tool_callable,
            name=tool_definition.name,
            description=tool_definition.description,
            json_schema=tool_definition.input_json_schema,
            takes_ctx=False,
            sequential=False,
        )
        return tool

    def _build_toolset(self) -> FunctionToolset:
        """Build a new toolset from current tool definitions."""
        toolset = FunctionToolset()
        for tool_definition in self.tool_definitions:
            tool = self._create_tool(tool_definition)
            toolset.add_tool(tool)
            logger.info(f"Added tool '{tool_definition.name}' to agent toolset.")
        return toolset

    def _get_toolset(self) -> FunctionToolset:
        """Get the toolset, rebuilding if tools have changed.

        This method checks if the tools have changed. If so, it:
        1. Reloads tool definitions (via parent class)
        2. Rebuilds the FunctionToolset

        Returns:
            FunctionToolset with all configured tools.
        """
        current_tools = self.tool_definitions

        # Rebuild if tools changed or toolset not initialized
        # Compare by identity first (same list object), then by content
        needs_rebuild = (
            self._cached_toolset is None
            or self._cached_tool_definitions is None
            or self._cached_tool_definitions is not current_tools
        )

        if needs_rebuild:
            logger.info("Rebuilding toolset due to configuration change")
            self._cached_toolset = self._build_toolset()
            self._cached_tool_definitions = current_tools

        return self._cached_toolset

    def _get_model_config(self) -> ModelConfig | None:
        """Get model configuration with caching.

        Returns:
            ModelConfig if configured, None otherwise.
        """
        return self.model_config

    def invalidate_caches(self) -> None:
        """Invalidate all caches and force toolset rebuild on next access."""
        super().invalidate_caches()
        self._cached_toolset = None
        self._cached_tool_definitions = None

    # def _create_model(self) -> Any:
    #     model_config = self._get_model_config()
    #     if model_config:
    #         provider_entry = PROVIDER_MODEL_MAP.get(model_config.provider.type, None) if model_config else None
    #         model_class = provider_entry.get("model_class") if provider_entry else None
    #         provider_class = provider_entry.get("provider_class") if provider_entry else None
    #         settings_class = provider_entry.get("settings_class") if provider_entry else None

    @override
    async def execute(self, context: RequestContext, event_queue: EventQueue) -> None:
        request = context.get_user_input()
        logger.info(f"Executing PydanticAI agent with request: {request}")
        result = await self.agent.run(request, toolsets=[self._get_toolset()])
        await event_queue.enqueue_event(new_agent_text_message(result.output))

    @override
    async def cancel(self, context: RequestContext, event_queue: EventQueue) -> None:
        raise CancelNotSupportedError("cancel not supported")
