# flokoa-common

Shared internals for the flokoa Python packages: OpenAPI spec parsing
(`flokoa_common.utils.openapi`), auth credential models and async credential
exchangers (`flokoa_common.auth`), URL/SSRF validation
(`flokoa_common.utils.url_validation`), and text utilities.

This package exists so `flokoa`, `flokoa-openapi`, and `flokoa-codemode-mcp`
can share one implementation — it is **not a public API promise**. Depend on
it through those packages; interfaces here change whenever their consumers
need them to.

## The auth layer

`flokoa_common.auth` carries the OpenAPI-style auth scheme/credential models
and the exchangers that turn a configured credential into a usable one at
call time: `OAuth2CredentialExchanger` (expiry-aware token refresh, 30s
buffer, single-flight under concurrency), `ServiceAccountCredentialExchanger`
(Google SA JSON or ADC → bearer token, cached per account+scopes), and
`AutoAuthCredentialExchanger`, which dispatches on the credential type and
reuses exchanger instances so token caches survive across calls:

```python
from flokoa_common.auth.exchangers import AutoAuthCredentialExchanger
from flokoa_common.auth.helpers import credential_to_param, token_to_scheme_credential

scheme, credential = token_to_scheme_credential("apikey", "header", "X-API-Key", "...")

exchanger = AutoAuthCredentialExchanger()
exchanged = await exchanger.exchange_credential(scheme, credential)  # async
param, kwargs = credential_to_param(scheme, exchanged)  # inject into the request
```

Exchange is async end-to-end; secret-bearing fields are excluded from
`repr()`. Token endpoints are SSRF-validated before use.

## The `[google]` extra

`ServiceAccountCredentialExchanger` needs `google-auth`, which is optional:

```bash
pip install "flokoa-common[google]"
```

Without the extra, the service-account path raises a clear `ImportError`;
everything else works.

## Tests

```bash
uv run --package flokoa-common pytest flokoa-common/tests
```
