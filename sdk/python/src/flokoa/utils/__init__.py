
from flokoa.types import ToolDefinition

def load_tools() -> list[ToolDefinition]:
    """Load tool definitions from /etc/flokoa/tools.json."""
    import os
    import json

    tools_path = "/etc/flokoa/tools.json"
    if not os.path.exists(tools_path):
        return []

    with open(tools_path) as f:
        tools_config = json.load(f)

    definitions = [
        ToolDefinition(
            name=t['name'],
            description=t['description'],
            input_json_schema=t['inputSchema'],
            output_json_schema=t['outputSchema'],
            type=t.get('type', 'api'),
            metadata=t.get('metadata', None),
            spec=t.get('spec', {}),
        )
        for t in tools_config
    ]

    return definitions