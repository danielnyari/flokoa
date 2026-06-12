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

"""The flokoa.OpenAPI capability: config validation and Agent.from_spec round-trip."""

from __future__ import annotations

import httpx
import pytest
from flokoa_openapi import OpenAPI, OpenAPIToolset
from pydantic_ai import Agent
from pydantic_ai.models.test import TestModel

pytestmark = pytest.mark.anyio


class TestCapabilityConfig:
    def test_spec_is_required(self):
        with pytest.raises(ValueError, match="requires `spec`"):
            OpenAPI()

    def test_serialization_name(self):
        assert OpenAPI.get_serialization_name() == "flokoa.OpenAPI"

    def test_get_toolset_returns_configured_toolset(self, petstore_spec):
        capability = OpenAPI(spec=petstore_spec, prefix="petstore", defer_tools="none")
        toolset = capability.get_toolset()
        assert isinstance(toolset, OpenAPIToolset)
        names = {td.name for td in toolset.tool_definitions}
        assert "petstore_list_pets" in names
        assert not any(td.defer_loading for td in toolset.tool_definitions)

    def test_auth_config_builds_scheme_and_credential(self, petstore_spec):
        capability = OpenAPI(
            spec=petstore_spec,
            auth={
                "scheme": {"type": "apiKey", "in": "header", "name": "X-API-Key"},
                "credential": {"auth_type": "apiKey", "api_key": "k"},
            },
        )
        toolset = capability.get_toolset()
        entry = toolset._tools["list_pets"]
        assert entry.auth_scheme is not None
        assert entry.auth_credential is not None
        assert entry.auth_credential.api_key == "k"

    def test_auth_config_without_scheme_raises(self, petstore_spec):
        capability = OpenAPI(spec=petstore_spec, auth={"credential": {"auth_type": "apiKey", "api_key": "k"}})
        with pytest.raises(ValueError, match="requires a `scheme`"):
            capability.get_toolset()


class TestFromSpecRoundTrip:
    """The capability must hydrate from a spec dict via Agent.from_spec with
    custom_capability_types=[OpenAPI] — exactly how the flokoa runner
    instantiates capabilities from a compiled AgentSpec."""

    def test_agent_from_spec_builds_capability(self, petstore_spec):
        spec_doc = {
            "model": "test",
            "instructions": "You are a pet store assistant.",
            "capabilities": [
                {
                    "flokoa.OpenAPI": {
                        "spec": petstore_spec,
                        "prefix": "petstore",
                        "defer_tools": "none",
                    }
                }
            ],
        }
        agent = Agent.from_spec(spec_doc, custom_capability_types=[OpenAPI])
        assert agent is not None

    async def test_from_spec_tools_reach_the_model(self, petstore_spec):
        spec_doc = {
            "capabilities": [
                {
                    "flokoa.OpenAPI": {
                        "spec": petstore_spec,
                        "defer_tools": "none",
                    }
                }
            ],
        }
        model = TestModel(call_tools=[])
        agent = Agent.from_spec(spec_doc, model=model, custom_capability_types=[OpenAPI])

        await agent.run("hello")
        params = model.last_model_request_parameters
        assert params is not None
        tool_names = {td.name for td in params.function_tools}
        assert {"list_pets", "create_pet", "get_pet_by_id"} <= tool_names

    async def test_from_spec_invalid_capability_config_fails_at_hydration(self):
        spec_doc = {"capabilities": [{"flokoa.OpenAPI": {}}]}
        with pytest.raises(Exception, match="requires `spec`"):
            Agent.from_spec(spec_doc, model=TestModel(), custom_capability_types=[OpenAPI])

    async def test_from_spec_end_to_end_tool_call(self, petstore_spec):
        """TestModel calls the tool; the toolset executes a (mocked) HTTP request."""
        captured: list[httpx.Request] = []

        def handler(request: httpx.Request) -> httpx.Response:
            captured.append(request)
            return httpx.Response(200, json=[{"id": 1, "name": "Buddy"}])

        capability = OpenAPI(spec=petstore_spec, defer_tools="none", allowed_operations=["list_pets"])
        toolset = capability.get_toolset()
        toolset._transport = httpx.MockTransport(handler)

        model = TestModel(call_tools=["list_pets"])
        agent = Agent(model, toolsets=[toolset])

        result = await agent.run("list the pets")
        assert len(captured) == 1
        assert captured[0].url.path == "/pets"
        assert result.output


class TestToolSearchSurface:
    """With deferred tools, the model-visible surface is the search tool, not
    the 500 operation schemas (discovery + sandboxing are upstream's job)."""

    async def test_deferred_tools_hidden_from_model(self, petstore_spec):
        spec_doc = {
            "capabilities": [
                {
                    "flokoa.OpenAPI": {
                        "spec": petstore_spec,
                        "defer_tools": "all",
                    }
                }
            ],
        }
        model = TestModel(call_tools=[])
        agent = Agent.from_spec(spec_doc, model=model, custom_capability_types=[OpenAPI])

        await agent.run("hello")
        params = model.last_model_request_parameters
        assert params is not None
        wire_names = {td.name for td in params.function_tools if not td.defer_loading}
        assert "list_pets" not in wire_names
        assert "create_pet" not in wire_names
        # Core ToolSearch auto-injects a discovery surface for deferred tools.
        all_names = {td.name for td in params.function_tools}
        assert any("search" in name for name in all_names), all_names
