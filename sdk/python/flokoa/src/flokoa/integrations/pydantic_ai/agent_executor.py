import logging
from typing import TYPE_CHECKING, Any, override

from a2a.server.agent_execution import RequestContext
from a2a.server.events import EventQueue
from a2a.utils import new_agent_text_message
from flokoa_types import (
    ToolDefinition as FlokoaToolDefinition,
)
from flokoa_types import ToolType
from flokoa_types.modelconfig import ProviderType
from pydantic_ai import FunctionToolset

from flokoa.agent_executor import FlokoaAgentExecutor
from flokoa.cache import ConfigCache
from flokoa.exceptions import (
    CancelNotSupportedError,
    ModelNotConfiguredError,
    ProviderNotConfiguredError,
)
from flokoa.tools import ToolsetFactory, default_factory

from .model_factory import create_model, create_provider


def _openapi_builder(tool_definition: FlokoaToolDefinition) -> list[Any]:
    from flokoa.tools.openapi import OpenAPIToolset

    return OpenAPIToolset.from_tool_definition(tool_definition).get_tools()


default_factory.register(ToolType.OPENAPI, _openapi_builder)

if TYPE_CHECKING:
    from pydantic_ai import Agent
    from pydantic_ai.models import Model
    from pydantic_ai.providers import Provider

    PydanticAIAgent = Agent[Any, Any]

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

    def __init__(
        self,
        agent: "PydanticAIAgent",
        cache: ConfigCache | None = None,
        toolset_factory: ToolsetFactory | None = None,
    ):
        """Initialize the executor.

        Args:
            agent: The PydanticAI agent to wrap.
            cache: Optional cache instance. Uses global cache if not provided.
            toolset_factory: Optional factory for building toolsets. Uses the
                default factory (with OpenAPI support) if not provided.
        """
        super().__init__(agent, cache)
        self._toolset_factory = toolset_factory or default_factory
        self._cached_toolset: FunctionToolset | None = None
        self._cached_tool_definitions: list[FlokoaToolDefinition] | None = None

    @property
    @override
    def agent(self) -> "PydanticAIAgent":
        return super().agent  # type: ignore[return-value]

    def _build_toolset(self) -> FunctionToolset:
        """Build a new toolset from current tool definitions via the factory."""
        logger.debug(
            "_build_toolset(): %d tool definition(s) to build",
            len(self.tool_definitions),
        )
        tools = self._toolset_factory.build(self.tool_definitions)
        toolset = FunctionToolset()
        for tool in tools:
            toolset.add_tool(tool)
        logger.debug("_build_toolset(): built FunctionToolset with %d tool(s)", len(tools))
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
        logger.debug(
            "_get_toolset(): %d current tool definition(s), cached_toolset=%s",
            len(current_tools),
            self._cached_toolset is not None,
        )

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
        else:
            logger.debug("_get_toolset(): using cached toolset (no changes)")

        # _cached_toolset is guaranteed to be set after the rebuild block above
        return self._cached_toolset  # type: ignore[return-value]

    def invalidate_caches(self) -> None:
        """Invalidate all caches and force toolset rebuild on next access."""
        super().invalidate_caches()
        self._cached_toolset = None
        self._cached_tool_definitions = None

    def _create_provider(self, provider_type: ProviderType) -> "Provider":
        if self.model_config is None:
            raise ProviderNotConfiguredError(f"Model config is required to create provider '{provider_type.value}'")
        return create_provider(self.model_config)

    def _create_model(self, provider: "Provider") -> "Model":
        if self.model_config is None:
            raise ModelNotConfiguredError("Model config is required to create a model")
        if self.model_provider is None:
            raise ProviderNotConfiguredError("Model provider is required to create a model")

        return create_model(self.model_config, provider)

    @override
    async def execute(self, context: RequestContext, event_queue: EventQueue) -> None:
        request = context.get_user_input()
        logger.info("Executing PydanticAI agent with request (length=%d)", len(request) if request else 0)

        if not self.model_config and not self.model_provider and self.agent.model is None:
            raise ProviderNotConfiguredError("Model provider must be configured to execute agent")

        run_kwargs: dict[str, Any] = {
            "toolsets": [self._get_toolset()],
            "model": (
                self._create_model(self._create_provider(self.model_config.provider.type))
                if self.model_config
                else self.agent.model
            ),
        }

        # Use operator-mounted instruction if available (overrides agent default)
        instruction = self.instruction
        if instruction is not None:
            run_kwargs["instructions"] = instruction

        result = await self.agent.run(request, **run_kwargs)
        await event_queue.enqueue_event(new_agent_text_message(result.output))

    @override
    async def cancel(self, context: RequestContext, event_queue: EventQueue) -> None:
        raise CancelNotSupportedError("cancel not supported")
