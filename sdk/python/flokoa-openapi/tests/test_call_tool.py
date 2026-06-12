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

"""call_tool execution: HTTP mapping, auth injection, SSRF, error sanitization."""

from __future__ import annotations

from typing import Any

import httpx
import pytest
from flokoa_common.auth.auth_credential import (
    AuthCredential,
    AuthCredentialTypes,
    HttpAuth,
    HttpCredentials,
)
from flokoa_common.auth.helpers import dict_to_auth_scheme
from flokoa_openapi import OpenAPIToolset
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


class TestHappyPath:
    async def test_get_with_path_and_query_params(self, petstore_spec):
        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(petstore_spec, transport=_capture_transport(captured))

        result = await _call(toolset, "list_pets", limit=5)
        assert result == {"ok": True}
        assert len(captured) == 1
        assert captured[0].method == "GET"
        assert captured[0].url.path == "/pets"
        assert captured[0].url.params["limit"] == "5"

    async def test_path_parameter_substitution(self, petstore_spec):
        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(petstore_spec, transport=_capture_transport(captured))

        await _call(toolset, "get_pet_by_id", pet_id=42)
        assert captured[0].url.path == "/pets/42"

    async def test_post_json_body(self, petstore_spec):
        import json

        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(petstore_spec, transport=_capture_transport(captured))

        await _call(toolset, "create_pet", name="Buddy", tag="dog")
        assert captured[0].method == "POST"
        assert json.loads(captured[0].content) == {"name": "Buddy", "tag": "dog"}
        assert captured[0].headers["content-type"] == "application/json"

    async def test_default_headers_applied(self, petstore_spec):
        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(
            petstore_spec,
            headers={"X-Tenant": "acme"},
            transport=_capture_transport(captured),
        )

        await _call(toolset, "list_pets")
        assert captured[0].headers["X-Tenant"] == "acme"

    async def test_text_response_routed_by_content_type(self, petstore_spec):
        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(
            petstore_spec,
            transport=_capture_transport(
                captured,
                lambda request: httpx.Response(200, text="plain text", headers={"content-type": "text/plain"}),
            ),
        )

        result = await _call(toolset, "list_pets")
        assert result == {"text": "plain text"}

    async def test_octet_stream_response(self, petstore_spec):
        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(
            petstore_spec,
            transport=_capture_transport(
                captured,
                lambda request: httpx.Response(
                    200, content=b"\x00\x01\x02", headers={"content-type": "application/octet-stream"}
                ),
            ),
        )

        result = await _call(toolset, "list_pets")
        assert result == {"binary_length": 3, "content_type": "application/octet-stream"}


class TestAuthInjection:
    async def test_api_key_header_injected(self, petstore_spec):
        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(
            petstore_spec,
            auth_scheme=dict_to_auth_scheme({"type": "apiKey", "in": "header", "name": "X-API-Key"}),
            auth_credential=AuthCredential(auth_type=AuthCredentialTypes.API_KEY, api_key="secret-key"),
            transport=_capture_transport(captured),
        )

        result = await _call(toolset, "list_pets", limit=3)
        assert result == {"ok": True}
        assert captured[0].headers["X-API-Key"] == "secret-key"

    async def test_api_key_query_injected(self, petstore_spec):
        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(
            petstore_spec,
            auth_scheme=dict_to_auth_scheme({"type": "apiKey", "in": "query", "name": "api_key"}),
            auth_credential=AuthCredential(auth_type=AuthCredentialTypes.API_KEY, api_key="qkey"),
            transport=_capture_transport(captured),
        )

        await _call(toolset, "list_pets")
        assert captured[0].url.params["api_key"] == "qkey"

    async def test_bearer_token_injected(self, petstore_spec):
        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(
            petstore_spec,
            auth_scheme=dict_to_auth_scheme({"type": "http", "scheme": "bearer"}),
            auth_credential=AuthCredential(
                auth_type=AuthCredentialTypes.HTTP,
                http=HttpAuth(scheme="bearer", credentials=HttpCredentials(token="tok-123")),
            ),
            transport=_capture_transport(captured),
        )

        await _call(toolset, "list_pets")
        assert captured[0].headers["Authorization"] == "Bearer tok-123"

    async def test_no_auth_sends_no_credentials(self, petstore_spec):
        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(petstore_spec, transport=_capture_transport(captured))

        await _call(toolset, "list_pets")
        assert "authorization" not in captured[0].headers
        assert "x-api-key" not in captured[0].headers


class TestSSRFProtection:
    @pytest.mark.parametrize(
        "base_url",
        [
            "http://169.254.169.254",  # cloud metadata endpoint
            "http://127.0.0.1:8080",  # loopback
            "http://10.0.0.5",  # private range
        ],
    )
    async def test_internal_urls_are_blocked(self, base_url):
        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(make_petstore_spec(base_url=base_url), transport=_capture_transport(captured))

        result = await _call(toolset, "list_pets")
        assert captured == [], "request must not be sent for a blocked URL"
        assert "blocked" in result["error"]

    async def test_allow_internal_permits_private_addresses(self):
        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(
            make_petstore_spec(base_url="http://127.0.0.1:8080"),
            allow_internal=True,
            transport=_capture_transport(captured),
        )

        result = await _call(toolset, "list_pets")
        assert result == {"ok": True}
        assert len(captured) == 1


class TestErrorSanitization:
    async def test_http_error_body_not_leaked(self, petstore_spec):
        captured: list[httpx.Request] = []
        secret_body = "stacktrace with internal hostnames and a token: sk-12345"
        toolset = OpenAPIToolset(
            petstore_spec,
            transport=_capture_transport(captured, lambda request: httpx.Response(500, text=secret_body)),
        )

        result = await _call(toolset, "list_pets")
        assert result == {"error": "Tool list_pets execution failed. Status Code: 500"}
        assert "sk-12345" not in str(result)

    async def test_error_body_logged_server_side(self, petstore_spec, caplog):
        import logging

        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(
            petstore_spec,
            transport=_capture_transport(captured, lambda request: httpx.Response(404, text="nope")),
        )

        with caplog.at_level(logging.WARNING, logger="flokoa_openapi.toolset"):
            await _call(toolset, "list_pets")
        assert any("404" in record.message or "404" in str(record.args) for record in caplog.records)

    async def test_timeout_returns_sanitized_error(self, petstore_spec):
        def raise_timeout(request: httpx.Request) -> httpx.Response:
            raise httpx.ConnectTimeout("slow")

        toolset = OpenAPIToolset(petstore_spec, transport=httpx.MockTransport(raise_timeout))
        result = await _call(toolset, "list_pets")
        assert result == {"error": "Tool list_pets request timed out"}

    async def test_connection_error_returns_sanitized_error(self, petstore_spec):
        def raise_connect(request: httpx.Request) -> httpx.Response:
            raise httpx.ConnectError("connection refused to internal-host:9999")

        toolset = OpenAPIToolset(petstore_spec, transport=httpx.MockTransport(raise_connect))
        result = await _call(toolset, "list_pets")
        assert result == {"error": "Tool list_pets request failed: ConnectError"}
        assert "internal-host" not in str(result)


class TestToolsetContract:
    async def test_get_tools_returns_toolset_tools(self, petstore_spec):
        toolset = OpenAPIToolset(petstore_spec, max_retries=2)
        tools = await toolset.get_tools(_ctx())
        assert set(tools) == {td.name for td in toolset.tool_definitions}
        for tool in tools.values():
            assert tool.toolset is toolset
            assert tool.max_retries == 2

    async def test_unknown_tool_raises(self, petstore_spec):
        toolset = OpenAPIToolset(petstore_spec)
        ctx = _ctx()
        tools = await toolset.get_tools(ctx)
        with pytest.raises(ValueError, match="Unknown tool"):
            await toolset.call_tool("nope", {}, ctx, next(iter(tools.values())))

    async def test_id_property(self, petstore_spec):
        assert OpenAPIToolset(petstore_spec).id is None
        assert OpenAPIToolset(petstore_spec, id="petstore").id == "petstore"

    async def test_call_outside_context_uses_one_shot_client(self, petstore_spec):
        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(petstore_spec, transport=_capture_transport(captured))
        ctx = _ctx()
        tools = await toolset.get_tools(ctx)
        result = await toolset.call_tool("list_pets", {}, ctx, tools["list_pets"])
        assert result == {"ok": True}

    async def test_user_agent_identifies_tool(self, petstore_spec):
        captured: list[httpx.Request] = []
        toolset = OpenAPIToolset(petstore_spec, transport=_capture_transport(captured))
        await _call(toolset, "list_pets")
        assert "flokoa" in captured[0].headers["user-agent"]
