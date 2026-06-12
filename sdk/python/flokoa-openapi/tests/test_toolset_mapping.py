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

"""Spec → ToolDefinition mapping: names, schemas, return schemas, defer flags."""

from __future__ import annotations

import pytest
from flokoa_openapi.toolset import TOOL_METADATA_KEY, OpenAPIToolset

from .conftest import make_big_spec, make_petstore_spec


def _defs_by_name(toolset: OpenAPIToolset):
    return {td.name: td for td in toolset.tool_definitions}


class TestToolDefinitionMapping:
    def test_one_tool_per_operation(self, petstore_spec):
        toolset = OpenAPIToolset(petstore_spec)
        names = {td.name for td in toolset.tool_definitions}
        assert names == {"list_pets", "create_pet", "get_pet_by_id", "delete_pet", "get_pet_photo"}

    def test_description_combines_summary_and_description(self, petstore_spec):
        defs = _defs_by_name(OpenAPIToolset(petstore_spec))
        assert defs["list_pets"].description == "List all pets\n\nReturns every pet, optionally limited."
        assert defs["create_pet"].description == "Create a pet"

    def test_parameters_json_schema_from_operation(self, petstore_spec):
        defs = _defs_by_name(OpenAPIToolset(petstore_spec))
        schema = defs["get_pet_by_id"].parameters_json_schema
        assert schema["type"] == "object"
        assert "pet_id" in schema["properties"]
        assert schema["required"] == ["pet_id"]

    def test_request_body_properties_become_parameters(self, petstore_spec):
        defs = _defs_by_name(OpenAPIToolset(petstore_spec))
        schema = defs["create_pet"].parameters_json_schema
        assert set(schema["properties"]) >= {"name", "tag"}

    def test_return_schema_from_2xx_json_response(self, petstore_spec):
        defs = _defs_by_name(OpenAPIToolset(petstore_spec))
        # object response (201)
        created = defs["create_pet"].return_schema
        assert created is not None
        assert created["type"] == "object"
        assert "name" in created["properties"]
        # array response (200)
        listed = defs["list_pets"].return_schema
        assert listed is not None
        assert listed["type"] == "array"
        assert listed["items"]["type"] == "object"

    def test_return_schema_none_without_json_response(self, petstore_spec):
        defs = _defs_by_name(OpenAPIToolset(petstore_spec))
        assert defs["delete_pet"].return_schema is None  # 204, no content
        assert defs["get_pet_photo"].return_schema is None  # image/png only

    def test_metadata_flag_set(self, petstore_spec):
        for td in OpenAPIToolset(petstore_spec).tool_definitions:
            assert td.metadata == {TOOL_METADATA_KEY: True}

    def test_auth_params_never_in_model_visible_schema(self, petstore_spec):
        from flokoa_common.auth.helpers import INTERNAL_AUTH_PREFIX

        toolset = OpenAPIToolset(
            petstore_spec,
            auth_scheme=None,
            auth_credential=None,
        )
        for td in toolset.tool_definitions:
            for prop in td.parameters_json_schema["properties"]:
                assert not prop.startswith(INTERNAL_AUTH_PREFIX)
                assert prop not in ("X-API-Key", "Authorization")


class TestDeferLoading:
    def test_defer_all(self, petstore_spec):
        toolset = OpenAPIToolset(petstore_spec, defer_loading="all")
        assert all(td.defer_loading for td in toolset.tool_definitions)

    def test_defer_none(self, petstore_spec):
        toolset = OpenAPIToolset(petstore_spec, defer_loading="none")
        assert not any(td.defer_loading for td in toolset.tool_definitions)

    def test_auto_below_threshold_stays_native(self, petstore_spec):
        toolset = OpenAPIToolset(petstore_spec, defer_loading="auto", defer_threshold=25)
        assert not any(td.defer_loading for td in toolset.tool_definitions)

    def test_auto_above_threshold_defers(self, petstore_spec):
        toolset = OpenAPIToolset(petstore_spec, defer_loading="auto", defer_threshold=3)
        assert all(td.defer_loading for td in toolset.tool_definitions)

    def test_invalid_mode_raises(self, petstore_spec):
        with pytest.raises(ValueError, match="Invalid defer_loading mode"):
            OpenAPIToolset(petstore_spec, defer_loading="sometimes")  # ty: ignore[invalid-argument-type]

    def test_500_operation_spec_defers_everything(self):
        """The two-tools-of-context property: every definition is deferred —
        surfacing only search_tools (+ run_code under CodeMode) is upstream's
        job from here."""
        toolset = OpenAPIToolset(make_big_spec(500), defer_loading="auto")
        defs = toolset.tool_definitions
        assert len(defs) == 500
        assert all(td.defer_loading for td in defs)
        # Typed signatures stay available for CodeMode sandbox rendering.
        assert all(td.return_schema is not None for td in defs)


class TestFilteringAndNaming:
    def test_allowed_operations_filter(self, petstore_spec):
        toolset = OpenAPIToolset(petstore_spec, allowed_operations=["list_pets", "get_pet_by_id"])
        assert {td.name for td in toolset.tool_definitions} == {"list_pets", "get_pet_by_id"}

    def test_prefix_applied(self, petstore_spec):
        toolset = OpenAPIToolset(petstore_spec, prefix="petstore")
        names = {td.name for td in toolset.tool_definitions}
        assert "petstore_list_pets" in names
        assert all(name.startswith("petstore_") for name in names)

    def test_names_are_valid_identifiers(self, petstore_spec):
        petstore_spec["paths"]["/odd"] = {
            "get": {
                "operationId": "123 weird-Name!",
                "responses": {"200": {"description": "ok"}},
            }
        }
        toolset = OpenAPIToolset(petstore_spec)
        for td in toolset.tool_definitions:
            assert td.name.isidentifier(), td.name

    def test_spec_as_json_string(self, petstore_spec):
        import json

        toolset = OpenAPIToolset(json.dumps(petstore_spec))
        assert len(toolset.tool_definitions) == 5

    def test_spec_as_yaml_string(self, petstore_spec):
        import yaml

        toolset = OpenAPIToolset(yaml.safe_dump(petstore_spec))
        assert len(toolset.tool_definitions) == 5

    def test_base_url_override_preserves_relative_server_path(self):
        spec = make_petstore_spec(base_url="/api/v3")
        toolset = OpenAPIToolset(spec, base_url="https://api.example.com")
        entry = toolset._tools["list_pets"]
        assert entry.endpoint.base_url == "https://api.example.com/api/v3"
