"""Unified declarative agent configuration system.

This package provides:

- :class:`AgentConfig` — top-level agent configuration
- :class:`LlmAgentConfig` — config for pydantic-ai LLM agents
- :class:`CodeRef` — reference to Python objects by fully-qualified dotted path
- :class:`ToolConfig` — multi-strategy tool configuration
- :class:`BaseAgentBuilder` — factory base with ``_parse_config()`` hook
- :class:`PydanticAIAgentBuilder` — the pydantic-ai builder implementation

Example::

    from flokoa.config import AgentConfig, load_agent_config

    # Load from unified config file
    config = load_agent_config("/path/to/agent-config.json")

    # Or build programmatically
    config = AgentConfig.model_validate({
        "name": "my_agent",
        "instruction": "You are helpful.",
        "model": {"provider": {"type": "openai"}, "model": "gpt-4o"},
        "tools": [
            {"name": "search", "type": "function", "code": {"name": "my_app.tools.search"}},
        ],
    })
"""

from flokoa.config.agent_builder import (
    BaseAgentBuilder,
    PydanticAIAgentBuilder,
    get_builder,
    register_builder,
)
from flokoa.config.agent_config import (
    AgentConfig,
    BaseAgentConfig,
    LlmAgentConfig,
)
from flokoa.config.code_ref import (
    Argument,
    CodeRef,
    resolve_callbacks,
    resolve_code_ref,
    resolve_qualified_name,
)
from flokoa.config.loader import (
    load_agent_config,
    load_agent_config_from_dict,
    load_legacy_llm_config,
)
from flokoa.config.tool_config import ToolConfig, ToolRefType

__all__ = [
    "AgentConfig",
    "Argument",
    "BaseAgentBuilder",
    "BaseAgentConfig",
    "CodeRef",
    "LlmAgentConfig",
    "PydanticAIAgentBuilder",
    "ToolConfig",
    "ToolRefType",
    "get_builder",
    "load_agent_config",
    "load_agent_config_from_dict",
    "load_legacy_llm_config",
    "register_builder",
    "resolve_callbacks",
    "resolve_code_ref",
    "resolve_qualified_name",
]
