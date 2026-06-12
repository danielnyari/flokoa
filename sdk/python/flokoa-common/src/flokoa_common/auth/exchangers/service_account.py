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

"""Credential fetcher for Google Service Account."""

from __future__ import annotations

import asyncio
import logging
import time
from datetime import UTC

from flokoa_common.auth.auth_credential import AuthCredential, AuthCredentialTypes, HttpAuth, HttpCredentials
from flokoa_common.auth.auth_schemes import AuthScheme

from .base import AuthCredentialMissingError, BaseAuthCredentialExchanger

logger = logging.getLogger(__name__)

# Reuse a cached token until within this many seconds of its expiry.
_EXPIRY_BUFFER_SECONDS = 30

_GOOGLE_AUTH_HINT = (
    "google-auth is required for service account credential exchange. "
    "Install the optional extra: pip install 'flokoa-common[google]'"
)


class ServiceAccountCredentialExchanger(BaseAuthCredentialExchanger):
    """Fetches credentials for Google Service Account.

    Uses Application Default Credentials when ``use_default_credential=True``,
    otherwise the service account key provided in the auth credential. The
    credential's ``scopes`` are required in both cases — there is no implicit
    cloud-platform default.

    Minted tokens are cached in memory per (account, scopes) and reused until
    within ``_EXPIRY_BUFFER_SECONDS`` of expiry, so a token is not requested
    from Google on every call. The synchronous ``google-auth`` refresh runs in
    a worker thread to keep async callers unblocked.

    Requires the optional ``google-auth`` dependency (the ``[google]`` extra
    on flokoa-common); it is imported lazily so the other auth schemes work
    without it.
    """

    def __init__(self) -> None:
        # (account identity, scopes) -> (bearer token, quota project, expires_at epoch)
        self._token_cache: dict[tuple[str, tuple[str, ...]], tuple[str, str | None, float]] = {}

    async def exchange_credential(
        self,
        auth_scheme: AuthScheme,
        auth_credential: AuthCredential | None = None,
    ) -> AuthCredential:
        """Exchanges the service account auth credential for an access token.

        If auth_credential contains a service account credential, it will be used
        to fetch an access token. Otherwise, the default service credential will be
        used for fetching an access token.

        Args:
            auth_scheme: The auth scheme.
            auth_credential: The auth credential.

        Returns:
            An AuthCredential in HTTPBearer format, containing the access token.

        Raises:
            AuthCredentialMissingError: When the credential or its scopes are
                missing, or the token exchange fails.
            ImportError: When google-auth is not installed.
        """
        service_account = auth_credential.service_account if auth_credential is not None else None
        if (
            auth_credential is None
            or service_account is None
            or (service_account.service_account_credential is None and not service_account.use_default_credential)
        ):
            raise AuthCredentialMissingError(
                "Service account credentials are missing. Please provide them, or set"
                " `use_default_credential = True` to use application default"
                " credential in a hosted service like Cloud Run."
            )

        scopes = service_account.scopes
        if not scopes:
            raise AuthCredentialMissingError(
                "Service account scopes are required: set `scopes` on the service"
                " account credential (no default scope is assumed)."
            )

        cache_key = self._cache_key(auth_credential)
        cached = self._token_cache.get(cache_key)
        if cached is not None:
            token, quota_project_id, expires_at = cached
            if time.time() < expires_at - _EXPIRY_BUFFER_SECONDS:
                return self._to_bearer_credential(token, quota_project_id)
            del self._token_cache[cache_key]

        # google-auth's refresh is synchronous (blocking network I/O) — run it
        # in a worker thread so the event loop is not stalled.
        token, quota_project_id, expires_at = await asyncio.to_thread(self._mint_token, auth_credential)

        if expires_at > 0:
            self._token_cache[cache_key] = (token, quota_project_id, expires_at)

        return self._to_bearer_credential(token, quota_project_id)

    @staticmethod
    def _cache_key(auth_credential: AuthCredential) -> tuple[str, tuple[str, ...]]:
        service_account = auth_credential.service_account
        assert service_account is not None  # validated by exchange_credential  # noqa: S101
        if service_account.use_default_credential:
            identity = "<application-default>"
        else:
            assert service_account.service_account_credential is not None  # noqa: S101
            identity = service_account.service_account_credential.client_email
        return identity, tuple(service_account.scopes)

    @staticmethod
    def _to_bearer_credential(token: str, quota_project_id: str | None) -> AuthCredential:
        return AuthCredential(
            auth_type=AuthCredentialTypes.HTTP,  # Store as a bearer token
            http=HttpAuth(
                scheme="bearer",
                credentials=HttpCredentials(token=token),
                additional_headers={
                    "x-goog-user-project": quota_project_id,
                }
                if quota_project_id
                else None,
            ),
        )

    def _mint_token(self, auth_credential: AuthCredential) -> tuple[str, str | None, float]:
        """Mints a fresh bearer token via google-auth (blocking).

        Returns:
            Tuple of (token, quota project id, expires_at epoch seconds —
            0.0 when the token carries no expiry, in which case it is not
            cached and a fresh token is minted on the next call).
        """
        try:
            import google.auth
            from google.auth.transport.requests import Request
            from google.oauth2 import service_account
        except ImportError as e:
            raise ImportError(_GOOGLE_AUTH_HINT) from e

        config = auth_credential.service_account
        assert config is not None  # validated by exchange_credential  # noqa: S101

        try:
            if config.use_default_credential:
                credentials, project_id = google.auth.default(scopes=config.scopes)
                quota_project_id = getattr(credentials, "quota_project_id", None) or project_id
            else:
                assert config.service_account_credential is not None  # noqa: S101
                credentials = service_account.Credentials.from_service_account_info(
                    config.service_account_credential.model_dump(), scopes=config.scopes
                )
                quota_project_id = None

            credentials.refresh(Request())

        except Exception as e:
            raise AuthCredentialMissingError(f"Failed to exchange service account token: {e}") from e

        expiry = getattr(credentials, "expiry", None)
        expires_at = expiry.replace(tzinfo=UTC).timestamp() if expiry is not None else 0.0
        return credentials.token, quota_project_id, expires_at
