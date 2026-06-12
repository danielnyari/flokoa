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

"""ServiceAccountCredentialExchanger: validation, scopes, token caching, threading."""

from __future__ import annotations

import importlib.util
import time

import pytest
from fastapi.openapi.models import HTTPBearer
from flokoa_common.auth.auth_credential import (
    AuthCredential,
    AuthCredentialTypes,
    ServiceAccount,
    ServiceAccountCredential,
)
from flokoa_common.auth.exchangers.base import AuthCredentialMissingError
from flokoa_common.auth.exchangers.service_account import ServiceAccountCredentialExchanger

pytestmark = pytest.mark.anyio


def _sa_credential(scopes: list[str] | None = None, use_default: bool = False) -> AuthCredential:
    sa_cred = None
    if not use_default:
        sa_cred = ServiceAccountCredential.model_construct(
            type="service_account",
            project_id="proj",
            private_key_id="kid",
            private_key="-----BEGIN PRIVATE KEY-----",
            client_email="svc@proj.iam.gserviceaccount.com",
            client_id="cid",
            auth_uri="https://accounts.google.com/o/oauth2/auth",
            token_uri="https://oauth2.googleapis.com/token",
            auth_provider_x509_cert_url="https://www.googleapis.com/oauth2/v1/certs",
            client_x509_cert_url="https://example.com/cert",
            universe_domain="googleapis.com",
        )
    return AuthCredential(
        auth_type=AuthCredentialTypes.SERVICE_ACCOUNT,
        service_account=ServiceAccount(
            service_account_credential=sa_cred,
            scopes=scopes if scopes is not None else ["https://www.googleapis.com/auth/devstorage.read_only"],
            use_default_credential=use_default,
        ),
    )


async def test_missing_credential_raises():
    exchanger = ServiceAccountCredentialExchanger()
    with pytest.raises(AuthCredentialMissingError, match="missing"):
        await exchanger.exchange_credential(HTTPBearer(), None)


async def test_missing_sa_and_no_default_raises():
    exchanger = ServiceAccountCredentialExchanger()
    cred = AuthCredential(
        auth_type=AuthCredentialTypes.SERVICE_ACCOUNT,
        service_account=ServiceAccount(scopes=["scope"], use_default_credential=False),
    )
    with pytest.raises(AuthCredentialMissingError, match="missing"):
        await exchanger.exchange_credential(HTTPBearer(), cred)


async def test_empty_scopes_raises():
    """No implicit cloud-platform default: scopes are required."""
    exchanger = ServiceAccountCredentialExchanger()
    with pytest.raises(AuthCredentialMissingError, match="scopes are required"):
        await exchanger.exchange_credential(HTTPBearer(), _sa_credential(scopes=[]))


async def test_mint_runs_in_thread_and_returns_bearer(monkeypatch):
    exchanger = ServiceAccountCredentialExchanger()
    monkeypatch.setattr(exchanger, "_mint_token", lambda credential: ("minted-token", "quota-proj", time.time() + 3600))

    result = await exchanger.exchange_credential(HTTPBearer(), _sa_credential())
    assert result.auth_type == AuthCredentialTypes.HTTP
    assert result.http.scheme == "bearer"
    assert result.http.credentials.token == "minted-token"
    assert result.http.additional_headers == {"x-goog-user-project": "quota-proj"}


async def test_token_is_cached_until_expiry(monkeypatch):
    exchanger = ServiceAccountCredentialExchanger()
    calls = {"n": 0}

    def fake_mint(credential):
        calls["n"] += 1
        return (f"token-{calls['n']}", None, time.time() + 3600)

    monkeypatch.setattr(exchanger, "_mint_token", fake_mint)
    cred = _sa_credential()

    first = await exchanger.exchange_credential(HTTPBearer(), cred)
    second = await exchanger.exchange_credential(HTTPBearer(), cred)
    assert calls["n"] == 1
    assert first.http.credentials.token == second.http.credentials.token == "token-1"


async def test_expired_cache_entry_is_refreshed(monkeypatch):
    exchanger = ServiceAccountCredentialExchanger()
    calls = {"n": 0}

    def fake_mint(credential):
        calls["n"] += 1
        # Expires within the 30s buffer — never reusable.
        return (f"token-{calls['n']}", None, time.time() + 5)

    monkeypatch.setattr(exchanger, "_mint_token", fake_mint)
    cred = _sa_credential()

    await exchanger.exchange_credential(HTTPBearer(), cred)
    result = await exchanger.exchange_credential(HTTPBearer(), cred)
    assert calls["n"] == 2
    assert result.http.credentials.token == "token-2"


async def test_token_without_expiry_is_not_cached(monkeypatch):
    exchanger = ServiceAccountCredentialExchanger()
    calls = {"n": 0}

    def fake_mint(credential):
        calls["n"] += 1
        return (f"token-{calls['n']}", None, 0.0)

    monkeypatch.setattr(exchanger, "_mint_token", fake_mint)
    cred = _sa_credential()

    await exchanger.exchange_credential(HTTPBearer(), cred)
    await exchanger.exchange_credential(HTTPBearer(), cred)
    assert calls["n"] == 2


async def test_cache_is_scoped_per_scopes(monkeypatch):
    exchanger = ServiceAccountCredentialExchanger()
    calls = {"n": 0}

    def fake_mint(credential):
        calls["n"] += 1
        return (f"token-{calls['n']}", None, time.time() + 3600)

    monkeypatch.setattr(exchanger, "_mint_token", fake_mint)

    await exchanger.exchange_credential(HTTPBearer(), _sa_credential(scopes=["scope-a"]))
    await exchanger.exchange_credential(HTTPBearer(), _sa_credential(scopes=["scope-b"]))
    assert calls["n"] == 2


@pytest.mark.skipif(
    importlib.util.find_spec("google") is not None and importlib.util.find_spec("google.auth") is not None,
    reason="google-auth is installed",
)
async def test_missing_google_auth_raises_clear_import_error():
    exchanger = ServiceAccountCredentialExchanger()
    with pytest.raises(ImportError, match=r"flokoa-common\[google\]"):
        await exchanger.exchange_credential(HTTPBearer(), _sa_credential())
