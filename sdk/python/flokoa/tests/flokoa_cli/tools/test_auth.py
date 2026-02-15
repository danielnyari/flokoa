"""Tests for authentication implementations.

Covers:
- OAuth2CredentialExchanger: token refresh flow, expiry detection, retry logic
- auth_helpers: credential_to_param split handling, OpenID Connect discovery,
  resolve_openid_connect_scheme
- PydocHelper: multiple content-type handling in generate_return_doc
- rest_api_tool: Content-Type-aware response parsing
"""

import time
from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest
from fastapi.openapi.models import (
    APIKey,
    HTTPBearer,
    MediaType,
    OAuth2,
    OAuthFlowAuthorizationCode,
    OAuthFlows,
    Response,
    Schema,
    SecuritySchemeType,
)

from flokoa.auth.auth_credential import (
    AuthCredential,
    AuthCredentialTypes,
    HttpAuth,
    HttpCredentials,
    OAuth2Auth,
)
from flokoa.auth.auth_schemes import (
    OpenIdConnectWithConfig,
)
from flokoa.tools.openapi import OpenAPIDeps, create_rest_api_callable
from flokoa.tools.openapi.auth.auth_helpers import (
    credential_to_param,
    dict_to_auth_scheme,
    resolve_openid_connect_scheme,
)
from flokoa.tools.openapi.auth.credential_exchangers.oauth2_exchanger import (
    _EXPIRY_BUFFER_SECONDS,
    OAuth2CredentialExchanger,
)
from flokoa.tools.openapi.common import PydocHelper

from ..fixtures import *

pytestmark = pytest.mark.anyio


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_run_context(deps):
    from pydantic_ai import RunContext

    ctx = MagicMock(spec=RunContext)
    ctx.deps = deps
    return ctx


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


def _make_media_type(schema):
    return MediaType.model_validate({"schema": schema.model_dump()})


# ===========================================================================
# OAuth2CredentialExchanger — token expiry detection
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

    def test_no_oauth2_data_not_expired(self):
        exchanger = OAuth2CredentialExchanger()
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.HTTP,
            http=HttpAuth(scheme="bearer", credentials=HttpCredentials(token="tok")),
        )
        assert exchanger._is_token_expired(cred) is False


# ===========================================================================
# OAuth2CredentialExchanger — token endpoint extraction
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
# OAuth2CredentialExchanger — refresh access token
# ===========================================================================


class TestRefreshAccessToken:
    def test_successful_refresh(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        scheme = _make_openid_scheme()

        new_token_response = {
            "access_token": "new-access-token",
            "refresh_token": "new-refresh-token",
            "expires_in": 3600,
        }
        mock_response = httpx.Response(200, json=new_token_response, request=httpx.Request("POST", "https://example.com"))

        with patch("flokoa.tools.openapi.auth.credential_exchangers.oauth2_exchanger.httpx.post", return_value=mock_response):
            result = exchanger._refresh_access_token(scheme, cred)

        assert result.oauth2.access_token == "new-access-token"
        assert result.oauth2.refresh_token == "new-refresh-token"
        assert result.oauth2.expires_in == 3600
        assert result.oauth2.expires_at is not None

    def test_refresh_without_refresh_token_returns_original(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        cred.oauth2.refresh_token = None
        scheme = _make_openid_scheme()

        result = exchanger._refresh_access_token(scheme, cred)
        assert result.oauth2.access_token == "old-token"

    def test_refresh_with_no_token_endpoint_returns_original(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        scheme = HTTPBearer()

        result = exchanger._refresh_access_token(scheme, cred)
        assert result.oauth2.access_token == "old-token"

    def test_refresh_400_returns_original_no_retry(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        scheme = _make_openid_scheme()

        mock_response = httpx.Response(400, json={"error": "invalid_grant"}, request=httpx.Request("POST", "https://example.com"))
        with patch("flokoa.tools.openapi.auth.credential_exchangers.oauth2_exchanger.httpx.post", return_value=mock_response) as mock_post:
            result = exchanger._refresh_access_token(scheme, cred)

        assert result.oauth2.access_token == "old-token"
        # Should NOT retry on 400
        assert mock_post.call_count == 1

    def test_refresh_500_retries(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        scheme = _make_openid_scheme()

        mock_500 = httpx.Response(500, text="Internal Server Error", request=httpx.Request("POST", "https://example.com"))
        mock_200 = httpx.Response(200, json={"access_token": "refreshed"}, request=httpx.Request("POST", "https://example.com"))

        with patch("flokoa.tools.openapi.auth.credential_exchangers.oauth2_exchanger.httpx.post", side_effect=[mock_500, mock_200]):
            result = exchanger._refresh_access_token(scheme, cred)

        assert result.oauth2.access_token == "refreshed"

    def test_refresh_network_error_retries(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        scheme = _make_openid_scheme()

        mock_200 = httpx.Response(200, json={"access_token": "recovered"}, request=httpx.Request("POST", "https://example.com"))

        with patch(
            "flokoa.tools.openapi.auth.credential_exchangers.oauth2_exchanger.httpx.post",
            side_effect=[httpx.ConnectError("conn refused"), mock_200],
        ):
            result = exchanger._refresh_access_token(scheme, cred)

        assert result.oauth2.access_token == "recovered"

    def test_refresh_all_retries_exhausted(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        scheme = _make_openid_scheme()

        with patch(
            "flokoa.tools.openapi.auth.credential_exchangers.oauth2_exchanger.httpx.post",
            side_effect=httpx.ConnectError("conn refused"),
        ):
            result = exchanger._refresh_access_token(scheme, cred)

        # Returns original credential
        assert result.oauth2.access_token == "old-token"

    def test_refresh_updates_expires_at_from_expires_in(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        scheme = _make_openid_scheme()

        before = int(time.time())
        mock_response = httpx.Response(
            200,
            json={"access_token": "new", "expires_in": 7200},
            request=httpx.Request("POST", "https://example.com"),
        )

        with patch("flokoa.tools.openapi.auth.credential_exchangers.oauth2_exchanger.httpx.post", return_value=mock_response):
            result = exchanger._refresh_access_token(scheme, cred)

        assert result.oauth2.expires_at >= before + 7200

    def test_refresh_prefers_expires_at_from_response(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        scheme = _make_openid_scheme()

        fixed_at = int(time.time()) + 9999
        mock_response = httpx.Response(
            200,
            json={"access_token": "new", "expires_at": fixed_at},
            request=httpx.Request("POST", "https://example.com"),
        )

        with patch("flokoa.tools.openapi.auth.credential_exchangers.oauth2_exchanger.httpx.post", return_value=mock_response):
            result = exchanger._refresh_access_token(scheme, cred)

        assert result.oauth2.expires_at == fixed_at


# ===========================================================================
# OAuth2CredentialExchanger — exchange_credential integration
# ===========================================================================


class TestOAuth2ExchangeCredentialIntegration:
    def test_valid_token_returned_as_bearer(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        scheme = _make_openid_scheme()

        result = exchanger.exchange_credential(scheme, cred)
        assert result.auth_type == AuthCredentialTypes.HTTP
        assert result.http.credentials.token == "old-token"

    def test_expired_token_triggers_refresh(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        cred.oauth2.expires_at = int(time.time()) - 100
        scheme = _make_openid_scheme()

        mock_response = httpx.Response(
            200,
            json={"access_token": "fresh-token", "expires_in": 3600},
            request=httpx.Request("POST", "https://example.com"),
        )

        with patch("flokoa.tools.openapi.auth.credential_exchangers.oauth2_exchanger.httpx.post", return_value=mock_response):
            result = exchanger.exchange_credential(scheme, cred)

        assert result.auth_type == AuthCredentialTypes.HTTP
        assert result.http.credentials.token == "fresh-token"

    def test_http_credential_returned_directly(self):
        exchanger = OAuth2CredentialExchanger()
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.HTTP,
            http=HttpAuth(scheme="bearer", credentials=HttpCredentials(token="existing")),
        )
        scheme = _make_openid_scheme()

        result = exchanger.exchange_credential(scheme, cred)
        assert result.http.credentials.token == "existing"

    def test_invalid_scheme_raises(self):
        exchanger = OAuth2CredentialExchanger()
        cred = _make_oauth2_credential()
        scheme = APIKey(**{"type": "apiKey", "in": "header", "name": "key"})

        with pytest.raises(ValueError, match="Invalid security scheme"):
            exchanger.exchange_credential(scheme, cred)

    def test_missing_credential_raises(self):
        exchanger = OAuth2CredentialExchanger()
        scheme = _make_openid_scheme()

        with pytest.raises(ValueError, match="auth_credential is empty"):
            exchanger.exchange_credential(scheme, None)

    def test_no_access_token_returns_none(self):
        exchanger = OAuth2CredentialExchanger()
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.OAUTH2,
            oauth2=OAuth2Auth(client_id="id", client_secret="secret"),
        )
        scheme = _make_openid_scheme()

        result = exchanger.exchange_credential(scheme, cred)
        assert result is None


# ===========================================================================
# credential_to_param — split OpenIDConnect vs HTTPBearer
# ===========================================================================


class TestCredentialToParamSplit:
    def test_openid_connect_exchanged_bearer_token(self):
        scheme = _make_openid_scheme()
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.HTTP,
            http=HttpAuth(scheme="bearer", credentials=HttpCredentials(token="oidc-token")),
        )

        param, kwargs = credential_to_param(scheme, cred)
        assert param is not None
        assert kwargs is not None
        assert "Bearer oidc-token" in next(iter(kwargs.values()))

    def test_openid_connect_unexchanged_returns_none(self):
        scheme = _make_openid_scheme()
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.OAUTH2,
            oauth2=OAuth2Auth(client_id="id", client_secret="secret"),
        )

        param, kwargs = credential_to_param(scheme, cred)
        assert param is None
        assert kwargs is None

    def test_oauth2_scheme_with_bearer_token(self):
        scheme = _make_oauth2_scheme()
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.HTTP,
            http=HttpAuth(scheme="bearer", credentials=HttpCredentials(token="oauth-tok")),
        )

        param, kwargs = credential_to_param(scheme, cred)
        assert param is not None
        assert "Bearer oauth-tok" in next(iter(kwargs.values()))

    def test_oauth2_scheme_without_token_returns_none(self):
        scheme = _make_oauth2_scheme()
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.OAUTH2,
            oauth2=OAuth2Auth(client_id="id"),
        )

        param, kwargs = credential_to_param(scheme, cred)
        assert param is None
        assert kwargs is None

    def test_http_bearer_scheme_with_token(self):
        scheme = HTTPBearer()
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.HTTP,
            http=HttpAuth(scheme="bearer", credentials=HttpCredentials(token="plain-bearer")),
        )

        param, kwargs = credential_to_param(scheme, cred)
        assert param is not None
        assert "Bearer plain-bearer" in next(iter(kwargs.values()))

    def test_http_basic_auth_raises(self):
        scheme = HTTPBearer()
        cred = AuthCredential(
            auth_type=AuthCredentialTypes.HTTP,
            http=HttpAuth(scheme="basic", credentials=HttpCredentials(username="user", password="pass")),
        )

        with pytest.raises(NotImplementedError, match="Basic Authentication"):
            credential_to_param(scheme, cred)

    def test_api_key_still_works(self):
        scheme = APIKey(**{"type": "apiKey", "in": "header", "name": "X-Key"})
        cred = AuthCredential(auth_type=AuthCredentialTypes.API_KEY, api_key="my-key")

        param, kwargs = credential_to_param(scheme, cred)
        assert param is not None
        assert param.param_location == "header"
        assert "my-key" in next(iter(kwargs.values()))

    def test_no_credential_returns_none(self):
        scheme = HTTPBearer()
        param, kwargs = credential_to_param(scheme, None)
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
        with patch("flokoa.tools.openapi.auth.auth_helpers.httpx.get", return_value=mock_response):
            result = resolve_openid_connect_scheme(
                "https://accounts.example.com/.well-known/openid-configuration"
            )

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
            patch("flokoa.tools.openapi.auth.auth_helpers.httpx.get", return_value=mock_response),
            pytest.raises(ValueError, match="missing required"),
        ):
            resolve_openid_connect_scheme("https://example.com/.well-known/openid-configuration")

    def test_http_error_raises(self):
        with patch(
            "flokoa.tools.openapi.auth.auth_helpers.httpx.get",
            side_effect=httpx.ConnectError("connection refused"),
        ), pytest.raises(ValueError, match="Failed to fetch"):
            resolve_openid_connect_scheme("https://unreachable.example.com/.well-known/openid-configuration")


# ===========================================================================
# dict_to_auth_scheme — openIdConnect with discovery
# ===========================================================================


class TestDictToAuthSchemeOpenIdConnect:
    def test_direct_config_with_endpoints(self):
        data = {
            "type": "openIdConnect",
            "openIdConnectUrl": "https://example.com/.well-known/openid-configuration",
            "authorization_endpoint": "https://example.com/auth",
            "token_endpoint": "https://example.com/token",
        }
        result = dict_to_auth_scheme(data)
        assert isinstance(result, OpenIdConnectWithConfig)
        assert result.token_endpoint == "https://example.com/token"

    def test_discovery_fetched_when_only_url_provided(self):
        discovery_doc = {
            "authorization_endpoint": "https://discovered.example.com/auth",
            "token_endpoint": "https://discovered.example.com/token",
        }
        mock_response = httpx.Response(200, json=discovery_doc, request=httpx.Request("GET", "https://example.com"))

        data = {
            "type": "openIdConnect",
            "openIdConnectUrl": "https://example.com/.well-known/openid-configuration",
        }

        with patch("flokoa.tools.openapi.auth.auth_helpers.httpx.get", return_value=mock_response):
            result = dict_to_auth_scheme(data)

        assert isinstance(result, OpenIdConnectWithConfig)
        assert result.token_endpoint == "https://discovered.example.com/token"

    def test_fallback_to_basic_on_discovery_failure(self):
        data = {
            "type": "openIdConnect",
            "openIdConnectUrl": "https://example.com/.well-known/openid-configuration",
        }

        with patch(
            "flokoa.tools.openapi.auth.auth_helpers.httpx.get",
            side_effect=httpx.ConnectError("connection refused"),
        ):
            result = dict_to_auth_scheme(data)

        # Falls back to basic OpenIdConnect (not OpenIdConnectWithConfig)
        assert result is not None
        assert result.type_ == SecuritySchemeType.openIdConnect


# ===========================================================================
# PydocHelper — multiple content types
# ===========================================================================


class TestPydocHelperMultipleContentTypes:
    def test_single_content_type(self):
        responses = {
            "200": Response(
                description="Success",
                content={"application/json": _make_media_type(Schema(type="string"))},
            )
        }
        doc = PydocHelper.generate_return_doc(responses)
        assert "Returns (str):" in doc
        assert "Content-Type" not in doc

    def test_multiple_identical_schemas(self):
        schema = Schema(type="object", properties={"id": Schema(type="integer")})
        responses = {
            "200": Response(
                description="Success",
                content={
                    "application/json": _make_media_type(schema),
                    "application/xml": _make_media_type(schema),
                },
            )
        }
        doc = PydocHelper.generate_return_doc(responses)
        # Same schema for both — should produce single doc
        assert "Content-Type" not in doc
        assert "Returns (Dict[str, Any]):" in doc

    def test_multiple_different_schemas(self):
        json_schema = Schema(type="object", properties={"id": Schema(type="integer")})
        text_schema = Schema(type="string")
        responses = {
            "200": Response(
                description="Success",
                content={
                    "application/json": _make_media_type(json_schema),
                    "text/plain": _make_media_type(text_schema),
                },
            )
        }
        doc = PydocHelper.generate_return_doc(responses)
        assert "Multiple response content types:" in doc
        assert "application/json" in doc
        assert "text/plain" in doc

    def test_no_2xx_response(self):
        responses = {
            "400": Response(description="Bad Request"),
            "500": Response(description="Server Error"),
        }
        doc = PydocHelper.generate_return_doc(responses)
        assert doc == ""

    def test_2xx_without_content(self):
        responses = {"200": Response(description="No content")}
        doc = PydocHelper.generate_return_doc(responses)
        assert doc == ""


# ===========================================================================
# rest_api_tool - Content-Type-aware response parsing
# ===========================================================================


class TestContentTypeResponseParsing:
    @pytest.fixture
    def get_pet_config(self, openapi_spec):
        from flokoa.tools.openapi import RestApiToolConfig
        from flokoa.tools.openapi.openapi_spec_parser import OpenApiSpecParser

        operations = OpenApiSpecParser().parse(openapi_spec)
        get_pet_ops = [o for o in operations if o.name == "get_pet_by_id"]
        return RestApiToolConfig.from_parsed_operation(get_pet_ops[0])

    async def test_json_content_type(self, get_pet_config):
        mock_response = httpx.Response(
            200,
            json={"id": 1, "name": "Buddy"},
            request=httpx.Request("GET", "https://example.com"),
            headers={"content-type": "application/json"},
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(get_pet_config)
        result = await callable_fn(ctx, pet_id=1)
        assert result == {"id": 1, "name": "Buddy"}

    async def test_json_with_charset(self, get_pet_config):
        mock_response = httpx.Response(
            200,
            json={"id": 1},
            request=httpx.Request("GET", "https://example.com"),
            headers={"content-type": "application/json; charset=utf-8"},
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(get_pet_config)
        result = await callable_fn(ctx, pet_id=1)
        assert result == {"id": 1}

    async def test_vendor_json(self, get_pet_config):
        """application/vnd.api+json should be treated as JSON."""
        mock_response = httpx.Response(
            200,
            content=b'{"data": []}',
            request=httpx.Request("GET", "https://example.com"),
            headers={"content-type": "application/vnd.api+json"},
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(get_pet_config)
        result = await callable_fn(ctx, pet_id=1)
        assert result == {"data": []}

    async def test_text_plain_content_type(self, get_pet_config):
        mock_response = httpx.Response(
            200,
            text="plain text here",
            request=httpx.Request("GET", "https://example.com"),
            headers={"content-type": "text/plain"},
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(get_pet_config)
        result = await callable_fn(ctx, pet_id=1)
        assert result == {"text": "plain text here"}

    async def test_text_html_content_type(self, get_pet_config):
        mock_response = httpx.Response(
            200,
            text="<html><body>Hello</body></html>",
            request=httpx.Request("GET", "https://example.com"),
            headers={"content-type": "text/html"},
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(get_pet_config)
        result = await callable_fn(ctx, pet_id=1)
        assert result == {"text": "<html><body>Hello</body></html>"}

    async def test_xml_content_type(self, get_pet_config):
        mock_response = httpx.Response(
            200,
            text="<pet><name>Buddy</name></pet>",
            request=httpx.Request("GET", "https://example.com"),
            headers={"content-type": "application/xml"},
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(get_pet_config)
        result = await callable_fn(ctx, pet_id=1)
        assert result["text"] == "<pet><name>Buddy</name></pet>"
        assert result["content_type"] == "application/xml"

    async def test_octet_stream_content_type(self, get_pet_config):
        mock_response = httpx.Response(
            200,
            content=b"\x00\x01\x02\x03",
            request=httpx.Request("GET", "https://example.com"),
            headers={"content-type": "application/octet-stream"},
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(get_pet_config)
        result = await callable_fn(ctx, pet_id=1)
        assert result["binary_length"] == 4
        assert result["content_type"] == "application/octet-stream"

    async def test_unknown_content_type_fallback_json(self, get_pet_config):
        """Unknown content type with valid JSON body should parse as JSON."""
        mock_response = httpx.Response(
            200,
            content=b'{"foo": "bar"}',
            request=httpx.Request("GET", "https://example.com"),
            headers={"content-type": "application/x-custom"},
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(get_pet_config)
        result = await callable_fn(ctx, pet_id=1)
        assert result == {"foo": "bar"}

    async def test_unknown_content_type_fallback_text(self, get_pet_config):
        """Unknown content type with non-JSON body falls back to text."""
        mock_response = httpx.Response(
            200,
            text="not json",
            request=httpx.Request("GET", "https://example.com"),
            headers={"content-type": "application/x-custom"},
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(get_pet_config)
        result = await callable_fn(ctx, pet_id=1)
        assert result == {"text": "not json"}

    async def test_error_response_still_works(self, get_pet_config):
        mock_response = httpx.Response(
            404,
            content=b"Not found",
            request=httpx.Request("GET", "https://example.com"),
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(get_pet_config)
        result = await callable_fn(ctx, pet_id=999)
        assert "error" in result
        assert "404" in result["error"]

    async def test_no_content_type_header_fallback(self, get_pet_config):
        """When no Content-Type header is present, fallback path should work."""
        mock_response = httpx.Response(
            200,
            content=b'{"ok": true}',
            request=httpx.Request("GET", "https://example.com"),
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(get_pet_config)
        result = await callable_fn(ctx, pet_id=1)
        assert result == {"ok": True}
