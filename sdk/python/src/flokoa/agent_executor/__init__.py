from typing import TYPE_CHECKING, Callable

from a2a.server.agent_execution import AgentExecutor

from flokoa.cache import (
    CACHE_KEY_MODEL_CONFIG,
    CACHE_KEY_TOOLS,
    ConfigCache,
    get_global_cache,
)
from flokoa.tools import TOOL_CALLABLES
from flokoa.types import ModelConfig
from flokoa.types import ToolDefinition as FlokoaToolDefinition
from flokoa.utils import load_model_config, load_tools

if TYPE_CHECKING:
    from pydantic_ai import Agent


class FlokoaAgentExecutor(AgentExecutor):
    """Base class for Flokoa AgentExecutors with caching support.

    This executor provides:
    - Cached loading of tool definitions with TTL
    - Cached loading of model configuration with TTL
    - Automatic detection of config changes
    - Lazy reloading when configs are modified

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
        self._agent = agent
        self._cache = cache or get_global_cache()
        self._tool_definitions: list[FlokoaToolDefinition] | None = None
        self._model_config: ModelConfig | None = None
        self._model_config_loaded = False
        # Load initial tool definitions
        self._reload_tools()

    @property
    def cache(self) -> ConfigCache:
        """Get the cache instance used by this executor."""
        return self._cache

    @property
    def tool_definitions(self) -> list[FlokoaToolDefinition]:
        """Get tool definitions, reloading if cache is invalid."""
        if not self._cache.is_valid(CACHE_KEY_TOOLS):
            self._reload_tools()
        return self._tool_definitions or []

    @property
    def model_config(self) -> ModelConfig | None:
        """Get model configuration, reloading if cache is invalid."""
        if not self._model_config_loaded or not self._cache.is_valid(CACHE_KEY_MODEL_CONFIG):
            self._reload_model_config()
        return self._model_config

    @property
    def agent(self) -> "Agent":
        return self._agent

    def _reload_tools(self) -> None:
        """Reload tool definitions from files."""
        self._tool_definitions = load_tools(use_cache=True, cache=self._cache)

    def _reload_model_config(self) -> None:
        """Reload model configuration from file."""
        self._model_config = load_model_config(use_cache=True, cache=self._cache)
        self._model_config_loaded = True

    def are_tools_changed(self) -> bool:
        """Check if tool definitions have changed since last load.

        Returns:
            True if tools need to be reloaded, False otherwise.
        """
        return not self._cache.is_valid(CACHE_KEY_TOOLS)

    def is_model_config_changed(self) -> bool:
        """Check if model configuration has changed since last load.

        Returns:
            True if model config needs to be reloaded, False otherwise.
        """
        return not self._cache.is_valid(CACHE_KEY_MODEL_CONFIG)

    def invalidate_caches(self) -> None:
        """Invalidate all caches and force reload on next access."""
        self._cache.invalidate_all()
        self._model_config_loaded = False

    def _get_tool_callable(self, tool_definition: FlokoaToolDefinition) -> Callable:
        return TOOL_CALLABLES[tool_definition.type]
