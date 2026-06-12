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

"""AutoAuthCredentialExchanger dispatch behavior."""

from __future__ import annotations

import time

import pytest
from fastapi.openapi.models import HTTPBearer
from flokoa_common.auth.auth_credential import (
    AuthCredential,
    AuthCredentialTypes,
    HttpAuth,
    HttpCredentials,
    OAuth2Auth,
)
from flokoa_common.auth.auth_schemes import AuthScheme, OpenIdConnectWithConfig
from flokoa_common.auth.exchangers import AutoAuthCredentialExchanger, BaseAuthCredentialExchanger

pytestmark = pytest.mark.anyio


def _make_openid_scheme():
    return OpenIdConnectWithConfig(
        openIdConnectUrl="https://example.com/.well-known/openid-configuration",
        authorization_endpoint="https://example.com/auth",
        token_endpoint="https://example.com/token",
    )


async def test_none_credential_returns_none():
    exchanger = AutoAuthCredentialExchanger()
    assert await exchanger.exchange_credential(HTTPBearer(), None) is None


async def test_unhandled_type_passes_through():
    exchanger = AutoAuthCredentialExchanger()
    cred = AuthCredential(auth_type=AuthCredentialTypes.API_KEY, api_key="key")
    result = await exchanger.exchange_credential(HTTPBearer(), cred)
    assert result is cred


async def test_oauth2_dispatches_to_oauth2_exchanger():
    exchanger = AutoAuthCredentialExchanger()
    cred = AuthCredential(
        auth_type=AuthCredentialTypes.OAUTH2,
        oauth2=OAuth2Auth(access_token="tok"),
    )
    result = await exchanger.exchange_credential(_make_openid_scheme(), cred)
    assert result.auth_type == AuthCredentialTypes.HTTP
    assert result.http.credentials.token == "tok"


async def test_custom_exchanger_override():
    class StaticExchanger(BaseAuthCredentialExchanger):
        async def exchange_credential(self, auth_scheme, auth_credential=None):
            return AuthCredential(
                auth_type=AuthCredentialTypes.HTTP,
                http=HttpAuth(scheme="bearer", credentials=HttpCredentials(token="static")),
            )

    exchanger = AutoAuthCredentialExchanger(custom_exchangers={AuthCredentialTypes.OAUTH2: StaticExchanger})
    cred = AuthCredential(auth_type=AuthCredentialTypes.OAUTH2, oauth2=OAuth2Auth(access_token="ignored"))
    result = await exchanger.exchange_credential(HTTPBearer(), cred)
    assert result.http.credentials.token == "static"


async def test_exchanger_instances_are_reused():
    """Instance reuse preserves per-exchanger token caches across calls."""
    instances: list[object] = []

    class RecordingExchanger(BaseAuthCredentialExchanger):
        def __init__(self):
            instances.append(self)
            self.created = time.time()

        async def exchange_credential(
            self, auth_scheme: AuthScheme, auth_credential: AuthCredential | None = None
        ) -> AuthCredential | None:
            return auth_credential

    exchanger = AutoAuthCredentialExchanger(custom_exchangers={AuthCredentialTypes.OAUTH2: RecordingExchanger})
    cred = AuthCredential(auth_type=AuthCredentialTypes.OAUTH2, oauth2=OAuth2Auth(access_token="tok"))

    await exchanger.exchange_credential(HTTPBearer(), cred)
    await exchanger.exchange_credential(HTTPBearer(), cred)
    assert len(instances) == 1
