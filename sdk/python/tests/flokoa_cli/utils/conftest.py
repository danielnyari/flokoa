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
# These use PydanticAI's ModelSettings field names (snake_case, flat structure)


@pytest.fixture
def minimal_model_config_data():
    """Minimal valid model config data with required fields only."""
    return {
        "provider": "openai",
        "model": "gpt-4o",
    }


@pytest.fixture
def openai_model_config_data():
    """OpenAI model config with all fields populated."""
    return {
        "provider": "openai",
        "model": "gpt-4o",
        "config": {
            "baseURL": "https://api.openai.com/v1",
            "organizationID": "org-12345",
            "timeoutSeconds": 120,
            "defaultHeaders": {
                "X-Custom-Header": "custom-value",
            },
        },
        "settings": {
            "temperature": 0.7,
            "max_tokens": 4096,
            "top_p": 0.9,
            "frequency_penalty": 0.5,
            "presence_penalty": 0.3,
        },
    }


@pytest.fixture
def anthropic_model_config_data():
    """Anthropic model config."""
    return {
        "provider": "anthropic",
        "model": "claude-sonnet-4-20250514",
        "config": {
            "baseURL": "https://api.anthropic.com",
            "timeoutSeconds": 90,
        },
        "settings": {
            "temperature": 0.5,
            "max_tokens": 8192,
        },
    }


@pytest.fixture
def ollama_model_config_data():
    """Ollama model config for local models."""
    return {
        "provider": "ollama",
        "model": "llama3.2",
        "config": {
            "host": "http://localhost:11434",
        },
        "settings": {
            "temperature": 0.8,
        },
    }


@pytest.fixture
def azure_openai_model_config_data():
    """Azure OpenAI model config."""
    return {
        "provider": "azure-openai",
        "model": "gpt-4o",
        "config": {
            "endpoint": "https://myresource.openai.azure.com",
            "deploymentName": "my-gpt4o-deployment",
            "apiVersion": "2024-02-15-preview",
        },
        "settings": {
            "temperature": 0.7,
            "max_tokens": 4096,
        },
    }


@pytest.fixture
def gemini_model_config_data():
    """Gemini model config."""
    return {
        "provider": "gemini",
        "model": "gemini-1.5-pro",
        "config": {
            "timeoutSeconds": 60,
        },
        "settings": {
            "temperature": 0.9,
            "seed": 42,
        },
    }


@pytest.fixture
def model_config_with_settings():
    """Model config with settings but no provider-specific config."""
    return {
        "provider": "openai",
        "model": "gpt-4o-mini",
        "settings": {
            "temperature": 1.0,
            "max_tokens": 2048,
            "stop_sequences": ["END", "STOP"],
            "seed": 42,
        },
    }


@pytest.fixture
def model_config_with_default_headers():
    """Model config with default headers."""
    return {
        "provider": "openai",
        "model": "gpt-4o",
        "config": {
            "defaultHeaders": {
                "X-Request-Source": "flokoa",
                "X-Tenant-ID": "tenant-123",
            },
        },
    }
