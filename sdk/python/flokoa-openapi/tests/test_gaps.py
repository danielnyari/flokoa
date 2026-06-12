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

"""Gap-filling tests: coverage items not addressed by the primary suites.

Covers:
- 204 No Content response in call_tool execution
- Malformed JSON body on JSON content-type response
- XML / application/xml and +json content-type routing
- Header parameter placement (vs query/path already tested)
- auto threshold boundary: exactly at and one above DEFAULT_DEFER_THRESHOLD (25)
- YAML spec that parses to a non-dict raises TypeError
- Capability: invalid auth scheme dict raises ValueError at get_toolset()
- Capability repr: auth and header values are redacted
- Two OpenAPI capabilities on one agent share no toolset_id clash
- exchanger error (AuthCredentialMissingError) is returned as a sanitised
  {"error": ...} envelope from call_tool
"""

from __future__ import annotations

import json
from typing import Any

import httpx
import pytest
import yaml
from flokoa_openapi import OpenAPI, OpenAPIToolset
from flokoa_openapi.toolset import DEFAULT_DEFER_THRESHOLD
from pydantic_ai._run_context import RunContext
from pydantic_ai.models.test import TestModel
from pydantic_ai.usage import RunUsage

from .conftest import make_petstore_spec

pytestmark = pytest.mark.anyio


def _ctx() -> RunContext[Any]:
    return RunContext(deps=None, model=TestModel(), usage=RunUsage())


def _capture_transport(
    captured: list[httpx.Request],
    response_factory=None,
) -> httpx.MockTransport:
    def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        if response_factory is not None:
            return response_factory(request)
        return httpx.Response(200, json={"ok": True})

    return httpx.MockTransport(handler)


async def _call(toolset: OpenAPIToolset, tool_name: str, /, **kwargs: Any) -> Any:
    ctx = _ctx()
    tools = await toolset.get_tools(ctx)
    async with toolset:
        return await toolset.call_tool(tool_name, kwargs, ctx, tools[tool_name])


# ---------------------------------------------------------------------------
# 204 No Content
# ---------------------------------------------------------------------------


class TestNoContent:
    async def test_204_delete_returns_empty_dict(self, petstore_spec):
        """A 204 response has no body; _parse_response must not raise."""
        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(
            petstore_spec,
            transport=_capture_transport(
                captured,
                lambda request: httpx.Response(204, content=b""),
            ),
        )
        result = await _call(toolset, "delete_pet", pet_id=42)
        # 204 doesn't raise for status; empty body falls through to text fallback
        assert isinstance(result, dict)
        assert "error" not in result or result.get("error") is None


# ---------------------------------------------------------------------------
# Malformed JSON body on application/json content-type
# ---------------------------------------------------------------------------


class TestMalformedJsonBody:
    async def test_malformed_json_falls_back_to_text(self, petstore_spec):
        """If Content-Type says JSON but body isn't valid JSON, return {'text': ...}."""
        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(
            petstore_spec,
            transport=_capture_transport(
                captured,
                lambda request: httpx.Response(
                    200,
                    content=b"not-valid-json{{{",
                    headers={"content-type": "application/json"},
                ),
            ),
        )
        result = await _call(toolset, "list_pets")
        # Falls back to the text branch of _parse_response
        assert "text" in result
        assert result["text"] == "not-valid-json{{{"


# ---------------------------------------------------------------------------
# XML and +json content-type routing
# ---------------------------------------------------------------------------


class TestXmlAndPlusJsonContentType:
    async def test_application_xml_returns_text_and_mime(self, petstore_spec):
        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(
            petstore_spec,
            transport=_capture_transport(
                captured,
                lambda request: httpx.Response(
                    200,
                    content=b"<root><item>1</item></root>",
                    headers={"content-type": "application/xml"},
                ),
            ),
        )
        result = await _call(toolset, "list_pets")
        assert "text" in result
        assert result["content_type"] == "application/xml"
        assert "<root>" in result["text"]

    async def test_plus_json_content_type_is_parsed_as_json(self, petstore_spec):
        """media types ending in +json (e.g. application/vnd.api+json) are parsed as JSON."""
        captured: list[httpx.Request] = []
        payload = {"data": [{"type": "pet", "id": "1"}]}
        toolset = OpenAPIToolset(
            petstore_spec,
            transport=_capture_transport(
                captured,
                lambda request: httpx.Response(
                    200,
                    content=json.dumps(payload).encode(),
                    headers={"content-type": "application/vnd.api+json"},
                ),
            ),
        )
        result = await _call(toolset, "list_pets")
        assert result == payload


# ---------------------------------------------------------------------------
# Header parameter placement
# ---------------------------------------------------------------------------


class TestHeaderParamPlacement:
    """Verify that header-located parameters are sent as request headers."""

    def _spec_with_header_param(self) -> dict[str, Any]:
        """Petstore variant with a header parameter on GET /pets."""
        base = make_petstore_spec()
        base["paths"]["/pets"]["get"]["parameters"].append(
            {
                "name": "X-Trace-Id",
                "in": "header",
                "required": False,
                "schema": {"type": "string"},
            }
        )
        return base

    async def test_header_param_is_sent_as_request_header(self):
        captured: list[httpx.Request] = []
        spec = self._spec_with_header_param()
        toolset = OpenAPIToolset(spec, transport=_capture_transport(captured))

        await _call(toolset, "list_pets", x_trace_id="trace-123")
        assert captured[0].headers.get("x-trace-id") == "trace-123"

    async def test_header_param_omitted_when_not_provided(self):
        captured: list[httpx.Request] = []
        spec = self._spec_with_header_param()
        toolset = OpenAPIToolset(spec, transport=_capture_transport(captured))

        await _call(toolset, "list_pets")
        # The optional header param should not be present (no default, not provided)
        assert "x-trace-id" not in captured[0].headers


# ---------------------------------------------------------------------------
# auto defer threshold boundary: exactly at 25 vs one above
# ---------------------------------------------------------------------------


class TestDeferAutoThresholdBoundary:
    """DEFAULT_DEFER_THRESHOLD == 25; condition is operation_count > threshold."""

    def _spec_with_n_ops(self, n: int) -> dict[str, Any]:
        paths = {}
        for i in range(n):
            paths[f"/ops/{i}"] = {
                "get": {
                    "operationId": f"getOp{i}",
                    "summary": f"Get op {i}",
                    "responses": {
                        "200": {
                            "description": "ok",
                            "content": {"application/json": {"schema": {"type": "object"}}},
                        }
                    },
                }
            }
        return {
            "openapi": "3.0.0",
            "info": {"title": "Test", "version": "1.0.0"},
            "servers": [{"url": "http://203.0.113.10"}],
            "paths": paths,
        }

    def test_exactly_at_threshold_stays_native(self):
        """25 operations with threshold=25: 25 > 25 is False → no deferral."""
        spec = self._spec_with_n_ops(DEFAULT_DEFER_THRESHOLD)
        toolset = OpenAPIToolset(spec, defer_loading="auto", defer_threshold=DEFAULT_DEFER_THRESHOLD)
        assert not any(td.defer_loading for td in toolset.tool_definitions)

    def test_one_above_threshold_defers_all(self):
        """26 operations with threshold=25: 26 > 25 is True → all deferred."""
        spec = self._spec_with_n_ops(DEFAULT_DEFER_THRESHOLD + 1)
        toolset = OpenAPIToolset(spec, defer_loading="auto", defer_threshold=DEFAULT_DEFER_THRESHOLD)
        assert all(td.defer_loading for td in toolset.tool_definitions)

    def test_zero_operations_stays_native(self):
        """Empty spec: 0 > 25 is False."""
        spec = {
            "openapi": "3.0.0",
            "info": {"title": "Empty", "version": "1.0.0"},
            "servers": [{"url": "http://203.0.113.10"}],
            "paths": {},
        }
        toolset = OpenAPIToolset(spec, defer_loading="auto")
        assert toolset.tool_definitions == []


# ---------------------------------------------------------------------------
# YAML spec that parses to a non-dict
# ---------------------------------------------------------------------------


class TestYamlNonDictSpec:
    def test_yaml_list_raises_type_error(self):
        """A YAML string that yields a list (not a mapping) must raise TypeError."""
        bad_yaml = yaml.safe_dump(["not", "a", "dict"])
        with pytest.raises(TypeError, match="mapping"):
            OpenAPIToolset(bad_yaml)

    def test_json_string_yielding_list_raises_type_error(self):
        """Same check for a JSON string that yields a list."""
        bad_json = json.dumps(["not", "a", "dict"])
        with pytest.raises(TypeError, match="mapping"):
            OpenAPIToolset(bad_json)


# ---------------------------------------------------------------------------
# Capability: invalid auth scheme dict
# ---------------------------------------------------------------------------


class TestCapabilityInvalidAuthScheme:
    def test_invalid_auth_scheme_type_raises(self, petstore_spec):
        """An auth config with an unrecognised scheme type should raise ValueError
        at get_toolset() time — the error must be informative."""
        capability = OpenAPI(
            spec=petstore_spec,
            auth={
                "scheme": {"type": "magic_unknown_type"},
                "credential": {"auth_type": "apiKey", "api_key": "k"},
            },
        )
        with pytest.raises(ValueError, match="Invalid security scheme"):
            capability.get_toolset()

    def test_auth_without_scheme_key_raises(self, petstore_spec):
        """auth dict with only a 'credential' key (no 'scheme') should raise."""
        capability = OpenAPI(
            spec=petstore_spec,
            auth={"credential": {"auth_type": "apiKey", "api_key": "k"}},
        )
        with pytest.raises(ValueError, match="scheme"):
            capability.get_toolset()


# ---------------------------------------------------------------------------
# Capability repr: auth + header values redacted
# ---------------------------------------------------------------------------


class TestCapabilityReprRedaction:
    """The OpenAPI capability's repr must never leak credential material:
    the resolved auth dict is fully redacted and header values are masked
    (keys stay visible for debugging)."""

    def test_auth_and_header_values_are_redacted(self, petstore_spec):
        capability = OpenAPI(
            spec=petstore_spec,
            headers={"X-Api-Key": "SECRET-header-value"},
            auth={
                "scheme": {"type": "http", "scheme": "bearer"},
                "credential": {
                    "auth_type": "http",
                    "http": {"scheme": "bearer", "credentials": {"token": "SECRET-bearer-token"}},
                },
            },
        )
        rendered = repr(capability)
        assert "SECRET" not in rendered
        assert "auth=<redacted>" in rendered
        # Header keys stay visible; values are masked
        assert "X-Api-Key" in rendered
        assert "<redacted>" in rendered

    def test_unset_auth_renders_as_none(self, petstore_spec):
        capability = OpenAPI(spec=petstore_spec)
        rendered = repr(capability)
        assert "auth=None" in rendered
        assert "headers=None" in rendered


# ---------------------------------------------------------------------------
# Two OpenAPI capabilities on one agent
# ---------------------------------------------------------------------------


class TestTwoCapabilitiesSameAgent:
    """Two flokoa.OpenAPI capabilities with different toolset_ids on the same agent
    must not collide — both toolsets contribute their tools without stomping."""

    def _make_spec(self, path: str, op_id: str) -> dict[str, Any]:
        return {
            "openapi": "3.0.0",
            "info": {"title": "Mini", "version": "1.0.0"},
            "servers": [{"url": "http://203.0.113.10"}],
            "paths": {
                path: {
                    "get": {
                        "operationId": op_id,
                        "responses": {
                            "200": {
                                "description": "ok",
                                "content": {"application/json": {"schema": {"type": "object"}}},
                            }
                        },
                    }
                }
            },
        }

    async def test_tools_from_both_capabilities_reach_model(self):
        spec_a = self._make_spec("/alpha", "getAlpha")
        spec_b = self._make_spec("/beta", "getBeta")

        spec_doc = {
            "capabilities": [
                {"flokoa.OpenAPI": {"spec": spec_a, "defer_tools": "none", "toolset_id": "cap-a"}},
                {"flokoa.OpenAPI": {"spec": spec_b, "defer_tools": "none", "toolset_id": "cap-b"}},
            ]
        }
        model = TestModel(call_tools=[])
        agent = __import__("pydantic_ai").Agent.from_spec(spec_doc, model=model, custom_capability_types=[OpenAPI])

        await agent.run("hello")
        params = model.last_model_request_parameters
        tool_names = {td.name for td in params.function_tools}
        assert "get_alpha" in tool_names
        assert "get_beta" in tool_names


# ---------------------------------------------------------------------------
# Exchanger error propagation into call_tool
# ---------------------------------------------------------------------------


class TestExchangerErrorPropagation:
    """When exchange_credential raises, call_tool must return a sanitised error dict
    (never propagate the raw exception to the model).

    OpenAPIToolset._execute wraps the exchanger call in a try/except and
    returns a sanitised {"error": ...} envelope on failure — the same
    pattern used for httpx.TimeoutException and httpx.RequestError.
    """

    async def test_exchange_error_returns_sanitised_envelope(self, petstore_spec):
        from flokoa_common.auth.auth_credential import (
            AuthCredential,
            AuthCredentialTypes,
        )
        from flokoa_common.auth.auth_schemes import AuthScheme
        from flokoa_common.auth.exchangers.base import AuthCredentialMissingError
        from flokoa_common.auth.helpers import dict_to_auth_scheme
        from flokoa_openapi.toolset import OpenAPIToolset

        scheme = dict_to_auth_scheme({"type": "http", "scheme": "bearer"})
        cred = AuthCredential(auth_type=AuthCredentialTypes.API_KEY, api_key="x")

        class _ExplodingExchanger:
            async def exchange_credential(self, auth_scheme: AuthScheme, auth_credential: AuthCredential | None = None):
                raise AuthCredentialMissingError("injected failure")

        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(
            petstore_spec,
            auth_scheme=scheme,
            auth_credential=cred,
            transport=_capture_transport(captured),
        )
        # Monkey-patch the exchanger on the toolset instance
        toolset._exchanger = _ExplodingExchanger()  # type: ignore[assignment]

        result = await _call(toolset, "list_pets")
        # The request must NOT have been sent
        assert captured == []
        # The returned value must be a sanitised error dict (not a raw exception)
        assert isinstance(result, dict)
        assert result == {"error": "Tool list_pets credential exchange failed"}
        # No exception detail (which could carry credential material) leaks
        assert "injected failure" not in str(result)
