from flokoa.types import ToolDefinition
from flokoa.types.agenttool import AgentToolSpec


def load_tools() -> list[ToolDefinition]:
    """Load tool definitions from /etc/flokoa/tools.json.

    The JSON format matches the Kubernetes AgentTool CRD structure:
    [
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
    ]
    """
    import json
    import os

    tools_path = "/etc/flokoa/tools.json"
    if not os.path.exists(tools_path):
        return []

    with open(tools_path) as f:
        tools_config = json.load(f)

    definitions = [
        ToolDefinition(
            name=t["name"],
            spec=AgentToolSpec(**t["spec"]),
            metadata=t.get("metadata", None),
        )
        for t in tools_config
    ]

    return definitions
