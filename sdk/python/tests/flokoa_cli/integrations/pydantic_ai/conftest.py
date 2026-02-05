"""Pytest fixtures for PydanticAI integration tests."""

import json
from unittest.mock import MagicMock

import pytest

from flokoa.types import ModelConfig, ToolDefinition
from flokoa.types.agenttool import AgentToolSpec, HttpApi, Method, Type


# =============================================================================
# Model Config Fixtures - JSON format matching /etc/flokoa/model.json
# =============================================================================


@pytest.fixture
def openai_model_config_json():
    """OpenAI model config in JSON format (as stored in model.json)."""
    return {
        "provider": {
            "type": "openai",
            "openai": {
                "baseURL": "https://api.openai.com/v1",
                "organizationID": "org-test-123",
                "timeoutSeconds": 120,
            },
        },
        "model": "gpt-4o",
        "parameters": {
            "temperature": "0.7",
            "maxTokens": 4096,
            "topP": "0.9",
            "frequencyPenalty": "0.5",
            "presencePenalty": "0.3",
            "seed": 42,
            "openai": {
                "reasoningEffort": "medium",
                "serviceTier": "default",
            },
        },
    }


@pytest.fixture
def anthropic_model_config_json():
    """Anthropic model config in JSON format."""
    return {
        "provider": {
            "type": "anthropic",
            "anthropic": {
                "baseURL": "https://api.anthropic.com",
                "timeoutSeconds": 90,
            },
        },
        "model": "claude-sonnet-4-20250514",
        "parameters": {
            "temperature": "0.5",
            "maxTokens": 8192,
            "anthropic": {
                "cacheInstructions": "5m",
                "cacheMessages": "5m",
            },
        },
    }


@pytest.fixture
def google_model_config_json():
    """Google/Gemini model config in JSON format."""
    return {
        "provider": {
            "type": "google",
            "google": {
                "location": "us-central1",
                "project": "test-project",
                "timeoutSeconds": 60,
            },
        },
        "model": "gemini-1.5-pro",
        "parameters": {
            "temperature": "0.9",
            "maxTokens": 2048,
            "google": {
                "safetySettings": [
                    {
                        "category": "HARM_CATEGORY_HARASSMENT",
                        "threshold": "BLOCK_MEDIUM_AND_ABOVE",
                    }
                ],
            },
        },
    }


@pytest.fixture
def bedrock_model_config_json():
    """AWS Bedrock model config in JSON format."""
    return {
        "provider": {
            "type": "bedrock",
            "bedrock": {
                "region": "us-east-1",
                "inferenceProfileARN": "arn:aws:bedrock:us-east-1:123456789:inference-profile/test",
            },
        },
        "model": "anthropic.claude-3-sonnet-20240229-v1:0",
        "parameters": {
            "temperature": "0.7",
            "maxTokens": 4096,
            "bedrock": {
                "cacheInstructions": True,
                "performanceConfiguration": {"latency": "optimized"},
            },
        },
    }


@pytest.fixture
def minimal_model_config_json():
    """Minimal model config with only required fields."""
    return {
        "provider": {"type": "openai"},
        "model": "gpt-4o-mini",
    }


@pytest.fixture
def model_config_with_thinking_json():
    """Anthropic model config with extended thinking enabled."""
    return {
        "provider": {
            "type": "anthropic",
            "anthropic": {"timeoutSeconds": 180},
        },
        "model": "claude-sonnet-4-20250514",
        "parameters": {
            "maxTokens": 16384,
            "anthropic": {
                "thinking": {
                    "type": "enabled",
                    "budgetTokens": 8192,
                },
            },
        },
    }


@pytest.fixture
def model_config_with_default_headers_json():
    """Model config with default headers."""
    return {
        "provider": {
            "type": "openai",
            "defaultHeaders": {
                "X-Request-Source": "flokoa-test",
                "X-Tenant-ID": "tenant-test-123",
            },
        },
        "model": "gpt-4o",
        "parameters": {"temperature": "0.7"},
    }


# =============================================================================
# Tool Definition Fixtures
# =============================================================================


@pytest.fixture
def api_tool_definition():
    """Create a sample API tool definition."""
    return ToolDefinition(
        name="get_weather",
        spec=AgentToolSpec(
            type=Type.http_api,
            description="Get the current weather for a location",
            inputSchema={
                "type": "object",
                "properties": {
                    "location": {"type": "string", "description": "The city name"},
                },
                "required": ["location"],
            },
            outputSchema={
                "type": "object",
                "properties": {
                    "temperature": {"type": "number"},
                    "condition": {"type": "string"},
                },
            },
            httpApi=HttpApi(url="https://api.weather.com/current", method=Method.get),
        ),
    )


@pytest.fixture
def multiple_tool_definitions():
    """Create multiple tool definitions for testing."""
    return [
        ToolDefinition(
            name="get_weather",
            spec=AgentToolSpec(
                type=Type.http_api,
                description="Get the current weather for a location",
                inputSchema={
                    "type": "object",
                    "properties": {"location": {"type": "string"}},
                    "required": ["location"],
                },
                outputSchema={"type": "object"},
                httpApi=HttpApi(url="https://api.weather.com/current", method=Method.get),
            ),
        ),
        ToolDefinition(
            name="send_email",
            spec=AgentToolSpec(
                type=Type.http_api,
                description="Send an email to a recipient",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "to": {"type": "string"},
                        "subject": {"type": "string"},
                        "body": {"type": "string"},
                    },
                    "required": ["to", "subject", "body"],
                },
                outputSchema={"type": "object"},
                httpApi=HttpApi(url="https://api.email.com/send", method=Method.post),
            ),
        ),
    ]


# =============================================================================
# Mock Fixtures
# =============================================================================


@pytest.fixture
def mock_http_api(monkeypatch):
    """Mock the HTTP API calls to avoid real network requests."""
    mock = MagicMock()

    async def mock_call_http_api_tool(endpoint: str, method: str, params: dict):
        mock(endpoint=endpoint, method=method, params=params)
        if "weather" in endpoint:
            return {"temperature": 20, "condition": "sunny", "location": params.get("location", "unknown")}
        elif "email" in endpoint:
            return {"sent": True, "to": params.get("to"), "subject": params.get("subject")}
        return {"success": True, "params": params}

    monkeypatch.setattr("flokoa.tools.call_http_api_tool", mock_call_http_api_tool)
    return mock


# =============================================================================
# File-based Config Fixtures (following tool loading pattern)
# =============================================================================


@pytest.fixture
def model_config_file(tmp_path, openai_model_config_json, monkeypatch):
    """Create a model config file and patch the path.

    This follows the same pattern as tool loading tests.
    """
    import flokoa.utils as utils_module

    model_config_path = tmp_path / "model.json"
    model_config_path.write_text(json.dumps(openai_model_config_json))
    monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))
    return model_config_path


@pytest.fixture
def tools_file(tmp_path, multiple_tool_definitions, monkeypatch, mock_http_api):
    """Create tools configuration file and patch load_tools."""
    tools_config = [
        {
            "name": td.name,
            "spec": {
                "type": td.spec.type.value,
                "description": td.spec.description,
                "inputSchema": td.spec.input_schema,
                "outputSchema": td.spec.output_schema,
                "httpApi": {
                    "url": td.spec.http_api.url,
                    "method": td.spec.http_api.method.value,
                },
            },
        }
        for td in multiple_tool_definitions
    ]

    tools_file = tmp_path / "tools.json"
    tools_file.write_text(json.dumps(tools_config))

    def patched_load_tools(use_cache=True, cache=None):
        return multiple_tool_definitions

    monkeypatch.setattr("flokoa.agent_executor.load_tools", patched_load_tools)
    return tools_file
