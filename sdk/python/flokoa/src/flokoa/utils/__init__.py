import json
import os
from glob import glob

from a2a.types import AgentCapabilities, AgentCard, AgentSkill
from flokoa_types import ModelConfig, ToolDefinition
from flokoa_types.agentcard import AgentCard as FlokoaAgentCard
from flokoa_types.agenttool import AgentToolSpec

from flokoa.cache import (
    CACHE_KEY_AGENT_CARD,
    CACHE_KEY_MODEL_CONFIG,
    CACHE_KEY_TOOLS,
    ConfigCache,
    get_global_cache,
)

TOOLS_PATH = "/etc/flokoa/tools/"
AGENT_CARD_PATH = "/etc/flokoa/agent-card.json"
MODEL_CONFIG_PATH = "/etc/flokoa/model.json"
INSTRUCTION_PATH = "/etc/flokoa/instruction.txt"


def _load_agent_card_from_file(url: str | None = None) -> AgentCard | None:
    """Internal function to load agent card from file (uncached).

    Args:
        url: Override URL for the agent card.

    Returns:
        AgentCard (a2a type) if the file exists, None otherwise.
    """
    if not os.path.exists(AGENT_CARD_PATH):
        return None

    with open(AGENT_CARD_PATH) as f:
        card_data = json.load(f)

    # Validate using generated Flokoa AgentCard type
    flokoa_card = FlokoaAgentCard.model_validate(card_data)

    # Get URL from parameter, env var, or default
    agent_url = url or os.environ.get("FLOKOA_AGENT_URL", "")

    # Convert skills from Flokoa format to a2a format
    skills = [
        AgentSkill(
            id=skill.id,
            name=skill.name,
            description=skill.description,
            tags=skill.tags,
            examples=skill.examples,
            input_modes=skill.input_modes,
            output_modes=skill.output_modes,
        )
        for skill in flokoa_card.skills
    ]

    # Convert capabilities
    capabilities = AgentCapabilities(
        streaming=flokoa_card.capabilities.streaming or False,
        push_notifications=flokoa_card.capabilities.push_notifications or False,
        state_transition_history=flokoa_card.capabilities.state_transition_history or False,
    )

    return AgentCard(
        name=flokoa_card.name,
        description=flokoa_card.description,
        version=flokoa_card.version,
        url=agent_url,
        default_input_modes=flokoa_card.default_input_modes,
        default_output_modes=flokoa_card.default_output_modes,
        capabilities=capabilities,
        skills=skills,
    )


def load_agent_card(
    url: str | None = None,
    use_cache: bool = True,
    cache: ConfigCache | None = None,
) -> AgentCard | None:
    """Load agent card from /etc/flokoa/agent-card.json with optional caching.

    Args:
        url: Override URL for the agent card. If not provided, uses FLOKOA_AGENT_URL
             environment variable or defaults to empty string.
        use_cache: Whether to use caching (default: True).
        cache: Optional cache instance. Uses global cache if not provided.

    Returns:
        AgentCard (a2a type) if the file exists, None otherwise.

    The JSON format matches the Kubernetes Agent CRD card structure.

    Caching:
        The result is cached with TTL and file modification detection.
        Set FLOKOA_CACHE_TTL_SECONDS to configure TTL (default: 60).
        Set FLOKOA_CACHE_ENABLED=false to disable caching.
    """
    if not use_cache:
        return _load_agent_card_from_file(url)

    cache = cache or get_global_cache()

    # Check cache first
    cached = cache.get(CACHE_KEY_AGENT_CARD)
    if cached is not None:
        return cached

    # Load from file
    result = _load_agent_card_from_file(url)

    # Cache the result (even None, to avoid repeated file checks)
    if os.path.exists(AGENT_CARD_PATH):
        cache.set(CACHE_KEY_AGENT_CARD, result, file_paths=[AGENT_CARD_PATH])
    else:
        # File doesn't exist - cache with path so we detect when it's created
        cache.set(CACHE_KEY_AGENT_CARD, result, file_paths=[AGENT_CARD_PATH])

    return result


def _load_tools_from_files() -> tuple[list[ToolDefinition], list[str]]:
    """Internal function to load tools from files (uncached).

    Returns:
        Tuple of (tool definitions list, list of file paths that were loaded).
    """
    if not os.path.exists(TOOLS_PATH):
        return [], []

    definitions: list[ToolDefinition] = []
    file_paths: list[str] = []

    for filename in glob(os.path.join(TOOLS_PATH, "*.json")):
        file_paths.append(filename)
        with open(filename) as f:
            tool_cfg = json.load(f)
            tool_definition = ToolDefinition(
                name=tool_cfg["name"],
                spec=AgentToolSpec(**tool_cfg["spec"]),
                metadata=tool_cfg.get("metadata", None),
            )
            definitions.append(tool_definition)

    return definitions, file_paths


def load_tools(
    use_cache: bool = True,
    cache: ConfigCache | None = None,
) -> list[ToolDefinition]:
    """Load tool definitions from /etc/flokoa/tools/ with optional caching.

    The JSON format matches the Kubernetes AgentTool CRD structure:
    {
        "name": "tool_name",
        "spec": {
            "type": "http-api",
            "description": "Tool description",
            "inputSchema": {...},
            "outputSchema": {...},
            "httpApi": {
                "url": "https://api.example.com",
                "method": "GET"
            }
        },
        "metadata": {...}  // optional
    }

    Args:
        use_cache: Whether to use caching (default: True).
        cache: Optional cache instance. Uses global cache if not provided.

    Returns:
        List of ToolDefinition objects.

    Caching:
        The result is cached with TTL and file modification detection.
        Set FLOKOA_CACHE_TTL_SECONDS to configure TTL (default: 60).
        Set FLOKOA_CACHE_ENABLED=false to disable caching.
    """
    if not use_cache:
        definitions, _ = _load_tools_from_files()
        return definitions

    cache = cache or get_global_cache()

    # Check cache first
    cached = cache.get(CACHE_KEY_TOOLS)
    if cached is not None:
        return cached

    # Load from files
    definitions, file_paths = _load_tools_from_files()

    # Cache the result with file tracking
    # Include the tools directory itself to detect new files
    tracked_paths = file_paths.copy()
    if os.path.exists(TOOLS_PATH):
        tracked_paths.append(TOOLS_PATH)

    cache.set(CACHE_KEY_TOOLS, definitions, file_paths=tracked_paths)

    return definitions


def _load_model_config_from_file() -> ModelConfig | None:
    """Internal function to load model config from file (uncached).

    Returns:
        ModelConfig if the file exists, None otherwise.
    """
    if not os.path.exists(MODEL_CONFIG_PATH):
        return None

    with open(MODEL_CONFIG_PATH) as f:
        config_data = json.load(f)

    return ModelConfig.model_validate(config_data)


def load_model_config(
    use_cache: bool = True,
    cache: ConfigCache | None = None,
) -> ModelConfig | None:
    """Load model configuration from /etc/flokoa/model.json with optional caching.

    Returns:
        ModelConfig if the file exists, None otherwise.

    Args:
        use_cache: Whether to use caching (default: True).
        cache: Optional cache instance. Uses global cache if not provided.

    The configuration maps to PydanticAI's provider/model architecture.
    See ModelConfig docstring for detailed usage examples.

    Example:
        from pydantic_ai import Agent
        from flokoa.utils import load_model_config

        config = load_model_config()
        if config:
            agent = Agent(config.get_model_name(), model_settings=config.settings)

    For local development without the operator, this function returns None,
    allowing the agent to use its default model configuration.

    Caching:
        The result is cached with TTL and file modification detection.
        Set FLOKOA_CACHE_TTL_SECONDS to configure TTL (default: 60).
        Set FLOKOA_CACHE_ENABLED=false to disable caching.
    """
    if not use_cache:
        return _load_model_config_from_file()

    cache = cache or get_global_cache()

    # Check cache first
    cached = cache.get(CACHE_KEY_MODEL_CONFIG)
    if cached is not None:
        return cached

    # Load from file
    result = _load_model_config_from_file()

    # Cache the result with file tracking
    cache.set(CACHE_KEY_MODEL_CONFIG, result, file_paths=[MODEL_CONFIG_PATH])

    return result


def load_instruction() -> str | None:
    """Load instruction text from /etc/flokoa/instruction.txt.

    Returns:
        The instruction text if the file exists, None otherwise.
    """
    path = os.environ.get("FLOKOA_INSTRUCTION_PATH", INSTRUCTION_PATH)
    if not os.path.exists(path):
        return None

    with open(path) as f:
        return f.read()


def invalidate_config_cache(cache: ConfigCache | None = None) -> None:
    """Invalidate all configuration caches.

    This forces the next load_* call to re-read from files.

    Args:
        cache: Optional cache instance. Uses global cache if not provided.
    """
    cache = cache or get_global_cache()
    cache.invalidate_all()


def is_config_cache_valid(key: str, cache: ConfigCache | None = None) -> bool:
    """Check if a specific config cache entry is still valid.

    Args:
        key: The cache key (use CACHE_KEY_* constants from flokoa.cache).
        cache: Optional cache instance. Uses global cache if not provided.

    Returns:
        True if the cache entry exists and is valid, False otherwise.
    """
    cache = cache or get_global_cache()
    return cache.is_valid(key)
