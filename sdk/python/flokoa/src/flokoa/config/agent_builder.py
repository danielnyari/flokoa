"""Agent builder factory with ``_parse_config()`` hook.

Implements the factory method pattern inspired by Google ADK's
``BaseAgent.from_config()`` design:

1. ``from_config()`` is ``@final`` — subclasses cannot change the overall flow.
2. ``_create_base_kwargs()`` handles fields shared across all agent types.
3. ``_parse_config()`` is the override hook for framework/type-specific fields.
4. ``_build()`` constructs the actual agent from the kwargs dict.

Each builder declares a ``config_type`` ClassVar so the system knows which
config model it expects, enabling two-phase validation for extensibility.
"""

from __future__ import annotations

import inspect
import logging
from typing import Any, ClassVar, final

from flokoa.config.agent_config import BaseAgentConfig, LlmAgentConfig, TaskAgentConfig
from flokoa.config.code_ref import CodeRef, resolve_code_ref, resolve_qualified_name
from flokoa.config.tool_config import ToolConfig, ToolRefType
from flokoa_types import IntegrationType

logger = logging.getLogger(__name__)


class BaseAgentBuilder:
    """Base class for all agent builders.

    Subclasses override :meth:`_parse_config` and :meth:`_build` to provide
    framework-specific or agent-type-specific construction.

    The ``config_type`` ClassVar declares which config model this builder
    handles.  This is used by the builder registry for dispatch.
    """

    config_type: ClassVar[type[BaseAgentConfig]] = BaseAgentConfig

    @final
    @classmethod
    def from_config(cls, config: BaseAgentConfig) -> Any:
        """Build an agent from a config object.

        This method is ``@final`` — subclasses must not override it.
        Override :meth:`_parse_config` and :meth:`_build` instead.

        Args:
            config: A validated agent config object.

        Returns:
            A live agent instance (framework-specific type).
        """
        kwargs = cls._create_base_kwargs(config)
        kwargs = cls._parse_config(config, kwargs)
        return cls._build(config, kwargs)

    @classmethod
    def _create_base_kwargs(cls, config: BaseAgentConfig) -> dict[str, Any]:
        """Extract common fields into a kwargs dict.

        Handles: name, description, instruction, model, tools, callbacks.
        """
        kwargs: dict[str, Any] = {
            "name": config.name,
        }
        if config.description:
            kwargs["description"] = config.description
        if config.instruction is not None:
            kwargs["instruction"] = config.instruction
        if config.model is not None:
            kwargs["model_config"] = config.model
        if config.tools:
            kwargs["tools"] = cls._resolve_tools(config.tools)
        if config.before_agent_callbacks:
            kwargs["before_agent_callbacks"] = [
                resolve_code_ref(ref) for ref in config.before_agent_callbacks
            ]
        if config.after_agent_callbacks:
            kwargs["after_agent_callbacks"] = [
                resolve_code_ref(ref) for ref in config.after_agent_callbacks
            ]
        return kwargs

    @classmethod
    def _parse_config(
        cls,
        config: BaseAgentConfig,
        kwargs: dict[str, Any],
    ) -> dict[str, Any]:
        """Override hook for subclass-specific config fields.

        Subclasses add their own fields to the kwargs dict here.
        The default implementation returns kwargs unchanged.

        Args:
            config: The full config object.
            kwargs: The kwargs dict built by :meth:`_create_base_kwargs`.

        Returns:
            The (possibly modified) kwargs dict.
        """
        return kwargs

    @classmethod
    def _build(cls, config: BaseAgentConfig, kwargs: dict[str, Any]) -> Any:
        """Construct the agent instance from kwargs.

        Subclasses must override this to create the framework-specific agent.

        Args:
            config: The full config object (for any extra context needed).
            kwargs: The final kwargs dict.

        Returns:
            A live agent instance.

        Raises:
            NotImplementedError: If not overridden by a subclass.
        """
        raise NotImplementedError(
            f"{cls.__name__}._build() must be implemented by subclasses."
        )

    @classmethod
    def _resolve_tools(cls, tool_configs: list[ToolConfig]) -> list[Any]:
        """Resolve tool configs into live tool objects.

        Uses multi-strategy resolution:

        - **openapi**: Returns the raw ToolConfig for the executor to handle
          (since OpenAPI toolset creation requires framework-specific builders).
        - **function**: Resolves the code reference; if the result is a plain
          callable it's used directly as a function tool.
        - **class**: Resolves the code reference; instantiates the class via
          the CodeRef args mechanism.

        Args:
            tool_configs: List of tool configs.

        Returns:
            Mixed list of resolved tools and unresolved OpenAPI ToolConfigs.
        """
        resolved: list[Any] = []
        for tc in tool_configs:
            match tc.type:
                case ToolRefType.OPENAPI:
                    # Keep as config — framework-specific executor resolves these
                    resolved.append(tc)

                case ToolRefType.FUNCTION:
                    tool = resolve_code_ref(tc.code)  # type: ignore[arg-type]
                    if not callable(tool):
                        raise TypeError(
                            f"Function tool '{tc.name}' resolved to a non-callable: {type(tool)}"
                        )
                    resolved.append(tool)

                case ToolRefType.CLASS:
                    tool = resolve_code_ref(tc.code)  # type: ignore[arg-type]
                    # If CodeRef had args, resolve_code_ref already called the class.
                    # If it didn't, and we got a class back, instantiate it.
                    if inspect.isclass(tool):
                        tool = tool()
                    resolved.append(tool)

                case _:
                    logger.warning("Unknown tool type '%s' for tool '%s', skipping.", tc.type, tc.name)

        return resolved


class PydanticAIAgentBuilder(BaseAgentBuilder):
    """Builds a ``pydantic_ai.Agent`` from :class:`LlmAgentConfig`."""

    config_type: ClassVar[type[BaseAgentConfig]] = LlmAgentConfig

    @classmethod
    def _parse_config(
        cls,
        config: BaseAgentConfig,
        kwargs: dict[str, Any],
    ) -> dict[str, Any]:
        assert isinstance(config, LlmAgentConfig)

        if config.output_schema is not None:
            kwargs["output_schema"] = config.output_schema
        if config.input_schema is not None:
            kwargs["input_schema"] = config.input_schema
        if config.before_model_callbacks:
            kwargs["before_model_callbacks"] = [
                resolve_code_ref(ref) for ref in config.before_model_callbacks
            ]
        if config.after_model_callbacks:
            kwargs["after_model_callbacks"] = [
                resolve_code_ref(ref) for ref in config.after_model_callbacks
            ]
        if config.agent_class:
            kwargs["agent_class"] = resolve_code_ref(config.agent_class)

        return kwargs

    @classmethod
    def _build(cls, config: BaseAgentConfig, kwargs: dict[str, Any]) -> Any:
        from pydantic_ai import Agent, StructuredDict

        assert isinstance(config, LlmAgentConfig)

        agent_kwargs: dict[str, Any] = {}

        # Build output type from schema
        output_schema = kwargs.get("output_schema")
        if output_schema is not None:
            agent_kwargs["output_type"] = StructuredDict(
                output_schema.json_schema,
                name=output_schema.name,
                description=output_schema.description,
            )

        # If a custom agent class was specified, use it
        custom_cls = kwargs.get("agent_class")
        if custom_cls is not None:
            if inspect.isclass(custom_cls) and issubclass(custom_cls, Agent):
                return custom_cls(**agent_kwargs)
            raise TypeError(
                f"agent_class must be a subclass of pydantic_ai.Agent, got: {custom_cls}"
            )

        return Agent(**agent_kwargs)


class GoogleADKAgentBuilder(BaseAgentBuilder):
    """Builds a ``google.adk.agents.LlmAgent`` from :class:`LlmAgentConfig`."""

    config_type: ClassVar[type[BaseAgentConfig]] = LlmAgentConfig

    @classmethod
    def _parse_config(
        cls,
        config: BaseAgentConfig,
        kwargs: dict[str, Any],
    ) -> dict[str, Any]:
        assert isinstance(config, LlmAgentConfig)

        if config.before_model_callbacks:
            kwargs["before_model_callbacks"] = [
                resolve_code_ref(ref) for ref in config.before_model_callbacks
            ]
        if config.after_model_callbacks:
            kwargs["after_model_callbacks"] = [
                resolve_code_ref(ref) for ref in config.after_model_callbacks
            ]
        if config.agent_class:
            kwargs["agent_class"] = resolve_code_ref(config.agent_class)

        return kwargs

    @classmethod
    def _build(cls, config: BaseAgentConfig, kwargs: dict[str, Any]) -> Any:
        from google.adk.agents import LlmAgent

        assert isinstance(config, LlmAgentConfig)

        agent_kwargs: dict[str, Any] = {
            "name": kwargs["name"],
        }
        if kwargs.get("description"):
            agent_kwargs["description"] = kwargs["description"]
        if kwargs.get("instruction"):
            agent_kwargs["instruction"] = kwargs["instruction"]

        custom_cls = kwargs.get("agent_class")
        if custom_cls is not None:
            if inspect.isclass(custom_cls) and issubclass(custom_cls, LlmAgent):
                return custom_cls(**agent_kwargs)
            raise TypeError(
                f"agent_class must be a subclass of google.adk.agents.LlmAgent, got: {custom_cls}"
            )

        return LlmAgent(**agent_kwargs)


class MarvinTaskBuilder(BaseAgentBuilder):
    """Builds and executes a Marvin task from :class:`TaskAgentConfig`.

    Unlike LLM builders that return a persistent agent, :meth:`_build`
    returns a dict of kwargs ready for :func:`marvin.run` / :func:`marvin.classify`
    / etc.  The actual execution is handled by the executor.
    """

    config_type: ClassVar[type[BaseAgentConfig]] = TaskAgentConfig

    @classmethod
    def _parse_config(
        cls,
        config: BaseAgentConfig,
        kwargs: dict[str, Any],
    ) -> dict[str, Any]:
        assert isinstance(config, TaskAgentConfig)

        kwargs["task_type"] = config.task_type
        if config.result_type is not None:
            kwargs["result_type"] = config.result_type
        if config.input is not None:
            kwargs["input"] = config.input
        if config.labels is not None:
            kwargs["labels"] = config.labels
        if config.multi_label is not None:
            kwargs["multi_label"] = config.multi_label
        if config.count is not None:
            kwargs["count"] = config.count
        if config.context is not None:
            kwargs["context"] = config.context

        return kwargs

    @classmethod
    def _build(cls, config: BaseAgentConfig, kwargs: dict[str, Any]) -> dict[str, Any]:
        """Return the kwargs dict for the managed-task executor.

        The executor calls the appropriate ``marvin.*`` function with these kwargs.
        """
        return kwargs


# ---------------------------------------------------------------------------
# Builder registry
# ---------------------------------------------------------------------------

_BUILDER_REGISTRY: dict[tuple[str, str], type[BaseAgentBuilder]] = {
    ("llm", IntegrationType.PYDANTIC_AI): PydanticAIAgentBuilder,
    ("llm", IntegrationType.GOOGLE_ADK): GoogleADKAgentBuilder,
    ("task", "marvin"): MarvinTaskBuilder,
}


def register_builder(
    agent_type: str,
    framework: str,
    builder_cls: type[BaseAgentBuilder],
) -> None:
    """Register a custom builder for an (agent_type, framework) pair.

    Args:
        agent_type: The agent type discriminator value (e.g., ``"llm"``, ``"task"``).
        framework: The framework identifier (e.g., ``"pydantic-ai"``, ``"google-adk"``).
        builder_cls: The builder class to register.
    """
    _BUILDER_REGISTRY[(agent_type, framework)] = builder_cls


def get_builder(
    agent_type: str,
    framework: str,
) -> type[BaseAgentBuilder]:
    """Look up a builder for the given (agent_type, framework) pair.

    Args:
        agent_type: The agent type discriminator value.
        framework: The framework identifier.

    Returns:
        The registered builder class.

    Raises:
        KeyError: If no builder is registered for the given pair.
    """
    key = (agent_type, framework)
    if key not in _BUILDER_REGISTRY:
        raise KeyError(
            f"No builder registered for (agent_type={agent_type!r}, framework={framework!r}). "
            f"Available: {list(_BUILDER_REGISTRY.keys())}"
        )
    return _BUILDER_REGISTRY[key]
