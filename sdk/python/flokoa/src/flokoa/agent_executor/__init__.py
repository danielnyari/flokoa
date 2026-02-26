import logging
from typing import TYPE_CHECKING, Any

from a2a.server.agent_execution import AgentExecutor
from flokoa_types import (
    ModelConfig,
    ProviderConfigType,
    ProviderModelParametersType,
)
from flokoa_types import (
    ModelParameters as ModelParameters,
)
from flokoa_types import (
    ToolDefinition as FlokoaToolDefinition,
)
from flokoa_types.modelconfig import ProviderType

from flokoa.cache import (
    CACHE_KEY_MODEL_CONFIG,
    CACHE_KEY_TOOLS,
    ConfigCache,
    get_global_cache,
)
from flokoa.utils import load_instruction, load_model_config, load_tools

logger = logging.getLogger("flokoa." + __name__)

if TYPE_CHECKING:
    from google.adk.agents import BaseAgent
    from pydantic_ai import Agent

    AgentType = Agent[Any, Any] | BaseAgent


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

    def __init__(self, agent: "AgentType", cache: ConfigCache | None = None):
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
    def model_provider(self) -> ProviderType | None:
        if self.model_config:
            return self.model_config.provider.type
        return None

    @property
    def provider_config(self) -> ProviderConfigType | None:
        if not self.model_config or self.model_provider is None:
            return None
        return getattr(self.model_config.provider, self.model_provider.value, None)

    @property
    def provider_model_parameters(self) -> ProviderModelParametersType | None:
        if not self.model_config or self.model_provider is None:
            return None
        return getattr(self.model_config.parameters, self.model_provider.value, None)

    @property
    def agent(self) -> "AgentType":
        return self._agent

    @property
    def instruction(self) -> str | None:
        """Load instruction text from the operator-mounted file.

        Returns the content of /etc/flokoa/instruction.txt if it exists,
        None otherwise. Supports both integration and managed runtimes.
        """
        return load_instruction()

    def _reload_tools(self) -> None:
        """Reload tool definitions from files."""
        self._tool_definitions = load_tools(use_cache=True, cache=self._cache)
        logger.debug(
            "_reload_tools(): loaded %d tool definition(s): %s",
            len(self._tool_definitions),
            [td.name for td in self._tool_definitions],
        )

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
