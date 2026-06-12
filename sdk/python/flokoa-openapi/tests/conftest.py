# Copyright 2026 Flokoa Contributors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from __future__ import annotations

from typing import Any

import pytest

# TEST-NET-3 (RFC 5737): a public, never-routed address — passes SSRF checks
# without a DNS lookup, and MockTransport never opens a real connection.
PUBLIC_BASE_URL = "http://203.0.113.10"


@pytest.fixture
def anyio_backend():
    return "asyncio"


def make_petstore_spec(base_url: str = PUBLIC_BASE_URL) -> dict[str, Any]:
    """A compact petstore covering params, bodies, responses, and security."""
    pet_schema = {
        "type": "object",
        "properties": {
            "id": {"type": "integer"},
            "name": {"type": "string"},
            "tag": {"type": "string"},
        },
        "required": ["id", "name"],
    }
    return {
        "openapi": "3.0.0",
        "info": {"title": "Petstore", "version": "1.0.0"},
        "servers": [{"url": base_url}],
        "components": {
            "schemas": {"Pet": pet_schema},
            "securitySchemes": {
                "api_key": {"type": "apiKey", "in": "header", "name": "X-API-Key"},
            },
        },
        "paths": {
            "/pets": {
                "get": {
                    "operationId": "listPets",
                    "summary": "List all pets",
                    "description": "Returns every pet, optionally limited.",
                    "parameters": [
                        {"name": "limit", "in": "query", "schema": {"type": "integer"}},
                    ],
                    "responses": {
                        "200": {
                            "description": "A list of pets",
                            "content": {
                                "application/json": {
                                    "schema": {"type": "array", "items": {"$ref": "#/components/schemas/Pet"}}
                                }
                            },
                        }
                    },
                },
                "post": {
                    "operationId": "createPet",
                    "summary": "Create a pet",
                    "requestBody": {
                        "content": {
                            "application/json": {
                                "schema": {
                                    "type": "object",
                                    "properties": {
                                        "name": {"type": "string"},
                                        "tag": {"type": "string"},
                                    },
                                    "required": ["name"],
                                }
                            }
                        }
                    },
                    "responses": {
                        "201": {
                            "description": "Created",
                            "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Pet"}}},
                        }
                    },
                },
            },
            "/pets/{petId}": {
                "get": {
                    "operationId": "getPetById",
                    "summary": "Get a pet by id",
                    "parameters": [
                        {"name": "petId", "in": "path", "required": True, "schema": {"type": "integer"}},
                    ],
                    "responses": {
                        "200": {
                            "description": "A pet",
                            "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Pet"}}},
                        }
                    },
                },
                "delete": {
                    "operationId": "deletePet",
                    "parameters": [
                        {"name": "petId", "in": "path", "required": True, "schema": {"type": "integer"}},
                    ],
                    "responses": {"204": {"description": "Deleted"}},
                },
            },
            "/pets/{petId}/photo": {
                "get": {
                    "operationId": "getPetPhoto",
                    "parameters": [
                        {"name": "petId", "in": "path", "required": True, "schema": {"type": "integer"}},
                    ],
                    "responses": {
                        "200": {
                            "description": "A photo",
                            "content": {"image/png": {"schema": {"type": "string", "format": "binary"}}},
                        }
                    },
                },
            },
        },
    }


def make_big_spec(operation_count: int, base_url: str = PUBLIC_BASE_URL) -> dict[str, Any]:
    """A generated spec with ``operation_count`` GET operations."""
    paths = {}
    for i in range(operation_count):
        paths[f"/things/{i}"] = {
            "get": {
                "operationId": f"getThing{i}",
                "summary": f"Get thing {i}",
                "responses": {
                    "200": {
                        "description": "ok",
                        "content": {
                            "application/json": {
                                "schema": {"type": "object", "properties": {"id": {"type": "integer"}}}
                            }
                        },
                    }
                },
            }
        }
    return {
        "openapi": "3.0.0",
        "info": {"title": "Big API", "version": "1.0.0"},
        "servers": [{"url": base_url}],
        "paths": paths,
    }


@pytest.fixture
def petstore_spec() -> dict[str, Any]:
    return make_petstore_spec()
