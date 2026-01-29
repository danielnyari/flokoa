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
            "stateTransitionHistroy": True,
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
            "stateTransitionHistroy": None,
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
