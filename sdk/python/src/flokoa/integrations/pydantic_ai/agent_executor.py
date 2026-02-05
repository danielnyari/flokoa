import logging
from enum import Enum
from typing import TYPE_CHECKING, Any, Callable, override

from a2a.server.agent_execution import RequestContext
from a2a.server.events import EventQueue
from a2a.utils import new_agent_text_message
from pydantic import BaseModel
from pydantic_ai import FunctionToolset, Tool
from pydantic_ai.providers import infer_provider_class
from pydantic_ai.settings import ModelSettings, merge_model_settings

from flokoa import tools as flokoa_tools
from flokoa.agent_executor import FlokoaAgentExecutor
from flokoa.cache import ConfigCache
from flokoa.exceptions import CancelNotSupportedError, ModelNotConfiguredError, ProviderNotConfiguredError
from flokoa.types import (
    ModelParameters,
    ToolType,
)
from flokoa.types import (
    ToolDefinition as FlokoaToolDefinition,
)
from flokoa.types.modelconfig import ProviderType

from .models import PROVIDER_MODEL_MAP

if TYPE_CHECKING:
    from pydantic_ai import Agent
    from pydantic_ai.models import Model
    from pydantic_ai.providers import Provider

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

        # _cached_toolset is guaranteed to be set after the rebuild block above
        return self._cached_toolset  # type: ignore[return-value]

    def invalidate_caches(self) -> None:
        """Invalidate all caches and force toolset rebuild on next access."""
        super().invalidate_caches()
        self._cached_toolset = None
        self._cached_tool_definitions = None

    def _create_provider(self, provider_type: ProviderType) -> "Provider":
        if self.provider_config is None:
            raise ProviderNotConfiguredError(f"Provider config is required to create provider '{provider_type.value}'")
        provider_cls = infer_provider_class(provider=provider_type.value)
        return provider_cls(**self.provider_config.model_dump())

    def _create_model(self, provider: "Provider") -> "Model":
        if self.model_config is None:
            raise ModelNotConfiguredError("Model config is required to create a model")
        if self.model_provider is None:
            raise ProviderNotConfiguredError("Model provider is required to create a model")

        provider_entry = PROVIDER_MODEL_MAP.get(self.model_provider)
        if not provider_entry:
            raise ModelNotConfiguredError(f"No model mapping found for provider '{self.model_provider}'")

        model_cls = provider_entry["model_class"]
        model_settings = self._build_model_settings()

        if model_settings is None:
            raise ModelNotConfiguredError("Model settings could not be built from configuration")

        return model_cls(provider=provider, settings=model_settings)

    def _build_model_settings(self) -> ModelSettings | None:
        """Build pydantic_ai ModelSettings from flokoa ModelParameters."""
        if not self.model_config or not self.model_config.parameters:
            return None

        params = self.model_config.parameters
        common_settings = self._params_to_settings(params)
        provider_settings = self._provider_params_to_settings()

        return merge_model_settings(common_settings, provider_settings)

    def _params_to_settings(self, params: ModelParameters) -> ModelSettings:
        """Convert common ModelParameters to ModelSettings dict."""
        # Mapping of param attribute -> (settings key, optional transform)
        param_mappings: list[tuple[str, str, Callable[[Any], Any] | None]] = [
            ("max_tokens", "max_tokens", None),
            ("temperature", "temperature", float),
            ("top_p", "top_p", float),
            ("seed", "seed", None),
            ("presence_penalty", "presence_penalty", float),
            ("frequency_penalty", "frequency_penalty", float),
            ("logit_bias", "logit_bias", None),
            ("stop_sequences", "stop_sequences", None),
            ("extra_headers", "extra_headers", None),
            ("extra_body", "extra_body", None),
            ("parallel_tool_calls", "parallel_tool_calls", None),
            ("time_out", "timeout", float),
        ]

        settings: ModelSettings = {}
        for attr, key, transform in param_mappings:
            value = getattr(params, attr)
            if value is not None:
                settings[key] = transform(value) if transform else value

        return settings

    def _provider_params_to_settings(self) -> ModelSettings | None:
        """Convert provider-specific parameters to prefixed ModelSettings dict."""
        provider_params = self.provider_model_parameters
        if not provider_params or not self.model_provider:
            return None

        prefix = self.model_provider.value + "_"
        settings: ModelSettings = {}

        for field_name in provider_params.__pydantic_fields__:
            value = getattr(provider_params, field_name)
            if value is not None:
                if isinstance(value, Enum):
                    value = value.value
                elif isinstance(value, BaseModel):
                    value = value.model_dump(exclude_none=True)
                settings[prefix + field_name] = value

        return settings if settings else None

    @override
    async def execute(self, context: RequestContext, event_queue: EventQueue) -> None:
        request = context.get_user_input()
        logger.info(f"Executing PydanticAI agent with request: {request}")
        if self.model_provider is None:
            raise ProviderNotConfiguredError("Model provider must be configured to execute agent")
        result = await self.agent.run(
            request,
            toolsets=[self._get_toolset()],
            model=self._create_model(self._create_provider(self.model_provider)),
        )  #
        await event_queue.enqueue_event(new_agent_text_message(result.output))

    @override
    async def cancel(self, context: RequestContext, event_queue: EventQueue) -> None:
        raise CancelNotSupportedError("cancel not supported")
