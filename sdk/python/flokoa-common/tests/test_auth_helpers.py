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

"""Auth helpers: scheme/credential constructors, credential_to_param, OIDC discovery."""

from __future__ import annotations

from unittest.mock import patch

import httpx
import pytest
from fastapi.openapi.models import (
    APIKey,
    HTTPBearer,
    OAuth2,
    OAuthFlowAuthorizationCode,
    OAuthFlows,
    SecuritySchemeType,
)
from flokoa_common.auth.auth_credential import (
    AuthCredential,
    AuthCredentialTypes,
    HttpAuth,
    HttpCredentials,
    OAuth2Auth,
)
from flokoa_common.auth.auth_schemes import OpenIdConnectWithConfig
from flokoa_common.auth.helpers import (
    INTERNAL_AUTH_PREFIX,
    credential_to_param,
    dict_to_auth_scheme,
    resolve_openid_connect_scheme,
    token_to_scheme_credential,
)


def _make_openid_scheme(**overrides):
    defaults = {
        "openIdConnectUrl": "https://example.com/.well-known/openid-configuration",
        "authorization_endpoint": "https://example.com/auth",
        "token_endpoint": "https://example.com/token",
    }
    defaults.update(overrides)
    return OpenIdConnectWithConfig(**defaults)


def _make_oauth2_scheme():
    return OAuth2(
        flows=OAuthFlows(
            authorizationCode=OAuthFlowAuthorizationCode(
                authorizationUrl="https://example.com/auth",
                tokenUrl="https://example.com/token",
            )
        )
    )


def _bearer_credential(token: str) -> AuthCredential:
    return AuthCredential(
        auth_type=AuthCredentialTypes.HTTP,
        http=HttpAuth(scheme="bearer", credentials=HttpCredentials(token=token)),
    )


# ===========================================================================
# token_to_scheme_credential
# ===========================================================================


class TestTokenToSchemeCredential:
    def test_apikey_header(self):
        scheme, cred = token_to_scheme_credential("apikey", "header", "X-API-Key", "key123")
        assert scheme.type_ == SecuritySchemeType.apiKey
        assert scheme.in_.value == "header"
        assert cred.api_key == "key123"

    def test_apikey_query(self):
        scheme, cred = token_to_scheme_credential("apikey", "query", "api_key", "key456")
        assert scheme.in_.value == "query"
        assert cred.api_key == "key456"

    def test_apikey_cookie(self):
        scheme, cred = token_to_scheme_credential("apikey", "cookie", "session", "cook789")
        assert scheme.in_.value == "cookie"
        assert cred.api_key == "cook789"

    def test_apikey_invalid_location_raises(self):
        with pytest.raises(ValueError, match="Invalid location"):
            token_to_scheme_credential("apikey", "body", "key", "v")

    def test_apikey_without_value_yields_no_credential(self):
        _, cred = token_to_scheme_credential("apikey", "header", "X-API-Key")
        assert cred is None

    def test_oauth2_token(self):
        scheme, cred = token_to_scheme_credential("oauth2Token", "header", "Authorization", "tok")
        assert isinstance(scheme, HTTPBearer)
        assert cred.auth_type == AuthCredentialTypes.HTTP
        assert cred.http.credentials.token == "tok"

    def test_invalid_type_raises(self):
        with pytest.raises(ValueError, match="Invalid security scheme type"):
            token_to_scheme_credential("magic", "header", "k", "v")


# ===========================================================================
# credential_to_param — split OpenIDConnect vs HTTPBearer
# ===========================================================================


class TestCredentialToParamSplit:
    def test_openid_connect_exchanged_bearer_token(self):
        param, kwargs = credential_to_param(_make_openid_scheme(), _bearer_credential("oidc-token"))
        assert param is not None
        assert param.py_name.startswith(INTERNAL_AUTH_PREFIX)
        assert kwargs is not None
        assert "Bearer oidc-token" in next(iter(kwargs.values()))

    def test_openid_connect_unexchanged_returns_none(self):
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.OAUTH2,
            oauth2=OAuth2Auth(client_id="id", client_secret="secret"),
        )
        param, kwargs = credential_to_param(_make_openid_scheme(), cred)
        assert param is None
        assert kwargs is None

    def test_oauth2_scheme_with_bearer_token(self):
        param, kwargs = credential_to_param(_make_oauth2_scheme(), _bearer_credential("oauth-tok"))
        assert param is not None
        assert "Bearer oauth-tok" in next(iter(kwargs.values()))

    def test_oauth2_scheme_without_token_returns_none(self):
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.OAUTH2,
            oauth2=OAuth2Auth(client_id="id"),
        )
        param, kwargs = credential_to_param(_make_oauth2_scheme(), cred)
        assert param is None
        assert kwargs is None

    def test_http_bearer_scheme_with_token(self):
        param, kwargs = credential_to_param(HTTPBearer(), _bearer_credential("plain-bearer"))
        assert param is not None
        assert "Bearer plain-bearer" in next(iter(kwargs.values()))

    def test_http_basic_auth_raises(self):
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.HTTP,
            http=HttpAuth(scheme="basic", credentials=HttpCredentials(username="user", password="pass")),
        )
        with pytest.raises(NotImplementedError, match="Basic Authentication"):
            credential_to_param(HTTPBearer(), cred)

    def test_api_key(self):
        scheme = APIKey(**{"type": "apiKey", "in": "header", "name": "X-Key"})
        cred = AuthCredential(auth_type=AuthCredentialTypes.API_KEY, api_key="my-key")

        param, kwargs = credential_to_param(scheme, cred)
        assert param is not None
        assert param.param_location == "header"
        assert param.py_name == INTERNAL_AUTH_PREFIX + "X-Key"
        assert "my-key" in next(iter(kwargs.values()))

    def test_no_credential_returns_none(self):
        param, kwargs = credential_to_param(HTTPBearer(), None)
        assert param is None
        assert kwargs is None


# ===========================================================================
# resolve_openid_connect_scheme
# ===========================================================================


class TestResolveOpenIdConnectScheme:
    def test_successful_discovery(self):
        discovery_doc = {
            "issuer": "https://accounts.example.com",
            "authorization_endpoint": "https://accounts.example.com/auth",
            "token_endpoint": "https://accounts.example.com/token",
            "userinfo_endpoint": "https://accounts.example.com/userinfo",
            "revocation_endpoint": "https://accounts.example.com/revoke",
            "token_endpoint_auth_methods_supported": ["client_secret_basic"],
            "grant_types_supported": ["authorization_code", "refresh_token"],
            "scopes_supported": ["openid", "profile", "email"],
        }
        mock_response = httpx.Response(
            200,
            json=discovery_doc,
            request=httpx.Request("GET", "https://accounts.example.com/.well-known/openid-configuration"),
        )
        with patch("flokoa_common.auth.helpers.httpx.get", return_value=mock_response):
            result = resolve_openid_connect_scheme("https://accounts.example.com/.well-known/openid-configuration")

        assert isinstance(result, OpenIdConnectWithConfig)
        assert result.authorization_endpoint == "https://accounts.example.com/auth"
        assert result.token_endpoint == "https://accounts.example.com/token"
        assert result.userinfo_endpoint == "https://accounts.example.com/userinfo"
        assert result.revocation_endpoint == "https://accounts.example.com/revoke"
        assert result.scopes == ["openid", "profile", "email"]

    def test_missing_required_endpoints_raises(self):
        discovery_doc = {
            "issuer": "https://accounts.example.com",
            # Missing authorization_endpoint and token_endpoint
        }
        mock_response = httpx.Response(
            200,
            json=discovery_doc,
            request=httpx.Request("GET", "https://example.com"),
        )
        with (
            patch("flokoa_common.auth.helpers.httpx.get", return_value=mock_response),
            pytest.raises(ValueError, match="missing required"),
        ):
            resolve_openid_connect_scheme("https://example.com/.well-known/openid-configuration")

    def test_http_error_raises(self):
        with (
            patch(
                "flokoa_common.auth.helpers.httpx.get",
                side_effect=httpx.ConnectError("connection refused"),
            ),
            pytest.raises(ValueError, match="Failed to fetch"),
        ):
            resolve_openid_connect_scheme("https://unreachable.example.com/.well-known/openid-configuration")

    def test_internal_discovery_url_is_blocked(self):
        from flokoa_common.utils.url_validation import SSRFError

        with pytest.raises(SSRFError):
            resolve_openid_connect_scheme("http://169.254.169.254/.well-known/openid-configuration")


# ===========================================================================
# dict_to_auth_scheme
# ===========================================================================


class TestDictToAuthScheme:
    def test_api_key(self):
        scheme = dict_to_auth_scheme({"type": "apiKey", "in": "header", "name": "X-API-Key"})
        assert isinstance(scheme, APIKey)

    def test_http_bearer(self):
        scheme = dict_to_auth_scheme({"type": "http", "scheme": "bearer", "bearerFormat": "JWT"})
        assert isinstance(scheme, HTTPBearer)

    def test_oauth2(self):
        data = {
            "type": "oauth2",
            "flows": {
                "authorizationCode": {
                    "authorizationUrl": "https://example.com/auth",
                    "tokenUrl": "https://example.com/token",
                }
            },
        }
        scheme = dict_to_auth_scheme(data)
        assert isinstance(scheme, OAuth2)

    def test_missing_type_raises(self):
        with pytest.raises(ValueError, match="Missing 'type' field"):
            dict_to_auth_scheme({})

    def test_invalid_type_raises(self):
        with pytest.raises(ValueError, match="Invalid security scheme type"):
            dict_to_auth_scheme({"type": "magic"})

    def test_openid_direct_config_with_endpoints(self):
        data = {
            "type": "openIdConnect",
            "openIdConnectUrl": "https://example.com/.well-known/openid-configuration",
            "authorization_endpoint": "https://example.com/auth",
            "token_endpoint": "https://example.com/token",
        }
        result = dict_to_auth_scheme(data)
        assert isinstance(result, OpenIdConnectWithConfig)
        assert result.token_endpoint == "https://example.com/token"

    def test_openid_discovery_fetched_when_only_url_provided(self):
        discovery_doc = {
            "authorization_endpoint": "https://discovered.example.com/auth",
            "token_endpoint": "https://discovered.example.com/token",
        }
        mock_response = httpx.Response(200, json=discovery_doc, request=httpx.Request("GET", "https://example.com"))

        data = {
            "type": "openIdConnect",
            "openIdConnectUrl": "https://example.com/.well-known/openid-configuration",
        }

        with patch("flokoa_common.auth.helpers.httpx.get", return_value=mock_response):
            result = dict_to_auth_scheme(data)

        assert isinstance(result, OpenIdConnectWithConfig)
        assert result.token_endpoint == "https://discovered.example.com/token"

    def test_openid_fallback_to_basic_on_discovery_failure(self):
        data = {
            "type": "openIdConnect",
            "openIdConnectUrl": "https://example.com/.well-known/openid-configuration",
        }

        with patch(
            "flokoa_common.auth.helpers.httpx.get",
            side_effect=httpx.ConnectError("connection refused"),
        ):
            result = dict_to_auth_scheme(data)

        # Falls back to basic OpenIdConnect (not OpenIdConnectWithConfig)
        assert not isinstance(result, OpenIdConnectWithConfig)
        assert result.type_ == SecuritySchemeType.openIdConnect
