"""Pytest fixtures for PydanticAI integration tests."""

import json

import pytest

from flokoa.types import ToolDefinition
from flokoa.types.agenttool import AgentToolSpec, OpenApi, OpenApiSchema, Type


WEATHER_API_SPEC = {
    "openapi": "3.0.0",
    "info": {"title": "Weather API", "version": "1.0.0"},
    "servers": [{"url": "https://api.weather.com"}],
    "paths": {
        "/current": {
            "get": {
                "operationId": "getWeather",
                "summary": "Get the current weather for a location",
                "parameters": [
                    {
                        "name": "location",
                        "in": "query",
                        "description": "The city name",
                        "required": True,
                        "schema": {"type": "string"},
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "content": {
                            "application/json": {
                                "schema": {
                                    "type": "object",
                                    "properties": {
                                        "temperature": {"type": "number"},
                                        "condition": {"type": "string"},
                                    },
                                }
                            }
                        },
                    }
                },
            }
        }
    },
}

EMAIL_API_SPEC = {
    "openapi": "3.0.0",
    "info": {"title": "Email API", "version": "1.0.0"},
    "servers": [{"url": "https://api.email.com"}],
    "paths": {
        "/send": {
            "post": {
                "operationId": "sendEmail",
                "summary": "Send an email to a recipient",
                "requestBody": {
                    "content": {
                        "application/json": {
                            "schema": {
                                "type": "object",
                                "properties": {
                                    "to": {"type": "string"},
                                    "subject": {"type": "string"},
                                    "body": {"type": "string"},
                                },
                                "required": ["to", "subject", "body"],
                            }
                        }
                    }
                },
                "responses": {"200": {"description": "OK"}},
            }
        }
    },
}


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
    """Create a sample API tool definition using OpenAPI spec."""
    return ToolDefinition(
        name="weather_api",
        spec=AgentToolSpec(
            type=Type.openapi,
            description="Get the current weather for a location",
            openApi=OpenApi(
                openApiSchema=OpenApiSchema(value=WEATHER_API_SPEC),
                url="https://api.weather.com",
            ),
        ),
    )


@pytest.fixture
def multiple_tool_definitions():
    """Create multiple tool definitions for testing."""
    return [
        ToolDefinition(
            name="weather_api",
            spec=AgentToolSpec(
                type=Type.openapi,
                description="Get the current weather for a location",
                openApi=OpenApi(
                    openApiSchema=OpenApiSchema(value=WEATHER_API_SPEC),
                    url="https://api.weather.com",
                ),
            ),
        ),
        ToolDefinition(
            name="email_api",
            spec=AgentToolSpec(
                type=Type.openapi,
                description="Send an email to a recipient",
                openApi=OpenApi(
                    openApiSchema=OpenApiSchema(value=EMAIL_API_SPEC),
                    url="https://api.email.com",
                ),
            ),
        ),
    ]


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
def tools_file(tmp_path, multiple_tool_definitions, monkeypatch):
    """Create tools configuration file and patch load_tools."""
    tools_config = [
        {
            "name": td.name,
            "spec": {
                "type": td.spec.type.value,
                "description": td.spec.description,
                "openApi": {
                    "openApiSchema": {"value": td.spec.open_api.open_api_schema.value},
                    "url": td.spec.open_api.url,
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
