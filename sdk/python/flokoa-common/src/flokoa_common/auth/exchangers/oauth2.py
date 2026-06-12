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

"""Credential exchanger for OAuth2 and OpenID Connect."""

from __future__ import annotations

import asyncio
import logging
import time
from typing import Any

import httpx

from flokoa_common.auth.auth_credential import (
    AuthCredential,
    AuthCredentialTypes,
    HttpAuth,
    HttpCredentials,
    OAuth2Auth,
)
from flokoa_common.auth.auth_schemes import AuthScheme, AuthSchemeType, OpenIdConnectWithConfig
from flokoa_common.utils.url_validation import validate_url

from .base import BaseAuthCredentialExchanger

logger = logging.getLogger(__name__)

# Refresh tokens proactively when within this many seconds of expiration
_EXPIRY_BUFFER_SECONDS = 30

# Number of retry attempts for token refresh on transient failures
_REFRESH_MAX_RETRIES = 2

# Timeout (seconds) for token endpoint requests
_REFRESH_TIMEOUT_SECONDS = 10


class OAuth2CredentialExchanger(BaseAuthCredentialExchanger):
    """Fetches and refreshes credentials for OAuth2 and OpenID Connect.

    Token caching: refreshed tokens (and their ``expires_at``) are written
    back onto the ``OAuth2Auth`` credential in place, so a long-lived
    credential object acts as the expiry-aware in-memory cache — a refresh
    only happens when the cached token is within ``_EXPIRY_BUFFER_SECONDS``
    of expiry (or its lifetime is unknown).

    Concurrent refreshes are deduplicated per exchanger instance: an
    ``asyncio.Lock`` with a double-checked expiry test ensures N concurrent
    callers sharing an expired credential trigger exactly one token-endpoint
    request; the fast path (token still valid) never touches the lock.
    """

    def __init__(self, http_client: httpx.AsyncClient | None = None):
        """Initializes the exchanger.

        Args:
            http_client: Optional shared httpx.AsyncClient used for token
                endpoint requests (connection pooling, custom transports in
                tests). When None, a short-lived client is created per refresh.
        """
        self._http_client = http_client
        self._refresh_lock = asyncio.Lock()

    def _check_scheme_credential_type(
        self,
        auth_scheme: AuthScheme,
        auth_credential: AuthCredential | None = None,
    ):
        if not auth_credential:
            raise ValueError("auth_credential is empty. Please create AuthCredential using OAuth2Auth.")

        if auth_scheme.type_ not in (
            AuthSchemeType.openIdConnect,
            AuthSchemeType.oauth2,
        ):
            raise ValueError(
                "Invalid security scheme, expect AuthSchemeType.openIdConnect or "
                f"AuthSchemeType.oauth2 auth scheme, but got {auth_scheme.type_}"
            )

        if not auth_credential.oauth2 and not auth_credential.http:
            raise ValueError(
                "auth_credential is not configured with oauth2. Please create AuthCredential and set OAuth2Auth."
            )

    def _is_token_expired(self, auth_credential: AuthCredential) -> bool:
        """Checks whether the OAuth2 access token is expired or about to expire.

        Uses ``expires_at`` (absolute epoch) when available. A credential that
        carries only the relative ``expires_in`` (no absolute timestamp) is
        treated as **needing refresh**: without a creation timestamp the
        remaining lifetime is unknowable, so assuming validity would risk
        sending an expired token upstream. Returns ``False`` only when
        neither field is set (e.g. a static, non-expiring token).
        """
        if not auth_credential.oauth2:
            return False

        oauth2 = auth_credential.oauth2
        now = int(time.time())

        if oauth2.expires_at is not None:
            return now >= (oauth2.expires_at - _EXPIRY_BUFFER_SECONDS)

        # expires_in is relative to an unknown creation time — treat the
        # token's validity as unknown and refresh proactively.
        return oauth2.expires_in is not None

    def _get_token_endpoint(self, auth_scheme: AuthScheme) -> str | None:
        """Extracts the token endpoint URL from the auth scheme."""
        if isinstance(auth_scheme, OpenIdConnectWithConfig):
            return auth_scheme.token_endpoint

        # Standard OAuth2 SecurityScheme — dig into OAuthFlows
        flows = getattr(auth_scheme, "flows", None)
        if not flows:
            return None

        for flow_name in ("authorizationCode", "clientCredentials", "password", "implicit"):
            flow = getattr(flows, flow_name, None)
            if flow is not None:
                token_url = getattr(flow, "tokenUrl", None)
                if token_url:
                    return token_url

        return None

    @staticmethod
    def _update_credential_with_tokens(oauth2: OAuth2Auth, token_response: dict[str, Any]) -> None:
        """Updates an OAuth2Auth credential in-place from a token endpoint response."""
        if token_response.get("access_token"):
            oauth2.access_token = token_response["access_token"]
        if token_response.get("refresh_token"):
            oauth2.refresh_token = token_response["refresh_token"]
        if token_response.get("expires_at"):
            oauth2.expires_at = int(token_response["expires_at"])
        elif token_response.get("expires_in"):
            oauth2.expires_in = int(token_response["expires_in"])
            oauth2.expires_at = int(time.time()) + oauth2.expires_in

    async def _post_token_request(self, token_endpoint: str, data: dict[str, str]) -> httpx.Response:
        """POSTs to the token endpoint, via the shared client or a one-shot one."""
        if self._http_client is not None:
            return await self._http_client.post(token_endpoint, data=data, timeout=_REFRESH_TIMEOUT_SECONDS)
        async with httpx.AsyncClient() as client:
            return await client.post(token_endpoint, data=data, timeout=_REFRESH_TIMEOUT_SECONDS)

    async def _refresh_access_token(
        self,
        auth_scheme: AuthScheme,
        auth_credential: AuthCredential,
    ) -> AuthCredential:
        """Refreshes an expired access token using the refresh token.

        Performs an async HTTP POST to the token endpoint with the
        ``refresh_token`` grant type.  On success the credential is updated
        in-place with the new tokens and expiry metadata.  On failure the
        original credential is returned unchanged so callers can attempt to
        use the (possibly still valid) access token — the failure is logged
        loudly so it is observable, never silent.

        Retries up to ``_REFRESH_MAX_RETRIES`` times on transient HTTP /
        network errors. Credential material (tokens, client secrets) is never
        logged.
        """
        oauth2 = auth_credential.oauth2
        if not oauth2 or not oauth2.refresh_token:
            logger.debug("No refresh token available; skipping token refresh.")
            return auth_credential

        token_endpoint = self._get_token_endpoint(auth_scheme)
        if not token_endpoint:
            logger.warning("Cannot refresh token: no token endpoint found in auth scheme.")
            return auth_credential

        # validate_url does a blocking DNS lookup; keep it off the event loop.
        await asyncio.to_thread(validate_url, token_endpoint)

        data = {
            "grant_type": "refresh_token",
            "refresh_token": oauth2.refresh_token,
        }
        if oauth2.client_id:
            data["client_id"] = oauth2.client_id
        if oauth2.client_secret:
            data["client_secret"] = oauth2.client_secret

        last_error: Exception | None = None
        for attempt in range(_REFRESH_MAX_RETRIES + 1):
            try:
                response = await self._post_token_request(token_endpoint, data)
                response.raise_for_status()

            except httpx.HTTPStatusError as e:
                status = e.response.status_code
                if status in (400, 401):
                    # Expected terminal condition (revoked/expired refresh
                    # token), logged loudly but without a traceback.
                    logger.error(  # noqa: TRY400
                        "OAuth2 token refresh rejected (HTTP %d) by %s: the refresh token is likely "
                        "expired or revoked; proceeding with the stale access token.",
                        status,
                        token_endpoint,
                    )
                    return auth_credential
                last_error = e
                logger.debug("OAuth2 token refresh attempt %d failed (HTTP %d).", attempt + 1, status)

            except httpx.RequestError as e:
                last_error = e
                logger.debug("OAuth2 token refresh attempt %d failed: %s", attempt + 1, e)

            else:
                self._update_credential_with_tokens(oauth2, response.json())
                logger.debug("OAuth2 token refreshed successfully.")
                return auth_credential

        logger.error(
            "OAuth2 token refresh against %s failed after %d attempts (%s: %s); "
            "proceeding with the stale access token.",
            token_endpoint,
            _REFRESH_MAX_RETRIES + 1,
            type(last_error).__name__ if last_error else "unknown",
            last_error,
        )
        return auth_credential

    def generate_auth_token(
        self,
        auth_credential: AuthCredential,
    ) -> AuthCredential:
        """Generates an auth token from the authorization response.

        Args:
            auth_credential: The auth credential.

        Returns:
            An AuthCredential object containing the HTTP bearer access token. If the
            HTTP bearer token cannot be generated, return the original credential.
        """
        if not auth_credential.oauth2 or not auth_credential.oauth2.access_token:
            return auth_credential

        # Return the access token as a bearer token.
        updated_credential = AuthCredential(
            auth_type=AuthCredentialTypes.HTTP,  # Store as a bearer token
            http=HttpAuth(
                scheme="bearer",
                credentials=HttpCredentials(token=auth_credential.oauth2.access_token),
            ),
        )
        return updated_credential

    async def exchange_credential(
        self,
        auth_scheme: AuthScheme,
        auth_credential: AuthCredential | None = None,
    ) -> AuthCredential | None:
        """Exchanges the OAuth2/OpenID Connect auth credential for an access token.

        If the access token is expired and a refresh token is available, the
        token is automatically refreshed before being converted to a bearer
        credential.

        Args:
            auth_scheme: The auth scheme.
            auth_credential: The auth credential.

        Returns:
            An AuthCredential object containing the HTTP Bearer access token,
            or None when no usable token is available (e.g. the credential
            still requires an interactive authorization flow).

        Raises:
            ValueError: If the auth scheme or auth credential is invalid.
        """
        self._check_scheme_credential_type(auth_scheme, auth_credential)
        assert auth_credential is not None  # narrowed by the check above  # noqa: S101

        # If token is already HTTPBearer token, do nothing assuming that this token
        #  is valid.
        if auth_credential.http:
            return auth_credential

        # Check if access token needs refreshing. The lock + re-check
        # deduplicates concurrent refreshes (thundering herd): whoever wins
        # the lock refreshes the shared credential in place, and the waiters
        # see the updated expiry and skip their own refresh. The fast path
        # above (valid token, exits via the expiry check) never acquires.
        if auth_credential.oauth2 and self._is_token_expired(auth_credential):
            async with self._refresh_lock:
                if self._is_token_expired(auth_credential):
                    auth_credential = await self._refresh_access_token(auth_scheme, auth_credential)

        # If access token is exchanged, exchange a HTTPBearer token.
        if auth_credential.oauth2 and auth_credential.oauth2.access_token:
            return self.generate_auth_token(auth_credential)

        return None
