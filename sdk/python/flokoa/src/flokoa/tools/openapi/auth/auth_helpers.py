# Copyright 2026 Google LLC
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

from __future__ import annotations

import logging
from typing import Any, Literal

import httpx
from fastapi.openapi.models import APIKey, APIKeyIn, HTTPBase, HTTPBearer, OAuth2, OpenIdConnect, Schema
from flokoa_common.auth.auth_credential import (
    AuthCredential,
    AuthCredentialTypes,
    HttpAuth,
    HttpCredentials,
    OAuth2Auth,
    ServiceAccount,
    ServiceAccountCredential,
)
from flokoa_common.auth.auth_schemes import AuthScheme, AuthSchemeType, OpenIdConnectWithConfig
from flokoa_common.utils.openapi.common import ApiParameter
from pydantic import BaseModel, ValidationError

from ....utils.url_validation import validate_url

logger = logging.getLogger("flokoa." + __name__)


class OpenIdConfig(BaseModel):
    """Represents OpenID Connect configuration.

    Attributes:
        client_id: The client ID.
        auth_uri: The authorization URI.
        token_uri: The token URI.
        client_secret: The client secret.
        redirect_uri: The redirect URI.

    Example:
        config = OpenIdConfig(
            client_id="your_client_id",
            auth_uri="https://accounts.google.com/o/oauth2/auth",
            token_uri="https://oauth2.googleapis.com/token",
            client_secret="your_client_secret",
            redirect_uri="http://localhost:8080/callback",
        )
    """

    client_id: str
    auth_uri: str
    token_uri: str
    client_secret: str
    redirect_uri: str | None = None


def token_to_scheme_credential(
    token_type: Literal["apikey", "oauth2Token"],
    location: Literal["header", "query", "cookie"] | None = None,
    name: str | None = None,
    credential_value: str | None = None,
) -> tuple[AuthScheme, AuthCredential]:
    """Creates a AuthScheme and AuthCredential for API key or bearer token.

    Examples:
    ```
    # API Key in header
    auth_scheme, auth_credential = token_to_scheme_credential("apikey", "header",
    "X-API-Key", "your_api_key_value")

    # API Key in query parameter
    auth_scheme, auth_credential = token_to_scheme_credential("apikey", "query",
    "api_key", "your_api_key_value")

    # OAuth2 Bearer Token in Authorization header
    auth_scheme, auth_credential = token_to_scheme_credential("oauth2Token",
    "header", "Authorization", "your_bearer_token_value")
    ```

    Args:
        type: 'apikey' or 'oauth2Token'.
        location: 'header', 'query', or 'cookie' (only 'header' for oauth2Token).
        name: The name of the header, query parameter, or cookie.
        credential_value:  The value of the API Key/ Token.

    Returns:
        Tuple: (AuthScheme, AuthCredential)

    Raises:
        ValueError: For invalid type or location.
    """
    if token_type == "apikey":
        in_: APIKeyIn
        if location == "header":
            in_ = APIKeyIn.header
        elif location == "query":
            in_ = APIKeyIn.query
        elif location == "cookie":
            in_ = APIKeyIn.cookie
        else:
            raise ValueError(f"Invalid location for apiKey: {location}")
        auth_scheme = APIKey(**{
            "type": AuthSchemeType.apiKey,
            "in": in_,
            "name": name,
        })
        if credential_value:
            auth_credential = AuthCredential(auth_type=AuthCredentialTypes.API_KEY, api_key=credential_value)
        else:
            auth_credential = None

        return auth_scheme, auth_credential

    elif token_type == "oauth2Token":
        # ignore location. OAuth2 Bearer Token is always in Authorization header.
        auth_scheme = HTTPBearer(bearerFormat="JWT")  # Common format, can be omitted.
        if credential_value:
            auth_credential = AuthCredential(
                auth_type=AuthCredentialTypes.HTTP,
                http=HttpAuth(
                    scheme="bearer",
                    credentials=HttpCredentials(token=credential_value),
                ),
            )
        else:
            auth_credential = None

        return auth_scheme, auth_credential

    else:
        raise ValueError(f"Invalid security scheme type: {type}")


def service_account_dict_to_scheme_credential(
    config: dict[str, Any],
    scopes: list[str],
) -> tuple[AuthScheme, AuthCredential]:
    """Creates AuthScheme and AuthCredential for Google Service Account.

    Returns a bearer token scheme, and a service account credential.

    Args:
        config: A ServiceAccount object containing the Google Service Account
          configuration.
        scopes: A list of scopes to be used.

    Returns:
        Tuple: (AuthScheme, AuthCredential)
    """
    auth_scheme = HTTPBearer(bearerFormat="JWT")
    service_account = ServiceAccount(
        service_account_credential=ServiceAccountCredential.model_construct(**config),
        scopes=scopes,
    )
    auth_credential = AuthCredential(
        auth_type=AuthCredentialTypes.SERVICE_ACCOUNT,
        service_account=service_account,
    )
    return auth_scheme, auth_credential


def service_account_scheme_credential(
    config: ServiceAccount,
) -> tuple[AuthScheme, AuthCredential]:
    """Creates AuthScheme and AuthCredential for Google Service Account.

    Returns a bearer token scheme, and a service account credential.

    Args:
        config: A ServiceAccount object containing the Google Service Account
          configuration.

    Returns:
        Tuple: (AuthScheme, AuthCredential)
    """
    auth_scheme = HTTPBearer(bearerFormat="JWT")
    auth_credential = AuthCredential(auth_type=AuthCredentialTypes.SERVICE_ACCOUNT, service_account=config)
    return auth_scheme, auth_credential


def openid_dict_to_scheme_credential(
    config_dict: dict[str, Any],
    scopes: list[str],
    credential_dict: dict[str, Any],
) -> tuple[OpenIdConnectWithConfig, AuthCredential]:
    """Constructs OpenID scheme and credential from configuration and credential dictionaries.

    Args:
        config_dict: Dictionary containing OpenID Connect configuration,  must
          include at least 'authorization_endpoint' and 'token_endpoint'.
        scopes: List of scopes to be used.
        credential_dict: Dictionary containing credential information, must
          include 'client_id', 'client_secret', and 'scopes'.  May optionally
          include 'redirect_uri'.

    Returns:
        Tuple: (OpenIdConnectWithConfig, AuthCredential)

    Raises:
        ValueError: If required fields are missing in the input dictionaries.
    """

    # Validate and create the OpenIdConnectWithConfig scheme
    try:
        config_dict["scopes"] = scopes
        # If user provides the OpenID Config as a static dict, it may not contain
        # openIdConnect URL.
        if "openIdConnectUrl" not in config_dict:
            config_dict["openIdConnectUrl"] = ""
        openid_scheme = OpenIdConnectWithConfig.model_validate(config_dict)
    except ValidationError as e:
        raise ValueError(f"Invalid OpenID Connect configuration: {e}") from e

    # Attempt to adjust credential_dict if this is a key downloaded from Google
    # OAuth config
    if len(list(credential_dict.values())) == 1:
        credential_value = next(iter(credential_dict.values()))
        if "client_id" in credential_value and "client_secret" in credential_value:
            credential_dict = credential_value

    # Validate credential_dict
    required_credential_fields = ["client_id", "client_secret"]
    missing_fields = [field for field in required_credential_fields if field not in credential_dict]
    if missing_fields:
        raise ValueError(f"Missing required fields in credential_dict: {', '.join(missing_fields)}")

    # Construct AuthCredential
    auth_credential = AuthCredential(
        auth_type=AuthCredentialTypes.OPEN_ID_CONNECT,
        oauth2=OAuth2Auth(
            client_id=credential_dict["client_id"],
            client_secret=credential_dict["client_secret"],
            redirect_uri=credential_dict.get("redirect_uri", None),
        ),
    )

    return openid_scheme, auth_credential


def openid_url_to_scheme_credential(
    openid_url: str, scopes: list[str], credential_dict: dict[str, Any]
) -> tuple[OpenIdConnectWithConfig, AuthCredential]:
    """Constructs OpenID scheme and credential from OpenID URL, scopes, and credential dictionary.

    Fetches OpenID configuration from the provided URL.

    Args:
        openid_url: The OpenID Connect discovery URL.
        scopes: List of scopes to be used.
        credential_dict: Dictionary containing credential information, must
          include at least "client_id" and "client_secret", may optionally include
          "redirect_uri" and "scope"

    Returns:
        Tuple: (AuthScheme, AuthCredential)

    Raises:
        ValueError: If the OpenID URL is invalid, fetching fails, or required
          fields are missing.
        httpx.HTTPStatusError or httpx.RequestError: If there's an error during the
            HTTP request.
    """
    validate_url(openid_url)
    try:
        response = httpx.get(openid_url, timeout=10)
        response.raise_for_status()
        config_dict = response.json()
    except httpx.RequestError as e:
        raise ValueError(f"Failed to fetch OpenID configuration from {openid_url}: {e}") from e
    except ValueError as e:
        raise ValueError(f"Invalid JSON response from OpenID configuration endpoint {openid_url}: {e}") from e

    # Add openIdConnectUrl to config dict
    config_dict["openIdConnectUrl"] = openid_url

    return openid_dict_to_scheme_credential(config_dict, scopes, credential_dict)


INTERNAL_AUTH_PREFIX = "_auth_prefix_vaf_"


def _bearer_param_and_kwargs(
    token: str,
    description: str = "Bearer token",
) -> tuple[ApiParameter, dict[str, Any]]:
    """Creates a Bearer Authorization header parameter and matching kwargs."""
    param = ApiParameter(
        original_name="Authorization",
        param_location="header",
        param_schema=Schema(type="string"),
        description=description,
        py_name=INTERNAL_AUTH_PREFIX + "Authorization",
    )
    kwargs = {param.py_name: f"Bearer {token}"}
    return param, kwargs


def credential_to_param(
    auth_scheme: AuthScheme,
    auth_credential: AuthCredential,
) -> tuple[ApiParameter | None, dict[str, Any] | None]:
    """Converts AuthCredential and AuthScheme to a Parameter and a dictionary for additional kwargs.

    This function supports all credential types returned by the exchangers:
    - API Key
    - HTTP Bearer (native HTTPBearer scheme)
    - OpenID Connect (bearer token from OIDC exchange)
    - OAuth2 (bearer token from OAuth2 exchange)
    - Service Account (bearer token from service account exchange)

    Args:
        auth_scheme: The AuthScheme object.
        auth_credential: The AuthCredential object.

    Returns:
        Tuple: (ApiParameter, Dict[str, Any])
    """
    if not auth_credential:
        return None, None

    # --- API Key ---
    if auth_scheme.type_ == AuthSchemeType.apiKey and auth_credential.api_key:
        param_name = auth_scheme.name or ""
        python_name = INTERNAL_AUTH_PREFIX + param_name
        if auth_scheme.in_ == APIKeyIn.header:
            param_location = "header"
        elif auth_scheme.in_ == APIKeyIn.query:
            param_location = "query"
        elif auth_scheme.in_ == APIKeyIn.cookie:
            param_location = "cookie"
        else:
            raise ValueError(f"Invalid API Key location: {auth_scheme.in_}")

        param = ApiParameter(
            original_name=param_name,
            param_location=param_location,
            param_schema=Schema(type="string"),
            description=auth_scheme.description or "",
            py_name=python_name,
        )
        kwargs = {param.py_name: auth_credential.api_key}
        return param, kwargs

    # --- OpenID Connect scheme ---
    # OpenID Connect credentials are exchanged into HTTP bearer tokens by
    # OAuth2CredentialExchanger.  If the credential has already been exchanged
    # (auth_type == HTTP with a bearer token), emit the Authorization header.
    # If it still carries raw OAuth2 data (pre-exchange), return None so the
    # caller can trigger exchange first.
    if auth_scheme.type_ == AuthSchemeType.openIdConnect:
        if (
            auth_credential.auth_type == AuthCredentialTypes.HTTP
            and auth_credential.http
            and auth_credential.http.credentials
            and auth_credential.http.credentials.token
        ):
            return _bearer_param_and_kwargs(
                auth_credential.http.credentials.token,
                description=auth_scheme.description or "OpenID Connect bearer token",
            )
        # Credential has not been exchanged yet
        return None, None

    # --- OAuth2 scheme ---
    if auth_scheme.type_ == AuthSchemeType.oauth2:
        if (
            auth_credential.auth_type == AuthCredentialTypes.HTTP
            and auth_credential.http
            and auth_credential.http.credentials
            and auth_credential.http.credentials.token
        ):
            return _bearer_param_and_kwargs(
                auth_credential.http.credentials.token,
                description=auth_scheme.description or "OAuth2 bearer token",
            )
        return None, None

    # --- Native HTTP scheme (Bearer / other) ---
    if auth_credential.auth_type == AuthCredentialTypes.HTTP:
        if auth_credential.http and auth_credential.http.credentials and auth_credential.http.credentials.token:
            return _bearer_param_and_kwargs(
                auth_credential.http.credentials.token,
                description=auth_scheme.description or "Bearer token",
            )
        if (
            auth_credential.http
            and auth_credential.http.credentials
            and (auth_credential.http.credentials.username or auth_credential.http.credentials.password)
        ):
            raise NotImplementedError("Basic Authentication is not supported.")
        raise ValueError("Invalid HTTP auth credentials")

    raise ValueError("Invalid security scheme and credential combination")


def resolve_openid_connect_scheme(
    openid_connect_url: str,
) -> OpenIdConnectWithConfig:
    """Fetches the OpenID Connect discovery document and returns a fully resolved scheme.

    Retrieves the ``/.well-known/openid-configuration`` document from the
    provider and maps the standard discovery fields to an
    ``OpenIdConnectWithConfig`` instance.

    Args:
        openid_connect_url: The OpenID Connect discovery URL
            (e.g. ``https://accounts.google.com/.well-known/openid-configuration``).

    Returns:
        An ``OpenIdConnectWithConfig`` with endpoints populated from the
        discovery document.

    Raises:
        ValueError: If the URL cannot be fetched, returns invalid JSON, or
            is missing the required ``authorization_endpoint`` and
            ``token_endpoint`` fields.
    """
    validate_url(openid_connect_url)
    try:
        response = httpx.get(openid_connect_url, timeout=10)
        response.raise_for_status()
        config = response.json()
    except httpx.RequestError as e:
        raise ValueError(f"Failed to fetch OpenID Connect discovery document from {openid_connect_url}: {e}") from e
    except ValueError as e:
        raise ValueError(f"Invalid JSON in OpenID Connect discovery document from {openid_connect_url}: {e}") from e

    authorization_endpoint = config.get("authorization_endpoint")
    token_endpoint = config.get("token_endpoint")
    if not authorization_endpoint or not token_endpoint:
        raise ValueError(
            f"OpenID Connect discovery document at {openid_connect_url} is missing "
            "required 'authorization_endpoint' and/or 'token_endpoint' fields."
        )

    return OpenIdConnectWithConfig(
        openIdConnectUrl=openid_connect_url,
        authorization_endpoint=authorization_endpoint,
        token_endpoint=token_endpoint,
        userinfo_endpoint=config.get("userinfo_endpoint"),
        revocation_endpoint=config.get("revocation_endpoint"),
        token_endpoint_auth_methods_supported=config.get("token_endpoint_auth_methods_supported"),
        grant_types_supported=config.get("grant_types_supported"),
        scopes=config.get("scopes_supported"),
    )


def dict_to_auth_scheme(data: dict[str, Any]) -> AuthScheme:
    """Converts a dictionary to a FastAPI AuthScheme object.

    For ``openIdConnect`` schemes that include an ``openIdConnectUrl``, this
    function fetches the discovery document and returns a fully resolved
    ``OpenIdConnectWithConfig`` instead of a bare ``OpenIdConnect`` stub.
    If the discovery fetch fails, it falls back to the basic
    ``OpenIdConnect`` model.

    Args:
        data: The dictionary representing the security scheme.

    Returns:
        A AuthScheme object (APIKey, HTTPBase, OAuth2, OpenIdConnectWithConfig,
        OpenIdConnect, or HTTPBearer).

    Raises:
        ValueError: If the 'type' field is missing or invalid, or if the
            dictionary cannot be converted to the corresponding Pydantic model.

    Example:
    ```python
    api_key_data = {
        "type": "apiKey",
        "in": "header",
        "name": "X-API-Key",
    }
    api_key_scheme = dict_to_auth_scheme(api_key_data)

    bearer_data = {
        "type": "http",
        "scheme": "bearer",
        "bearerFormat": "JWT",
    }
    bearer_scheme = dict_to_auth_scheme(bearer_data)


    oauth2_data = {
        "type": "oauth2",
        "flows": {
            "authorizationCode": {
                "authorizationUrl": "https://example.com/auth",
                "tokenUrl": "https://example.com/token",
            }
        }
    }
    oauth2_scheme = dict_to_auth_scheme(oauth2_data)

    openid_data = {
        "type": "openIdConnect",
        "openIdConnectUrl": "https://example.com/.well-known/openid-configuration"
    }
    openid_scheme = dict_to_auth_scheme(openid_data)


    ```
    """
    if "type" not in data:
        raise ValueError("Missing 'type' field in security scheme dictionary.")

    security_type = data["type"]
    try:
        if security_type == "apiKey":
            return APIKey.model_validate(data)
        elif security_type == "http":
            if data.get("scheme") == "bearer":
                return HTTPBearer.model_validate(data)
            else:
                return HTTPBase.model_validate(data)  # Generic HTTP
        elif security_type == "oauth2":
            return OAuth2.model_validate(data)
        elif security_type == "openIdConnect":
            # If the dict already contains endpoint fields, build
            # OpenIdConnectWithConfig directly (user-provided config).
            if "authorization_endpoint" in data and "token_endpoint" in data:
                return OpenIdConnectWithConfig.model_validate(data)

            # Attempt to fetch and resolve the discovery document.
            openid_url = data.get("openIdConnectUrl")
            if openid_url:
                try:
                    return resolve_openid_connect_scheme(openid_url)
                except ValueError:
                    logger.warning(
                        "Failed to resolve OpenID Connect discovery for %s; "
                        "falling back to basic OpenIdConnect scheme.",
                        openid_url,
                    )

            return OpenIdConnect.model_validate(data)
        else:
            raise ValueError(f"Invalid security scheme type: {security_type}")

    except ValidationError as e:
        raise ValueError(f"Invalid security scheme data: {e}") from e
