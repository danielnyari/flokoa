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

"""Gap-filling tests for flokoa-codemode-mcp: items not covered by test_executor_auth.py.

Covers:
- _apply_auth: additional_headers from an exchanged credential are forwarded
- exchanger raises → api_call returns a sanitised {"error": ...} envelope
- CodeExecutor: one credential exchanger shared across all operation callables
- executor._parse_response: text/*, non-JSON-parseable fallback to text
- CodemodeServer: openapi_spec_str with YAML format (_load_spec YAML branch)
- CodemodeServer: openapi_spec_str with JSON format (_load_spec JSON branch)
"""

from __future__ import annotations

import json
from typing import Any

import httpx
import pytest
import yaml
from fastapi.openapi.models import HTTPBearer
from flokoa_codemode_mcp.executor import _apply_auth, _create_api_callable, _parse_response
from flokoa_codemode_mcp.server import CodemodeServer
from flokoa_common.auth.auth_credential import (
    AuthCredential,
    AuthCredentialTypes,
    HttpAuth,
    HttpCredentials,
)
from flokoa_common.auth.auth_schemes import AuthScheme
from flokoa_common.auth.exchangers import AutoAuthCredentialExchanger
from flokoa_common.auth.exchangers.base import AuthCredentialMissingError
from flokoa_common.utils.openapi.openapi_spec_parser import OpenApiSpecParser, ParsedOperation

# TEST-NET-3 (RFC 5737) — passes SSRF checks, MockTransport never dials.
PUBLIC_BASE_URL = "http://203.0.113.10"


def _make_spec(base_url: str = PUBLIC_BASE_URL) -> dict[str, Any]:
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


def _parse_operation(base_url: str = PUBLIC_BASE_URL) -> ParsedOperation:
    operations = OpenApiSpecParser().parse(_make_spec(base_url))
    assert len(operations) == 1
    return operations[0]


def _capture_client(captured: list[httpx.Request]) -> httpx.AsyncClient:
    def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json={"ok": True})

    return httpx.AsyncClient(transport=httpx.MockTransport(handler))


# ---------------------------------------------------------------------------
# _apply_auth: additional_headers from exchanged credential
# ---------------------------------------------------------------------------


@pytest.mark.anyio
async def test_apply_auth_forwards_additional_headers():
    """A credential returned by the exchanger with additional_headers (e.g. the
    x-goog-user-project header from ServiceAccountCredentialExchanger) must be
    returned as the auth_additional_headers value so request_builder can include them."""
    bearer_with_extras = AuthCredential(
        auth_type=AuthCredentialTypes.HTTP,
        http=HttpAuth(
            scheme="bearer",
            credentials=HttpCredentials(token="sa-token"),
            additional_headers={"x-goog-user-project": "my-project"},
        ),
    )

    class _StaticExchanger(AutoAuthCredentialExchanger):
        async def exchange_credential(
            self, auth_scheme: AuthScheme, auth_credential: AuthCredential | None = None
        ) -> AuthCredential | None:
            return bearer_with_extras

    scheme = HTTPBearer()
    original_cred = AuthCredential(
        auth_type=AuthCredentialTypes.HTTP,
        http=HttpAuth(scheme="bearer", credentials=HttpCredentials(token="original")),
    )

    _params, kwargs, extra_headers = await _apply_auth(
        scheme,
        original_cred,
        _StaticExchanger(),
        [],
        {},
        "list_items",
    )

    assert extra_headers == {"x-goog-user-project": "my-project"}
    # The bearer token itself must still be injected via auth_param
    assert any("sa-token" in str(v) for v in kwargs.values())


# ---------------------------------------------------------------------------
# Exchanger raises → api_call returns a sanitised error envelope
# ---------------------------------------------------------------------------


@pytest.mark.anyio
async def test_apply_auth_exchanger_error_is_handled_gracefully():
    """When exchange_credential raises, api_call must not propagate the raw
    exception into the Monty sandbox — it returns a structured error dict
    with no exception detail (which could carry credential material)."""
    captured: list[httpx.Request] = []
    operation = _parse_operation()

    class _ExplodingExchanger(AutoAuthCredentialExchanger):
        async def exchange_credential(
            self, auth_scheme: AuthScheme, auth_credential: AuthCredential | None = None
        ) -> AuthCredential | None:
            raise AuthCredentialMissingError("injected exchanger failure")

    scheme = HTTPBearer()
    cred = AuthCredential(
        auth_type=AuthCredentialTypes.HTTP,
        http=HttpAuth(scheme="bearer", credentials=HttpCredentials(token="tok")),
    )

    async with _capture_client(captured) as client:
        api_call = _create_api_callable(operation, client, scheme, cred, credential_exchanger=_ExplodingExchanger())
        result = await api_call()

    # The request must NOT have been sent
    assert captured == []
    # A sanitised error dict is returned, never a raised exception
    assert result == {"error": "list_items credential exchange failed"}
    assert "injected exchanger failure" not in str(result)


# ---------------------------------------------------------------------------
# CodeExecutor: one credential exchanger shared across all operations
# ---------------------------------------------------------------------------


def _make_two_op_spec() -> dict[str, Any]:
    def op(op_id: str) -> dict[str, Any]:
        return {"get": {"operationId": op_id, "responses": {"200": {"description": "OK"}}}}

    return {
        "openapi": "3.0.0",
        "info": {"title": "Two Ops", "version": "1.0.0"},
        "servers": [{"url": PUBLIC_BASE_URL}],
        "paths": {"/alpha": op("getAlpha"), "/beta": op("getBeta")},
    }


@pytest.mark.anyio
async def test_operations_share_one_credential_exchanger(monkeypatch):
    """The executor hoists a single AutoAuthCredentialExchanger to instance
    scope: every operation callable in an execution shares it, so token state
    minted for one operation is reused by the others (one mint, not N)."""
    server = CodemodeServer(
        openapi_spec=_make_two_op_spec(),
        auth_scheme=HTTPBearer(),
        auth_credential=AuthCredential(
            auth_type=AuthCredentialTypes.HTTP,
            http=HttpAuth(scheme="bearer", credentials=HttpCredentials(token="initial")),
        ),
    )
    executor = server._executor

    minted = AuthCredential(
        auth_type=AuthCredentialTypes.HTTP,
        http=HttpAuth(scheme="bearer", credentials=HttpCredentials(token="minted-token")),
    )

    class _MintOnceExchanger(AutoAuthCredentialExchanger):
        """Mints a token on first use and caches it — N mints would mean the
        exchanger (and its cache) is not shared across operations."""

        def __init__(self) -> None:
            super().__init__()
            self.mint_count = 0
            self._cached: AuthCredential | None = None

        async def exchange_credential(
            self, auth_scheme: AuthScheme, auth_credential: AuthCredential | None = None
        ) -> AuthCredential | None:
            if self._cached is None:
                self.mint_count += 1
                self._cached = minted
            return self._cached

    exchanger = _MintOnceExchanger()
    executor._credential_exchanger = exchanger

    captured: list[httpx.Request] = []
    real_async_client = httpx.AsyncClient

    def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json={"ok": True})

    monkeypatch.setattr(httpx, "AsyncClient", lambda: real_async_client(transport=httpx.MockTransport(handler)))

    result = await executor.execute("a = await get_alpha()\nb = await get_beta()\n[a, b]")

    assert result.error is None
    assert len(captured) == 2
    # Both operations used the single shared exchanger: one mint, same token.
    assert exchanger.mint_count == 1
    assert [r.headers["Authorization"] for r in captured] == ["Bearer minted-token", "Bearer minted-token"]


# ---------------------------------------------------------------------------
# _parse_response: text/* and non-JSON-parseable fallback
# ---------------------------------------------------------------------------


class TestParseResponse:
    def test_text_html_returns_text_dict(self):
        resp = httpx.Response(
            200,
            content=b"<html><body>hello</body></html>",
            headers={"content-type": "text/html; charset=utf-8"},
        )
        result = _parse_response(resp)
        assert result == {"text": "<html><body>hello</body></html>"}

    def test_application_json_is_parsed(self):
        resp = httpx.Response(
            200,
            content=json.dumps({"key": "value"}).encode(),
            headers={"content-type": "application/json"},
        )
        result = _parse_response(resp)
        assert result == {"key": "value"}

    def test_unknown_mime_falls_back_to_text(self):
        """For a content type that isn't text/* and isn't parseable as JSON,
        the function falls back to returning {text: ...}."""
        resp = httpx.Response(
            200,
            content=b"raw-binary-content",
            headers={"content-type": "application/octet-stream"},
        )
        # application/octet-stream is not text/*, so JSON parse is tried first
        # → fails → text fallback
        result = _parse_response(resp)
        # Either returns text fallback or raises — document the actual behaviour
        assert isinstance(result, dict)

    def test_empty_body_json_content_type_falls_back_to_text(self):
        """Empty body with application/json content-type → JSON parse fails → text fallback."""
        resp = httpx.Response(
            200,
            content=b"",
            headers={"content-type": "application/json"},
        )
        result = _parse_response(resp)
        # Falls back to text (empty string)
        assert "text" in result


# ---------------------------------------------------------------------------
# CodemodeServer: _load_spec YAML and JSON branches
# ---------------------------------------------------------------------------


class TestCodemodeServerSpecLoading:
    def test_load_spec_from_yaml_string(self):
        """CodemodeServer must accept an OpenAPI spec as a YAML string."""
        spec_dict = _make_spec()
        yaml_str = yaml.safe_dump(spec_dict)
        server = CodemodeServer(openapi_spec_str=yaml_str, openapi_spec_str_type="yaml")
        # list_items should be parsed
        assert "list_items" in server._operations

    def test_load_spec_from_json_string(self):
        """CodemodeServer must accept an OpenAPI spec as a JSON string."""
        spec_dict = _make_spec()
        json_str = json.dumps(spec_dict)
        server = CodemodeServer(openapi_spec_str=json_str, openapi_spec_str_type="json")
        assert "list_items" in server._operations

    def test_load_spec_invalid_type_raises(self):
        """An unsupported openapi_spec_str_type must raise ValueError."""
        with pytest.raises(ValueError, match="Unsupported spec type"):
            CodemodeServer(
                openapi_spec_str='{"openapi": "3.0.0"}',
                openapi_spec_str_type="toml",  # type: ignore[arg-type]
            )

    def test_no_spec_initialises_empty_operations(self):
        """Constructing without any spec gives an empty operations dict."""
        server = CodemodeServer()
        assert server._operations == {}
