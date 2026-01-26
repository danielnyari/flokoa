import json
import os
from glob import glob

from flokoa.types import ToolDefinition
from flokoa.types.agenttool import AgentToolSpec

TOOLS_PATH = "/etc/flokoa/tools/"


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
