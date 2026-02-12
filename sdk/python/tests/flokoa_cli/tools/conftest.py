
from ..fixtures import *


import pytest


@pytest.fixture
def tools_config():
    """Tool configuration in the new AgentToolSpec format."""
    return [
        {
            "name": "test_api_tool",
            "spec": {
                "type": "http-api",
                "description": "A test API tool",
                "inputSchema": {"type": "object", "properties": {"input": {"type": "string"}}},
                "outputSchema": {"type": "object", "properties": {"output": {"type": "string"}}},
                "httpApi": {"url": "https://api.example.com/test", "method": "POST"},
            },
        },
        {
            "name": "another_api_tool",
            "metadata": {"version": "2.0"},
            "spec": {
                "type": "http-api",
                "description": "Another API tool",
                "inputSchema": {"type": "object"},
                "outputSchema": {"type": "object"},
                "httpApi": {"url": "https://api.example.com/another", "method": "GET"},
            },
        },
    ]
