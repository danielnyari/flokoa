import pytest


@pytest.fixture
def minimal_agent_card_data():
    """Minimal valid agent card data with required fields only."""
    return {
        "name": "Test Agent",
        "description": "A test agent for unit testing",
        "version": "1.0.0",
        "defaultInputModes": ["application/json"],
        "defaultOutputModes": ["application/json"],
        "capabilities": {"streaming": False},
        "skills": [
            {
                "id": "test-skill",
                "name": "Test Skill",
                "description": "A test skill",
                "tags": ["test", "example"],
            }
        ],
    }


@pytest.fixture
def full_agent_card_data():
    """Agent card data with all optional fields populated."""
    return {
        "name": "Full Test Agent",
        "description": "A fully configured test agent",
        "version": "2.0.0",
        "defaultInputModes": ["application/json", "text/plain"],
        "defaultOutputModes": ["application/json", "text/plain"],
        "capabilities": {
            "streaming": True,
            "pushNotifications": True,
            "stateTransitionHistory": True,
            "extensions": [
                {
                    "uri": "https://example.com/extension",
                    "description": "Test extension",
                    "required": False,
                    "params": {"key": "value"},
                }
            ],
        },
        "skills": [
            {
                "id": "skill-1",
                "name": "Primary Skill",
                "description": "The primary skill of this agent",
                "tags": ["primary", "main"],
                "examples": ["Example prompt 1", "Example prompt 2"],
                "inputModes": ["application/json"],
                "outputModes": ["text/plain"],
            },
            {
                "id": "skill-2",
                "name": "Secondary Skill",
                "description": "A secondary skill",
                "tags": ["secondary"],
            },
        ],
    }


@pytest.fixture
def agent_card_with_none_capabilities():
    """Agent card with None values in capabilities."""
    return {
        "name": "Agent with None Caps",
        "description": "An agent with None capability values",
        "version": "1.0.0",
        "defaultInputModes": ["application/json"],
        "defaultOutputModes": ["application/json"],
        "capabilities": {
            "streaming": None,
            "pushNotifications": None,
            "stateTransitionHistory": None,
        },
        "skills": [
            {
                "id": "basic-skill",
                "name": "Basic Skill",
                "description": "A basic skill",
                "tags": ["basic"],
            }
        ],
    }


# Model config fixtures
# These use the CRD-based schema (provider with type field, parameters structure)


@pytest.fixture
def minimal_model_config_data():
    """Minimal valid model config data with required fields only."""
    return {
        "provider": {"type": "openai"},
        "model": "gpt-4o",
    }


@pytest.fixture
def openai_model_config_data():
    """OpenAI model config with all fields populated."""
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
            "openai": {
                "serviceTier": "default",
            },
        },
    }


@pytest.fixture
def anthropic_model_config_data():
    """Anthropic model config."""
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
            },
        },
    }


@pytest.fixture
def google_model_config_data():
    """Google/Gemini model config."""
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
        },
    }


@pytest.fixture
def bedrock_model_config_data():
    """AWS Bedrock model config."""
    return {
        "provider": {
            "type": "bedrock",
            "bedrock": {
                "region": "us-east-1",
                "inferenceProfileARN": "arn:aws:bedrock:us-east-1:123456789:inference-profile/test",
            },
        },
        "model": "anthropic.claude-3-sonnet",
        "parameters": {
            "temperature": "0.7",
            "maxTokens": 4096,
        },
    }


@pytest.fixture
def model_config_with_settings():
    """Model config with parameters including stop sequences and seed."""
    return {
        "provider": {"type": "openai"},
        "model": "gpt-4o-mini",
        "parameters": {
            "temperature": "1.0",
            "maxTokens": 2048,
            "stopSequences": ["END", "STOP"],
            "seed": 42,
        },
    }


@pytest.fixture
def model_config_with_default_headers():
    """Model config with default headers."""
    return {
        "provider": {
            "type": "openai",
            "defaultHeaders": {
                "X-Request-Source": "flokoa",
                "X-Tenant-ID": "tenant-123",
            },
        },
        "model": "gpt-4o",
    }
