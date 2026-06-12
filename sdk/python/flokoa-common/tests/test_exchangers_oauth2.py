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

"""OAuth2CredentialExchanger: expiry detection, endpoint extraction, async refresh."""

from __future__ import annotations

import logging
import time

import httpx
import pytest
from fastapi.openapi.models import (
    APIKey,
    HTTPBearer,
    OAuth2,
    OAuthFlowAuthorizationCode,
    OAuthFlows,
)
from flokoa_common.auth.auth_credential import (
    AuthCredential,
    AuthCredentialTypes,
    HttpAuth,
    HttpCredentials,
    OAuth2Auth,
)
from flokoa_common.auth.auth_schemes import OpenIdConnectWithConfig
from flokoa_common.auth.exchangers.oauth2 import (
    _EXPIRY_BUFFER_SECONDS,
    OAuth2CredentialExchanger,
)

pytestmark = pytest.mark.anyio


def _make_openid_scheme(**overrides):
    defaults = {
        "openIdConnectUrl": "https://example.com/.well-known/openid-configuration",
        "authorization_endpoint": "https://example.com/auth",
        "token_endpoint": "https://example.com/token",
    }
    defaults.update(overrides)
    return OpenIdConnectWithConfig(**defaults)


def _make_oauth2_scheme(token_url="https://example.com/token", auth_url="https://example.com/auth"):  # noqa: S107
    return OAuth2(
        flows=OAuthFlows(
            authorizationCode=OAuthFlowAuthorizationCode(
                authorizationUrl=auth_url,
                tokenUrl=token_url,
            )
        )
    )


def _make_oauth2_credential(**overrides):
    defaults = {
        "auth_type": AuthCredentialTypes.OAUTH2,
        "oauth2": OAuth2Auth(
            client_id="test-client",
            client_secret="test-secret",
            access_token="old-token",
            refresh_token="refresh-tok",
        ),
    }
    defaults.update(overrides)
    return AuthCredential(**defaults)


def _exchanger_with_responses(responses: list[httpx.Response]) -> tuple[OAuth2CredentialExchanger, list[httpx.Request]]:
    """An exchanger whose token endpoint requests are served from a queue."""
    captured: list[httpx.Request] = []
    queue = list(responses)

    def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        # Serve responses in order; repeat the last one if exhausted.
        return queue.pop(0) if len(queue) > 1 else queue[0]

    client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
    return OAuth2CredentialExchanger(http_client=client), captured


# ===========================================================================
# Token expiry detection
# ===========================================================================


class TestTokenExpiryDetection:
    def test_not_expired_when_no_expiry_fields(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        cred.oauth2.expires_at = None
        cred.oauth2.expires_in = None
        assert exchanger._is_token_expired(cred) is False

    def test_expired_when_expires_at_in_past(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        cred.oauth2.expires_at = int(time.time()) - 60
        assert exchanger._is_token_expired(cred) is True

    def test_not_expired_when_expires_at_in_future(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        cred.oauth2.expires_at = int(time.time()) + 3600
        assert exchanger._is_token_expired(cred) is False

    def test_expired_within_buffer(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        # Expires in 10 seconds, but buffer is 30 — should be considered expired
        cred.oauth2.expires_at = int(time.time()) + 10
        assert exchanger._is_token_expired(cred) is True

    def test_not_expired_just_outside_buffer(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        cred.oauth2.expires_at = int(time.time()) + _EXPIRY_BUFFER_SECONDS + 60
        assert exchanger._is_token_expired(cred) is False

    def test_expires_in_without_timestamp_needs_refresh(self):
        """expires_in without an absolute timestamp means unknown validity → refresh."""
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        cred.oauth2.expires_at = None
        cred.oauth2.expires_in = 3600
        assert exchanger._is_token_expired(cred) is True

    def test_no_oauth2_data_not_expired(self):
        exchanger = OAuth2CredentialExchanger()
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.HTTP,
            http=HttpAuth(scheme="bearer", credentials=HttpCredentials(token="tok")),
        )
        assert exchanger._is_token_expired(cred) is False


# ===========================================================================
# Token endpoint extraction
# ===========================================================================


class TestTokenEndpointExtraction:
    def test_from_openid_connect_scheme(self):
        exchanger = OAuth2CredentialExchanger()
        scheme = _make_openid_scheme(token_endpoint="https://oidc.example.com/token")
        assert exchanger._get_token_endpoint(scheme) == "https://oidc.example.com/token"

    def test_from_oauth2_authorization_code_flow(self):
        exchanger = OAuth2CredentialExchanger()
        scheme = _make_oauth2_scheme(token_url="https://oauth.example.com/token")
        assert exchanger._get_token_endpoint(scheme) == "https://oauth.example.com/token"

    def test_no_flows_returns_none(self):
        exchanger = OAuth2CredentialExchanger()
        scheme = HTTPBearer()
        assert exchanger._get_token_endpoint(scheme) is None


# ===========================================================================
# Refresh access token (async, via MockTransport)
# ===========================================================================


class TestRefreshAccessToken:
    async def test_successful_refresh(self):
        exchanger, captured = _exchanger_with_responses(
            [
                httpx.Response(
                    200,
                    json={
                        "access_token": "new-access-token",
                        "refresh_token": "new-refresh-token",
                        "expires_in": 3600,
                    },
                )
            ]
        )
        cred = _make_oauth2_credential()
        result = await exchanger._refresh_access_token(_make_openid_scheme(), cred)

        assert result.oauth2.access_token == "new-access-token"
        assert result.oauth2.refresh_token == "new-refresh-token"
        assert result.oauth2.expires_in == 3600
        assert result.oauth2.expires_at is not None
        assert len(captured) == 1
        assert captured[0].url == "https://example.com/token"

    async def test_refresh_rejects_internal_token_endpoint(self):
        # SSRF validation still fires even though it now runs via
        # asyncio.to_thread to keep the blocking DNS lookup off the event loop.
        from flokoa_common.utils.url_validation import SSRFError

        exchanger, captured = _exchanger_with_responses([httpx.Response(200, json={})])
        scheme = _make_openid_scheme(token_endpoint="http://169.254.169.254/token")
        with pytest.raises(SSRFError):
            await exchanger._refresh_access_token(scheme, _make_oauth2_credential())
        assert captured == []  # never reached the transport

    async def test_refresh_without_refresh_token_returns_original(self):
        exchanger, captured = _exchanger_with_responses([httpx.Response(200, json={})])
        cred = _make_oauth2_credential()
        cred.oauth2.refresh_token = None

        result = await exchanger._refresh_access_token(_make_openid_scheme(), cred)
        assert result.oauth2.access_token == "old-token"
        assert captured == []

    async def test_refresh_with_no_token_endpoint_returns_original(self):
        exchanger, captured = _exchanger_with_responses([httpx.Response(200, json={})])
        cred = _make_oauth2_credential()

        result = await exchanger._refresh_access_token(HTTPBearer(), cred)
        assert result.oauth2.access_token == "old-token"
        assert captured == []

    async def test_refresh_400_returns_original_no_retry_and_logs_loudly(self, caplog):
        exchanger, captured = _exchanger_with_responses([httpx.Response(400, json={"error": "invalid_grant"})])
        cred = _make_oauth2_credential()

        with caplog.at_level(logging.ERROR, logger="flokoa_common.auth.exchangers.oauth2"):
            result = await exchanger._refresh_access_token(_make_openid_scheme(), cred)

        assert result.oauth2.access_token == "old-token"
        # Should NOT retry on 400
        assert len(captured) == 1
        assert any("refresh" in record.message.lower() for record in caplog.records)

    async def test_refresh_500_retries(self):
        exchanger, captured = _exchanger_with_responses(
            [
                httpx.Response(500, text="Internal Server Error"),
                httpx.Response(200, json={"access_token": "refreshed"}),
            ]
        )
        cred = _make_oauth2_credential()

        result = await exchanger._refresh_access_token(_make_openid_scheme(), cred)
        assert result.oauth2.access_token == "refreshed"
        assert len(captured) == 2

    async def test_refresh_network_error_retries(self):
        captured: list[httpx.Request] = []
        calls = {"n": 0}

        def handler(request: httpx.Request) -> httpx.Response:
            captured.append(request)
            calls["n"] += 1
            if calls["n"] == 1:
                raise httpx.ConnectError("conn refused")
            return httpx.Response(200, json={"access_token": "recovered"})

        client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
        exchanger = OAuth2CredentialExchanger(http_client=client)
        cred = _make_oauth2_credential()

        result = await exchanger._refresh_access_token(_make_openid_scheme(), cred)
        assert result.oauth2.access_token == "recovered"

    async def test_refresh_all_retries_exhausted_logs_loudly(self, caplog):
        def handler(request: httpx.Request) -> httpx.Response:
            raise httpx.ConnectError("conn refused")

        client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
        exchanger = OAuth2CredentialExchanger(http_client=client)
        cred = _make_oauth2_credential()

        with caplog.at_level(logging.ERROR, logger="flokoa_common.auth.exchangers.oauth2"):
            result = await exchanger._refresh_access_token(_make_openid_scheme(), cred)

        # Returns original credential, loudly
        assert result.oauth2.access_token == "old-token"
        assert any("failed after" in record.message for record in caplog.records)

    async def test_refresh_updates_expires_at_from_expires_in(self):
        before = int(time.time())
        exchanger, _ = _exchanger_with_responses(
            [httpx.Response(200, json={"access_token": "new", "expires_in": 7200})]
        )
        cred = _make_oauth2_credential()

        result = await exchanger._refresh_access_token(_make_openid_scheme(), cred)
        assert result.oauth2.expires_at >= before + 7200

    async def test_refresh_prefers_expires_at_from_response(self):
        fixed_at = int(time.time()) + 9999
        exchanger, _ = _exchanger_with_responses(
            [httpx.Response(200, json={"access_token": "new", "expires_at": fixed_at})]
        )
        cred = _make_oauth2_credential()

        result = await exchanger._refresh_access_token(_make_openid_scheme(), cred)
        assert result.oauth2.expires_at == fixed_at


# ===========================================================================
# exchange_credential integration
# ===========================================================================


class TestOAuth2ExchangeCredentialIntegration:
    async def test_valid_token_returned_as_bearer(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        cred.oauth2.expires_at = int(time.time()) + 3600

        result = await exchanger.exchange_credential(_make_openid_scheme(), cred)
        assert result.auth_type == AuthCredentialTypes.HTTP
        assert result.http.credentials.token == "old-token"

    async def test_expired_token_triggers_refresh(self):
        exchanger, captured = _exchanger_with_responses(
            [httpx.Response(200, json={"access_token": "fresh-token", "expires_in": 3600})]
        )
        cred = _make_oauth2_credential()
        cred.oauth2.expires_at = int(time.time()) - 100

        result = await exchanger.exchange_credential(_make_openid_scheme(), cred)
        assert result.auth_type == AuthCredentialTypes.HTTP
        assert result.http.credentials.token == "fresh-token"
        assert len(captured) == 1

    async def test_http_credential_returned_directly(self):
        exchanger = OAuth2CredentialExchanger()
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.HTTP,
            http=HttpAuth(scheme="bearer", credentials=HttpCredentials(token="existing")),
        )

        result = await exchanger.exchange_credential(_make_openid_scheme(), cred)
        assert result.http.credentials.token == "existing"

    async def test_invalid_scheme_raises(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        scheme = APIKey(**{"type": "apiKey", "in": "header", "name": "key"})

        with pytest.raises(ValueError, match="Invalid security scheme"):
            await exchanger.exchange_credential(scheme, cred)

    async def test_missing_credential_raises(self):
        exchanger = OAuth2CredentialExchanger()
        with pytest.raises(ValueError, match="auth_credential is empty"):
            await exchanger.exchange_credential(_make_openid_scheme(), None)

    async def test_no_access_token_returns_none(self):
        exchanger = OAuth2CredentialExchanger()
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.OAUTH2,
            oauth2=OAuth2Auth(client_id="id", client_secret="secret"),
        )

        result = await exchanger.exchange_credential(_make_openid_scheme(), cred)
        assert result is None
