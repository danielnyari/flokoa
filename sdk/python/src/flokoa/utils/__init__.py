import json
import os
from glob import glob

from a2a.types import AgentCapabilities, AgentCard, AgentSkill

from flokoa.types import ModelConfig, ToolDefinition
from flokoa.types.agentcard import AgentCard as FlokoaAgentCard
from flokoa.types.agenttool import AgentToolSpec

TOOLS_PATH = "/etc/flokoa/tools/"
AGENT_CARD_PATH = "/etc/flokoa/agent-card.json"
MODEL_CONFIG_PATH = "/etc/flokoa/model.json"


def load_agent_card(url: str | None = None) -> AgentCard | None:
    """Load agent card from /etc/flokoa/agent-card.json.

    Args:
        url: Override URL for the agent card. If not provided, uses FLOKOA_AGENT_URL
             environment variable or defaults to empty string.

    Returns:
        AgentCard (a2a type) if the file exists, None otherwise.

    The JSON format matches the Kubernetes Agent CRD card structure.
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


def load_tools() -> list[ToolDefinition]:
    """Load tool definitions from /etc/flokoa/tools/.

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
    """
    if not os.path.exists(TOOLS_PATH):
        return []

    definitions: list[ToolDefinition] = []

    for filename in glob(os.path.join(TOOLS_PATH, "*.json")):
        with open(filename) as f:
            tool_cfg = json.load(f)
            tool_definition = ToolDefinition(
                name=tool_cfg["name"],
                spec=AgentToolSpec(**tool_cfg["spec"]),
                metadata=tool_cfg.get("metadata", None),
            )
            definitions.append(tool_definition)

    return definitions


def load_model_config() -> ModelConfig | None:
    """Load model configuration from /etc/flokoa/model.json.

    Returns:
        ModelConfig if the file exists, None otherwise.

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
    """
    if not os.path.exists(MODEL_CONFIG_PATH):
        return None

    with open(MODEL_CONFIG_PATH) as f:
        config_data = json.load(f)

    return ModelConfig.model_validate(config_data)
