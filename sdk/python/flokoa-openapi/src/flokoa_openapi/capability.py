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

"""The ``flokoa.OpenAPI`` capability: an OpenAPI spec as agent tools.

Spec entry form (hydrated by ``Agent.from_spec(...,
custom_capability_types=[OpenAPI])`` — this is how the flokoa runner
instantiates it)::

    capabilities:
      - flokoa.OpenAPI:
          spec: {...}                  # inline OpenAPI document (dict or JSON/YAML string)
          base_url: https://api.example.com
          auth:
            scheme: {type: http, scheme: bearer}
            credential: {auth_type: http, http: {scheme: bearer, credentials: {token: "${secret:API_TOKEN}"}}}
          defer_tools: auto            # all | none | auto(threshold)
          prefix: petstore
"""

from __future__ import annotations

from dataclasses import dataclass, field, fields
from typing import Any

from flokoa_common.auth.auth_credential import AuthCredential
from flokoa_common.auth.auth_schemes import AuthScheme
from flokoa_common.auth.helpers import dict_to_auth_scheme
from pydantic_ai.capabilities import AbstractCapability

from .toolset import DEFAULT_DEFER_THRESHOLD, DeferLoadingMode, OpenAPIToolset


@dataclass
class OpenAPI(AbstractCapability[Any]):
    """Capability that turns an OpenAPI document into typed agent tools.

    Stacks with core ``ToolSearch`` (auto-injected by pydantic-ai whenever
    deferred tools exist) and harness ``CodeMode`` — the capability itself
    only owns the spec→ToolDefinition mapping and the hardened ``call_tool``
    execution path; discovery and sandboxing come from upstream.

    ```python
    from pydantic_ai import Agent
    from flokoa_openapi import OpenAPI

    agent = Agent(
        "openai:gpt-5",
        capabilities=[OpenAPI(spec=petstore_spec, base_url="https://petstore.example.com")],
    )
    ```

    Note: the tool-deferral knob is named ``defer_tools`` (not
    ``defer_loading``) because ``AbstractCapability.defer_loading`` already
    exists upstream with different semantics — hiding the *capability* until
    the model loads it via ``load_capability``.
    """

    spec: dict[str, Any] | str | None = None
    """The OpenAPI document, inline: a dict or a JSON/YAML string. Required."""

    base_url: str | None = None
    """Overrides the spec's ``servers`` entry (relative server paths are preserved)."""

    headers: dict[str, str] | None = None
    """Default headers added to every request."""

    auth: dict[str, Any] | None = None
    """Auth configuration: ``{"scheme": <OpenAPI securityScheme dict>, "credential": <AuthCredential dict>}``.

    The scheme follows the OpenAPI ``securitySchemes`` object grammar
    (``apiKey``, ``http``, ``oauth2``, ``openIdConnect``); the credential is
    an ``flokoa_common.auth.auth_credential.AuthCredential``-shaped dict.
    Secret values must arrive as ``${secret:NAME}`` placeholders resolved by
    the runner before hydration — never inline plaintext in a ConfigMap.
    Applied to operations that don't carry their own security scheme.
    """

    defer_tools: DeferLoadingMode = "auto"
    """Tool deferral: ``'all'`` hides every tool behind ToolSearch discovery,
    ``'none'`` keeps all tools native, ``'auto'`` defers when the operation
    count exceeds ``defer_threshold``."""

    defer_threshold: int = DEFAULT_DEFER_THRESHOLD
    """Operation-count threshold used by ``defer_tools='auto'``."""

    prefix: str | None = None
    """Prefix prepended to every tool name."""

    allowed_operations: list[str] | None = None
    """Allow-list of operation names (snake_case operationId, unprefixed)."""

    verify_ssl: bool | str = True
    """SSL verification: bool or a CA bundle path."""

    timeout: float = 30.0
    """HTTP request timeout in seconds."""

    allow_internal: bool = False
    """Skip SSRF private-range checks — only for operator-resolved, trusted
    in-cluster service URLs."""

    toolset_id: str | None = field(default=None)
    """Optional id for the contributed toolset (needed for durable execution)."""

    def __post_init__(self) -> None:
        if self.spec is None:
            raise ValueError("flokoa.OpenAPI requires `spec`: an inline OpenAPI document as a dict or JSON/YAML string")

    def __repr__(self) -> str:
        """Dataclass-style repr with secret-bearing fields redacted.

        ``auth`` carries credential material and ``headers`` may carry static
        secrets (e.g. API keys) — the auth dict is fully redacted and header
        values are masked (keys stay visible for debugging).
        """
        parts: list[str] = []
        for f in fields(self):
            value = getattr(self, f.name)
            if f.name == "auth":
                rendered = "<redacted>" if value is not None else "None"
            elif f.name == "headers" and value is not None:
                rendered = "{" + ", ".join(f"{k!r}: '<redacted>'" for k in value) + "}"
            else:
                rendered = repr(value)
            parts.append(f"{f.name}={rendered}")
        return f"{type(self).__name__}({', '.join(parts)})"

    @classmethod
    def get_serialization_name(cls) -> str:
        return "flokoa.OpenAPI"

    def get_toolset(self) -> OpenAPIToolset:
        """Builds the configured toolset (called once at agent construction)."""
        assert self.spec is not None  # validated in __post_init__  # noqa: S101
        auth_scheme, auth_credential = _parse_auth(self.auth)
        return OpenAPIToolset(
            self.spec,
            base_url=self.base_url,
            headers=self.headers,
            auth_scheme=auth_scheme,
            auth_credential=auth_credential,
            defer_loading=self.defer_tools,
            defer_threshold=self.defer_threshold,
            prefix=self.prefix,
            allowed_operations=self.allowed_operations,
            verify_ssl=self.verify_ssl,
            timeout=self.timeout,
            allow_internal=self.allow_internal,
            id=self.toolset_id,
        )


def _parse_auth(auth: dict[str, Any] | None) -> tuple[AuthScheme | None, AuthCredential | None]:
    """Builds typed auth objects from the capability's ``auth`` config dict."""
    if auth is None:
        return None, None
    scheme_dict = auth.get("scheme")
    if not scheme_dict:
        raise ValueError("flokoa.OpenAPI `auth` config requires a `scheme` entry (OpenAPI securityScheme dict)")
    auth_scheme = dict_to_auth_scheme(scheme_dict)
    credential_dict = auth.get("credential")
    auth_credential = AuthCredential.model_validate(credential_dict) if credential_dict else None
    return auth_scheme, auth_credential
