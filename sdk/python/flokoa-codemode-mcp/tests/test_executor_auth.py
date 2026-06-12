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

"""Auth application and SSRF validation in the codemode executor's API callables."""

from __future__ import annotations

import logging
from typing import Any

import httpx
import pytest
from fastapi.openapi.models import APIKey, HTTPBearer, OAuth2, OAuthFlows
from flokoa_codemode_mcp.executor import _create_api_callable
from flokoa_codemode_mcp.server import CodemodeServer
from flokoa_common.auth.auth_credential import (
    AuthCredential,
    AuthCredentialTypes,
    HttpAuth,
    HttpCredentials,
    OAuth2Auth,
)
from flokoa_common.utils.openapi.openapi_spec_parser import OpenApiSpecParser, ParsedOperation

# TEST-NET-3 (RFC 5737): a public, never-routed address — passes SSRF checks
# without a DNS lookup, and MockTransport never opens a real connection.
PUBLIC_BASE_URL = "http://203.0.113.10"


def _make_spec(base_url: str) -> dict[str, Any]:
    return {
        "openapi": "3.0.0",
        "info": {"title": "Test API", "version": "1.0.0"},
        "servers": [{"url": base_url}],
        "paths": {
            "/items": {
                "get": {
                    "operationId": "listItems",
                    "parameters": [
                        {"name": "limit", "in": "query", "schema": {"type": "integer"}},
                    ],
                    "responses": {"200": {"description": "OK"}},
                }
            }
        },
    }


def _parse_operation(base_url: str) -> ParsedOperation:
    operations = OpenApiSpecParser().parse(_make_spec(base_url))
    assert len(operations) == 1
    return operations[0]


def _capture_client(captured: list[httpx.Request]) -> httpx.AsyncClient:
    def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json={"ok": True})

    return httpx.AsyncClient(transport=httpx.MockTransport(handler))


def _api_key_scheme(location: str, name: str) -> APIKey:
    return APIKey(**{"type": "apiKey", "in": location, "name": name})


def _api_key_credential(value: str) -> AuthCredential:
    return AuthCredential(auth_type=AuthCredentialTypes.API_KEY, api_key=value)


def _bearer_credential(token: str) -> AuthCredential:
    return AuthCredential(
        auth_type=AuthCredentialTypes.HTTP,
        http=HttpAuth(scheme="bearer", credentials=HttpCredentials(token=token)),
    )


@pytest.mark.anyio
async def test_api_key_header_credential_is_applied():
    captured: list[httpx.Request] = []
    operation = _parse_operation(PUBLIC_BASE_URL)
    async with _capture_client(captured) as client:
        api_call = _create_api_callable(
            operation,
            client,
            _api_key_scheme("header", "X-API-Key"),
            _api_key_credential("secret-key"),
        )
        result = await api_call(limit=5)

    assert result == {"ok": True}
    assert len(captured) == 1
    assert captured[0].headers["X-API-Key"] == "secret-key"


@pytest.mark.anyio
async def test_api_key_query_credential_is_applied():
    captured: list[httpx.Request] = []
    operation = _parse_operation(PUBLIC_BASE_URL)
    async with _capture_client(captured) as client:
        api_call = _create_api_callable(
            operation,
            client,
            _api_key_scheme("query", "api_key"),
            _api_key_credential("secret-key"),
        )
        result = await api_call()

    assert result == {"ok": True}
    assert len(captured) == 1
    assert captured[0].url.params["api_key"] == "secret-key"


@pytest.mark.anyio
async def test_bearer_token_credential_is_applied():
    captured: list[httpx.Request] = []
    operation = _parse_operation(PUBLIC_BASE_URL)
    async with _capture_client(captured) as client:
        api_call = _create_api_callable(
            operation,
            client,
            HTTPBearer(bearerFormat="JWT"),
            _bearer_credential("bearer-tok"),
        )
        result = await api_call()

    assert result == {"ok": True}
    assert captured[0].headers["Authorization"] == "Bearer bearer-tok"


@pytest.mark.anyio
async def test_oauth2_access_token_is_exchanged_to_bearer():
    captured: list[httpx.Request] = []
    operation = _parse_operation(PUBLIC_BASE_URL)
    credential = AuthCredential(
        auth_type=AuthCredentialTypes.OAUTH2,
        oauth2=OAuth2Auth(access_token="oauth-tok"),
    )
    async with _capture_client(captured) as client:
        api_call = _create_api_callable(operation, client, OAuth2(flows=OAuthFlows()), credential)
        result = await api_call()

    assert result == {"ok": True}
    assert captured[0].headers["Authorization"] == "Bearer oauth-tok"


@pytest.mark.anyio
async def test_unexchangeable_oauth2_credential_warns_and_sends_no_auth(caplog):
    captured: list[httpx.Request] = []
    operation = _parse_operation(PUBLIC_BASE_URL)
    # client_id/client_secret only: requires an interactive flow, cannot be
    # exchanged headlessly — the call must proceed unauthenticated but warn.
    credential = AuthCredential(
        auth_type=AuthCredentialTypes.OAUTH2,
        oauth2=OAuth2Auth(client_id="id", client_secret="secret"),
    )
    with caplog.at_level(logging.WARNING, logger="flokoa.codemode.executor"):
        async with _capture_client(captured) as client:
            api_call = _create_api_callable(operation, client, OAuth2(flows=OAuthFlows()), credential)
            result = await api_call()

    assert result == {"ok": True}
    assert "authorization" not in captured[0].headers
    assert any("auth" in record.message.lower() for record in caplog.records)


@pytest.mark.anyio
async def test_no_auth_sends_no_credentials():
    captured: list[httpx.Request] = []
    operation = _parse_operation(PUBLIC_BASE_URL)
    async with _capture_client(captured) as client:
        api_call = _create_api_callable(operation, client)
        result = await api_call(limit=3)

    assert result == {"ok": True}
    assert "authorization" not in captured[0].headers
    assert "x-api-key" not in captured[0].headers


@pytest.mark.anyio
@pytest.mark.parametrize(
    "base_url",
    [
        "http://169.254.169.254",  # cloud metadata endpoint
        "http://127.0.0.1:8080",  # loopback
        "http://10.0.0.5",  # private range
    ],
)
async def test_internal_urls_are_blocked(base_url):
    captured: list[httpx.Request] = []
    operation = _parse_operation(base_url)
    async with _capture_client(captured) as client:
        api_call = _create_api_callable(operation, client)
        result = await api_call()

    assert captured == [], "request must not be sent for a blocked URL"
    assert "blocked" in result["error"]


@pytest.mark.anyio
async def test_allow_internal_permits_private_addresses():
    captured: list[httpx.Request] = []
    operation = _parse_operation("http://127.0.0.1:8080")
    async with _capture_client(captured) as client:
        api_call = _create_api_callable(operation, client, allow_internal=True)
        result = await api_call()

    assert result == {"ok": True}
    assert len(captured) == 1


def test_server_threads_allow_internal_to_executor():
    server = CodemodeServer(openapi_spec=_make_spec(PUBLIC_BASE_URL), allow_internal=True)
    assert server._executor.allow_internal is True


def test_server_defaults_to_ssrf_protection():
    server = CodemodeServer(openapi_spec=_make_spec(PUBLIC_BASE_URL))
    assert server._executor.allow_internal is False


def test_server_threads_auth_to_executor():
    scheme = _api_key_scheme("header", "X-API-Key")
    credential = _api_key_credential("secret-key")
    server = CodemodeServer(
        openapi_spec=_make_spec(PUBLIC_BASE_URL),
        auth_scheme=scheme,
        auth_credential=credential,
    )
    assert server._executor.auth_scheme is scheme
    assert server._executor.auth_credential is credential
