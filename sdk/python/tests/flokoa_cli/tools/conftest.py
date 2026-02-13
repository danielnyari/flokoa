
from ..fixtures import *


import pytest


MINIMAL_OPENAPI_SPEC = {
    "openapi": "3.0.0",
    "info": {"title": "Test API", "version": "1.0.0"},
    "servers": [{"url": "https://api.example.com"}],
    "paths": {
        "/test": {
            "post": {
                "operationId": "testEndpoint",
                "summary": "A test API tool",
                "requestBody": {
                    "content": {
                        "application/json": {
                            "schema": {
                                "type": "object",
                                "properties": {"input": {"type": "string"}},
                            }
                        }
                    }
                },
                "responses": {
                    "200": {
                        "description": "OK",
                        "content": {
                            "application/json": {
                                "schema": {
                                    "type": "object",
                                    "properties": {"output": {"type": "string"}},
                                }
                            }
                        },
                    }
                },
            }
        },
        "/another": {
            "get": {
                "operationId": "anotherEndpoint",
                "summary": "Another API tool",
                "responses": {"200": {"description": "OK"}},
            }
        },
    },
}


@pytest.fixture
def tools_config():
    """Tool configuration in the OpenAPI AgentToolSpec format."""
    return [
        {
            "name": "test_api_tool",
            "spec": {
                "type": "openapi",
                "description": "A test API tool",
                "openApi": {
                    "openApiSchema": {"value": MINIMAL_OPENAPI_SPEC},
                    "url": "https://api.example.com",
                },
            },
        },
        {
            "name": "another_api_tool",
            "metadata": {"version": "2.0"},
            "spec": {
                "type": "openapi",
                "description": "Another API tool",
                "openApi": {
                    "openApiSchema": {"value": MINIMAL_OPENAPI_SPEC},
                    "url": "https://api.example.com",
                },
            },
        },
    ]
