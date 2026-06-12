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

"""Gap-filling tests for flokoa-common: items not covered by the primary suites.

Covers:
- credential_to_param: cookie-location API key
- OAuth2 expiry boundary exactly at 29s remaining (inside buffer → refresh)
  and exactly at 31s remaining (outside buffer → no refresh)
- Concurrent exchange_credential calls on one OAuth2CredentialExchanger
  instance: all resolve, and the per-instance refresh lock deduplicates
  in-flight refreshes to exactly one token-endpoint POST.
- Secret repr suppression: tokens/secrets/private keys never appear in
  repr() of credential models.
- request_builder hardening: path params are percent-encoded (no traversal),
  header param values are CR/LF-stripped.
- SSRF blocklist: IPv4-mapped forms of "this" network, CGNAT, and 192.0.0.0/8.
"""

from __future__ import annotations

import asyncio
import time

import httpx
import pytest
from fastapi.openapi.models import APIKey, Operation, Schema
from flokoa_common.auth.auth_credential import (
    AuthCredential,
    AuthCredentialTypes,
    HttpAuth,
    HttpCredentials,
    OAuth2Auth,
    ServiceAccountCredential,
)
from flokoa_common.auth.auth_schemes import OpenIdConnectWithConfig
from flokoa_common.auth.exchangers.oauth2 import _EXPIRY_BUFFER_SECONDS, OAuth2CredentialExchanger
from flokoa_common.auth.helpers import INTERNAL_AUTH_PREFIX, OpenIdConfig, credential_to_param
from flokoa_common.utils.openapi.common import ApiParameter
from flokoa_common.utils.openapi.openapi_spec_parser import OperationEndpoint
from flokoa_common.utils.openapi.request_builder import prepare_request_params
from flokoa_common.utils.url_validation import SSRFError, validate_url

pytestmark = pytest.mark.anyio


# ---------------------------------------------------------------------------
# credential_to_param: cookie-location API key
# ---------------------------------------------------------------------------


class TestCredentialToParamCookie:
    def test_api_key_cookie_produces_cookie_param(self):
        scheme = APIKey(**{"type": "apiKey", "in": "cookie", "name": "session_id"})
        cred = AuthCredential(auth_type=AuthCredentialTypes.API_KEY, api_key="cookie-val")

        param, kwargs = credential_to_param(scheme, cred)

        assert param is not None
        assert param.param_location == "cookie"
        assert param.py_name == INTERNAL_AUTH_PREFIX + "session_id"
        assert kwargs is not None
        assert kwargs[INTERNAL_AUTH_PREFIX + "session_id"] == "cookie-val"


# ---------------------------------------------------------------------------
# OAuth2 expiry boundary: exactly at buffer edges
# ---------------------------------------------------------------------------


class TestOAuth2ExpiryBoundaryExact:
    """The buffer is _EXPIRY_BUFFER_SECONDS (30).  A token expiring in exactly
    29 seconds is within the buffer (refresh needed); one expiring in exactly
    31 seconds is outside the buffer (no refresh)."""

    def _make_credential(self, seconds_until_expiry: int) -> AuthCredential:
        return AuthCredential(
            auth_type=AuthCredentialTypes.OAUTH2,
            oauth2=OAuth2Auth(
                client_id="client",
                client_secret="secret",
                access_token="current-token",
                refresh_token="refresh-tok",
                expires_at=int(time.time()) + seconds_until_expiry,
            ),
        )

    def _make_scheme(self) -> OpenIdConnectWithConfig:
        return OpenIdConnectWithConfig(
            openIdConnectUrl="https://example.com/.well-known/openid-configuration",
            authorization_endpoint="https://example.com/auth",
            token_endpoint="https://example.com/token",
        )

    def test_29s_remaining_is_expired(self):
        """expires_at = now + 29 → inside the 30s buffer → needs refresh."""
        exchanger = OAuth2CredentialExchanger()
        cred = self._make_credential(seconds_until_expiry=_EXPIRY_BUFFER_SECONDS - 1)
        assert exchanger._is_token_expired(cred) is True

    def test_31s_remaining_is_not_expired(self):
        """expires_at = now + 31 → outside the 30s buffer → no refresh."""
        exchanger = OAuth2CredentialExchanger()
        cred = self._make_credential(seconds_until_expiry=_EXPIRY_BUFFER_SECONDS + 1)
        assert exchanger._is_token_expired(cred) is False

    def test_exactly_at_buffer_is_expired(self):
        """expires_at = now + 30 → exactly at the boundary.
        The check is `now >= expires_at - buffer`, i.e. `now >= now + 30 - 30`
        which is `now >= now` → True (needs refresh)."""
        exchanger = OAuth2CredentialExchanger()
        cred = self._make_credential(seconds_until_expiry=_EXPIRY_BUFFER_SECONDS)
        assert exchanger._is_token_expired(cred) is True


# ---------------------------------------------------------------------------
# Concurrent exchange_credential calls
# ---------------------------------------------------------------------------


class TestOAuth2ConcurrentExchange:
    """N simultaneous calls to exchange_credential on a single
    OAuth2CredentialExchanger instance where the shared token is expired.

    The exchanger holds a per-instance asyncio.Lock with a double-checked
    expiry test: the winner refreshes the shared credential in place and
    every waiter re-checks under the lock, sees the fresh expiry, and skips
    its own refresh.  Exactly ONE token-endpoint POST must happen, and every
    caller must still receive a valid bearer credential.
    """

    async def test_concurrent_calls_trigger_exactly_one_refresh(self):
        refresh_count = {"n": 0}
        captured: list[httpx.Request] = []

        def handler(request: httpx.Request) -> httpx.Response:
            captured.append(request)
            refresh_count["n"] += 1
            return httpx.Response(
                200,
                json={
                    "access_token": f"fresh-token-{refresh_count['n']}",
                    "expires_in": 3600,
                },
            )

        client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
        exchanger = OAuth2CredentialExchanger(http_client=client)

        scheme = OpenIdConnectWithConfig(
            openIdConnectUrl="https://example.com/.well-known/openid-configuration",
            authorization_endpoint="https://example.com/auth",
            token_endpoint="https://example.com/token",
        )
        # Expired token: expires_at in the past
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.OAUTH2,
            oauth2=OAuth2Auth(
                client_id="client",
                access_token="stale-token",
                refresh_token="refresh-tok",
                expires_at=int(time.time()) - 60,
            ),
        )

        n_callers = 10
        results = await asyncio.gather(*(exchanger.exchange_credential(scheme, cred) for _ in range(n_callers)))

        # Every caller must receive a valid HTTP bearer credential carrying
        # the single freshly-minted token.
        assert len(results) == n_callers
        for result in results:
            assert result is not None
            assert result.auth_type == AuthCredentialTypes.HTTP
            assert result.http.credentials.token == "fresh-token-1"

        # The thundering herd is collapsed into exactly one refresh POST.
        assert len(captured) == 1


# ---------------------------------------------------------------------------
# Secret repr suppression
# ---------------------------------------------------------------------------


class TestSecretReprSuppression:
    """Secret-bearing fields are excluded from repr() so credentials never
    leak through tracebacks, debug logs, or interactive sessions."""

    def test_oauth2_secrets_not_in_repr(self):
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.OAUTH2,
            oauth2=OAuth2Auth(
                client_id="public-client-id",
                client_secret="SECRET-client-secret",
                auth_response_uri="https://cb.example.com/?code=SECRET-code-in-uri",
                auth_code="SECRET-auth-code",
                access_token="SECRET-access-token",
                refresh_token="SECRET-refresh-token",
            ),
        )
        rendered = repr(cred)
        assert "SECRET" not in rendered
        # Non-secret metadata stays visible for debugging
        assert "public-client-id" in rendered

    def test_http_credentials_secrets_not_in_repr(self):
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.HTTP,
            http=HttpAuth(
                scheme="basic",
                credentials=HttpCredentials(username="user", password="SECRET-password", token="SECRET-token"),
            ),
        )
        rendered = repr(cred)
        assert "SECRET" not in rendered
        assert "user" in rendered

    def test_service_account_private_key_not_in_repr(self):
        sa_cred = ServiceAccountCredential(
            type="service_account",
            project_id="proj",
            private_key_id="key-id",
            private_key="-----BEGIN PRIVATE KEY-----SECRET-private-key",
            client_email="sa@proj.iam.gserviceaccount.com",
            client_id="123",
            auth_uri="https://accounts.google.com/o/oauth2/auth",
            token_uri="https://oauth2.googleapis.com/token",
            auth_provider_x509_cert_url="https://www.googleapis.com/oauth2/v1/certs",
            client_x509_cert_url="https://www.googleapis.com/robot/v1/metadata/x509/sa",
            universe_domain="googleapis.com",
        )
        rendered = repr(sa_cred)
        assert "SECRET" not in rendered
        assert "proj" in rendered

    def test_openid_config_client_secret_not_in_repr(self):
        config = OpenIdConfig(
            client_id="client",
            auth_uri="https://example.com/auth",
            token_uri="https://example.com/token",
            client_secret="SECRET-client-secret",
        )
        assert "SECRET" not in repr(config)


# ---------------------------------------------------------------------------
# request_builder hardening: path param encoding, header CRLF stripping
# ---------------------------------------------------------------------------


def _prepare(parameters: list[ApiParameter], kwargs: dict, path: str = "/pets/{petId}") -> dict:
    endpoint = OperationEndpoint(base_url="http://203.0.113.10", path=path, method="get")
    operation = Operation.model_validate({"operationId": "getPet", "responses": {"200": {"description": "ok"}}})
    return prepare_request_params(
        endpoint=endpoint,
        operation=operation,
        default_headers={},
        tool_name="get_pet",
        parameters=parameters,
        kwargs=kwargs,
    )


class TestPathParamPercentEncoding:
    """Path param values are percent-encoded with no safe characters, so a
    model-provided value cannot traverse outside its path segment."""

    def _path_param(self) -> ApiParameter:
        return ApiParameter(original_name="petId", param_location="path", param_schema=Schema(type="string"))

    def test_traversal_value_is_encoded(self):
        params = self._prepare_url(pet_id="../../../etc/passwd")
        assert "../" not in params["url"]
        assert params["url"] == "http://203.0.113.10/pets/..%2F..%2F..%2Fetc%2Fpasswd"

    def test_normal_value_is_unchanged(self):
        params = self._prepare_url(pet_id="fluffy-42")
        assert params["url"] == "http://203.0.113.10/pets/fluffy-42"

    def _prepare_url(self, **kwargs) -> dict:
        return _prepare([self._path_param()], kwargs)


class TestQueryParamFalsyValues:
    """Query params with falsy-but-meaningful values (0, False, "") must be
    sent; only None is dropped."""

    def _query_param(self, name: str) -> ApiParameter:
        return ApiParameter(original_name=name, param_location="query", param_schema=Schema(type="string"))

    def test_zero_false_and_empty_string_are_kept(self):
        params = _prepare(
            [self._query_param("limit"), self._query_param("active"), self._query_param("q")],
            {"limit": 0, "active": False, "q": ""},
            path="/pets",
        )
        assert params["params"] == {"limit": 0, "active": False, "q": ""}

    def test_none_is_dropped(self):
        params = _prepare([self._query_param("limit")], {"limit": None}, path="/pets")
        assert "limit" not in params["params"]


class TestHeaderParamCrlfStripping:
    """CR/LF in header param values is stripped before the request is built,
    so a value cannot smuggle extra headers (or echo CRLF into error logs)."""

    def _header_param(self) -> ApiParameter:
        return ApiParameter(original_name="X-Trace-Id", param_location="header", param_schema=Schema(type="string"))

    def test_crlf_is_stripped_from_header_value(self):
        params = _prepare([self._header_param()], {"x_trace_id": "abc\r\nX-Injected: 1"}, path="/pets")
        assert params["headers"]["X-Trace-Id"] == "abcX-Injected: 1"

    def test_clean_header_value_is_unchanged(self):
        params = _prepare([self._header_param()], {"x_trace_id": "trace-123"}, path="/pets")
        assert params["headers"]["X-Trace-Id"] == "trace-123"


# ---------------------------------------------------------------------------
# SSRF blocklist: IPv4-mapped IPv6 ranges
# ---------------------------------------------------------------------------


class TestIpv4MappedBlocklist:
    """IPv4-mapped IPv6 literals must not bypass the IPv4 blocklist."""

    @pytest.mark.parametrize(
        "url",
        [
            "http://[::ffff:100.64.0.1]/x",  # shared address space (CGN)
            "http://[::ffff:100.127.255.254]/x",  # upper end of 100.64.0.0/10
            "http://[::ffff:0.0.0.1]/x",  # "this" network
            "http://[::ffff:192.0.0.8]/x",  # IETF protocol assignments
        ],
    )
    def test_mapped_internal_ranges_are_blocked(self, url):
        with pytest.raises(SSRFError):
            validate_url(url)

    def test_mapped_public_address_is_allowed(self):
        # TEST-NET-3 mapped: public, no DNS lookup involved.
        assert validate_url("http://[::ffff:203.0.113.10]/x")
