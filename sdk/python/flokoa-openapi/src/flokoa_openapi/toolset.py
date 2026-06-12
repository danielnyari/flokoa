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

"""OpenAPI spec → pydantic-ai toolset with typed, hardened REST execution.

One :class:`~pydantic_ai.tools.ToolDefinition` is emitted per operation, with
``parameters_json_schema`` from the operation's parameters/body,
``return_schema`` from its 2xx JSON response schema (typed sandbox signatures
under harness CodeMode), and ``defer_loading`` per configuration (so large
specs ride core ToolSearch instead of flooding the model's context).

``call_tool`` is the salvaged REST execution engine: async credential
exchange via :mod:`flokoa_common.auth.exchangers`, auth parameter injection
that never reaches the model-visible schema, SSRF URL validation,
content-type-routed response parsing, and sanitized error envelopes
(upstream error bodies are logged server-side only).
"""

from __future__ import annotations

import asyncio
import json
import logging
import ssl
from dataclasses import dataclass
from typing import Any, Literal
from urllib.parse import quote

import httpx
import yaml
from fastapi.encoders import jsonable_encoder
from fastapi.openapi.models import Operation, Schema
from flokoa_common.auth.auth_credential import AuthCredential
from flokoa_common.auth.auth_schemes import AuthScheme
from flokoa_common.auth.exchangers import AutoAuthCredentialExchanger
from flokoa_common.auth.helpers import credential_to_param
from flokoa_common.utils.openapi.openapi_spec_parser import OpenApiSpecParser, OperationEndpoint
from flokoa_common.utils.openapi.operation_parser import OperationParser
from flokoa_common.utils.openapi.request_builder import prepare_request_params
from flokoa_common.utils.url_validation import SSRFError, validate_url
from pydantic_ai import AbstractToolset
from pydantic_ai._run_context import RunContext
from pydantic_ai.tools import ToolDefinition
from pydantic_ai.toolsets.abstract import ToolsetTool
from pydantic_core import SchemaValidator, core_schema

logger = logging.getLogger(__name__)

DeferLoadingMode = Literal["all", "none", "auto"]

DEFAULT_DEFER_THRESHOLD = 25
"""Operation count above which ``defer_loading='auto'`` defers every tool."""

TOOL_METADATA_KEY = "flokoa_openapi"
"""Metadata flag set on every ToolDefinition emitted by this toolset."""

_ERROR_BODY_LOG_LIMIT = 500

# Tool argument schemas come from the OpenAPI document, not from Python
# signatures — validation happens upstream against parameters_json_schema,
# so the per-tool pydantic-core validator is permissive (same approach as
# `Tool.from_schema`).
_ANY_ARGS_VALIDATOR = SchemaValidator(schema=core_schema.any_schema())


@dataclass
class _OperationTool:
    """Everything needed to describe and execute one OpenAPI operation."""

    tool_def: ToolDefinition
    endpoint: OperationEndpoint
    operation: Operation
    parser: OperationParser
    auth_scheme: AuthScheme | None
    auth_credential: AuthCredential | None


class OpenAPIToolset(AbstractToolset[Any]):
    """Parses an OpenAPI spec into a pydantic-ai toolset of typed REST tools.

    Usage::

        toolset = OpenAPIToolset(spec_dict, base_url="https://api.example.com")
        agent = Agent("openai:gpt-5", toolsets=[toolset])

    The toolset owns an ``httpx.AsyncClient`` configured with the given SSL
    and timeout settings; it is opened/closed with the toolset's async
    context (which the agent manages during runs). Calls made outside the
    context use a one-shot client.
    """

    def __init__(
        self,
        spec: dict[str, Any] | str,
        *,
        base_url: str | None = None,
        headers: dict[str, str] | None = None,
        auth_scheme: AuthScheme | None = None,
        auth_credential: AuthCredential | None = None,
        defer_loading: DeferLoadingMode = "auto",
        defer_threshold: int = DEFAULT_DEFER_THRESHOLD,
        prefix: str | None = None,
        allowed_operations: list[str] | None = None,
        verify_ssl: bool | str | ssl.SSLContext = True,
        timeout: float = 30.0,
        allow_internal: bool = False,
        max_retries: int | None = None,
        id: str | None = None,  # noqa: A002
        transport: httpx.AsyncBaseTransport | None = None,
    ):
        """Initialize the toolset from an OpenAPI document.

        Args:
            spec: The OpenAPI spec as a dict, or a JSON/YAML string.
            base_url: Overrides the spec's ``servers`` entry. A relative
                server path in the spec (e.g. ``/api/v3``) is preserved by
                appending it to the override.
            headers: Default headers added to every request (auth and
                per-operation headers take precedence).
            auth_scheme: Auth scheme applied to operations that don't carry
                their own security scheme.
            auth_credential: Auth credential paired with ``auth_scheme``.
            defer_loading: ``'all'`` marks every tool ``defer_loading=True``
                (hidden until ToolSearch discovery), ``'none'`` keeps all
                tools native, ``'auto'`` defers all tools when the operation
                count exceeds ``defer_threshold``.
            defer_threshold: Operation count threshold for ``'auto'``.
            prefix: Prefix prepended to every tool name (sanitized to a valid
                identifier).
            allowed_operations: Optional allow-list of operation names
                (snake_case of the operationId, before prefixing). Operations
                not listed are dropped.
            verify_ssl: SSL verification for the HTTP client (bool, CA bundle
                path, or ``ssl.SSLContext``).
            timeout: Request timeout in seconds for the HTTP client.
            allow_internal: Skip SSRF private-range checks — only for
                operator-resolved, trusted in-cluster service URLs.
            max_retries: Max retries for each tool; defaults to the agent's.
            id: Optional unique toolset id (required for durable execution
                environments).
            transport: Optional httpx transport for the owned client (custom
                routing, ``httpx.MockTransport`` in tests).
        """
        self._id = id
        self._headers = dict(headers) if headers else {}
        self._verify_ssl = verify_ssl
        self._timeout = timeout
        self._transport = transport
        self._allow_internal = allow_internal
        self._max_retries = max_retries
        self._exchanger = AutoAuthCredentialExchanger()

        self._client: httpx.AsyncClient | None = None
        self._enter_count = 0
        self._client_lock = asyncio.Lock()

        spec_dict = _load_spec(spec)
        spec_dict = _apply_base_url(spec_dict, base_url)
        operations = OpenApiSpecParser().parse(spec_dict)

        if allowed_operations is not None:
            allowed = set(allowed_operations)
            operations = [op for op in operations if op.name in allowed]

        defer = _resolve_defer(defer_loading, defer_threshold, len(operations))

        self._tools: dict[str, _OperationTool] = {}
        for parsed in operations:
            parser = OperationParser.load(parsed.operation, parsed.parameters, parsed.return_value)
            name = _tool_name(parsed.name, prefix=prefix, taken=self._tools.keys())

            tool_def = ToolDefinition(
                name=name,
                description=_describe(parsed.operation),
                parameters_json_schema=parser.get_json_schema(),
                return_schema=_extract_return_schema(parsed.operation),
                defer_loading=defer,
                metadata={TOOL_METADATA_KEY: True},
            )

            self._tools[name] = _OperationTool(
                tool_def=tool_def,
                endpoint=parsed.endpoint,
                operation=parsed.operation,
                parser=parser,
                auth_scheme=parsed.auth_scheme or auth_scheme,
                auth_credential=parsed.auth_credential or auth_credential,
            )
            logger.debug("Parsed OpenAPI tool: %s (defer_loading=%s)", name, defer)

    @property
    def id(self) -> str | None:
        return self._id

    @property
    def tool_definitions(self) -> list[ToolDefinition]:
        """The ToolDefinitions for every operation in this toolset."""
        return [entry.tool_def for entry in self._tools.values()]

    async def __aenter__(self) -> OpenAPIToolset:
        async with self._client_lock:
            if self._enter_count == 0:
                self._client = self._build_client()
            self._enter_count += 1
        return self

    async def __aexit__(self, *args: Any) -> bool | None:
        async with self._client_lock:
            self._enter_count -= 1
            if self._enter_count == 0 and self._client is not None:
                await self._client.aclose()
                self._client = None
        return None

    async def get_tools(self, ctx: RunContext[Any]) -> dict[str, ToolsetTool[Any]]:
        max_retries = self._max_retries if self._max_retries is not None else ctx.max_retries
        return {
            name: ToolsetTool(
                toolset=self,
                tool_def=entry.tool_def,
                max_retries=max_retries,
                args_validator=_ANY_ARGS_VALIDATOR,
            )
            for name, entry in self._tools.items()
        }

    async def call_tool(
        self, name: str, tool_args: dict[str, Any], ctx: RunContext[Any], tool: ToolsetTool[Any]
    ) -> Any:
        entry = self._tools.get(name)
        if entry is None:
            raise ValueError(f"Unknown tool {name!r} in {self.label}")
        return await self._execute(entry, tool_args)

    def _build_client(self) -> httpx.AsyncClient:
        if self._transport is not None:
            return httpx.AsyncClient(transport=self._transport, timeout=self._timeout)
        return httpx.AsyncClient(verify=self._verify_ssl, timeout=self._timeout)

    async def _request(self, request_params: dict[str, Any]) -> httpx.Response:
        if self._client is not None:
            return await self._client.request(**request_params)
        # Not entered as a context manager — use a one-shot client so no
        # connection is leaked.
        async with self._build_client() as client:
            return await client.request(**request_params)

    async def _execute(self, entry: _OperationTool, kwargs: dict[str, Any]) -> Any:  # noqa: C901
        name = entry.tool_def.name
        # Model-provided args only — auth material is merged below and never logged.
        logger.debug("TOOL CALL: %s(args=%s)", name, sorted(kwargs.keys()))

        # Exchange auth credentials (e.g. OAuth2 token refresh, service account)
        auth_scheme = entry.auth_scheme
        auth_credential = entry.auth_credential
        if auth_credential and auth_scheme:
            try:
                auth_credential = await self._exchanger.exchange_credential(auth_scheme, auth_credential)
            except Exception as e:
                # Sanitized envelope, mirroring the Timeout/RequestError
                # handling below: log the exception type server-side only —
                # exchanger errors can carry credential material.
                logger.warning("Credential exchange failed for tool %s: %s", name, type(e).__name__)
                return {"error": f"Tool {name} credential exchange failed"}

        # Build parameter list from operation parser
        api_params = entry.parser.get_parameters().copy()
        api_args = dict(kwargs)

        # Fill in missing required args with defaults
        for api_param in api_params:
            if (
                api_param.py_name
                and api_param.py_name not in api_args
                and (
                    api_param.required
                    and isinstance(api_param.param_schema, Schema)
                    and api_param.param_schema.default is not None
                )
            ):
                api_args[api_param.py_name] = api_param.param_schema.default

        # Collect auth additional headers
        auth_additional_headers: dict[str, str] | None = None
        if auth_credential and auth_credential.http and auth_credential.http.additional_headers:
            auth_additional_headers = dict(auth_credential.http.additional_headers)

        # Attach auth parameters (kept out of the model-visible schema via
        # the INTERNAL_AUTH_PREFIX py_name convention)
        if auth_credential and auth_scheme:
            auth_param, auth_args = credential_to_param(auth_scheme, auth_credential)
            if auth_param and auth_args:
                api_params = [auth_param, *api_params]
                api_args.update(auth_args)

        request_params = prepare_request_params(
            endpoint=entry.endpoint,
            operation=entry.operation,
            default_headers=self._headers,
            tool_name=name,
            parameters=api_params,
            kwargs=api_args,
            auth_additional_headers=auth_additional_headers,
        )

        # httpx deprecates per-request cookies; carry them as a header instead.
        # Values are percent-encoded so a value cannot inject `;`-separated
        # cookie attributes or additional cookies.
        cookies = request_params.pop("cookies", None)
        if cookies:
            cookie_header = "; ".join(f"{k}={quote(str(v), safe='')}" for k, v in cookies.items())
            request_params.setdefault("headers", {}).setdefault("Cookie", cookie_header)

        # Validate the constructed URL against SSRF attacks. Runs in a worker
        # thread: validation does blocking DNS resolution, which must not
        # stall the event loop on every tool call.
        # Skip for relative URLs (no scheme) — those can't target hosts directly.
        constructed_url = request_params["url"]
        if constructed_url.startswith(("http://", "https://")):
            try:
                await asyncio.to_thread(validate_url, constructed_url, allow_internal=self._allow_internal)
            except SSRFError as e:
                logger.warning("SSRF validation failed for tool %s: %s", name, e)
                return {"error": f"Tool {name} request blocked: URL failed security validation"}

        try:
            response = await self._request(request_params)
        except httpx.TimeoutException as e:
            logger.warning("Timeout calling tool %s: %s", name, e)
            return {"error": f"Tool {name} request timed out"}
        except httpx.RequestError as e:
            logger.warning("Request error calling tool %s: %s", name, e)
            return {"error": f"Tool {name} request failed: {type(e).__name__}"}

        # Scrubbed response log: status and length only, never headers.
        logger.info(
            "TOOL RESPONSE: %s %s %s - Status: %d, Content-Length: %s",
            name,
            request_params.get("method", "").upper(),
            request_params.get("url", ""),
            response.status_code,
            response.headers.get("content-length", "unknown"),
        )

        try:
            response.raise_for_status()
        except httpx.HTTPStatusError:
            # Log truncated error details server-side only; avoid leaking
            # full upstream response bodies to the model.
            raw = response.content.decode("utf-8", errors="replace")
            truncated = (raw[:_ERROR_BODY_LOG_LIMIT] + "...") if len(raw) > _ERROR_BODY_LOG_LIMIT else raw
            logger.warning(
                "API call failed for tool %s: Status %d - %s",
                name,
                response.status_code,
                truncated,
            )
            return {"error": f"Tool {name} execution failed. Status Code: {response.status_code}"}

        return _parse_response(response)


def _load_spec(spec: dict[str, Any] | str) -> dict[str, Any]:
    """Accept a spec dict, or a JSON/YAML string (JSON tried first)."""
    if isinstance(spec, dict):
        return spec
    try:
        loaded = json.loads(spec)
    except ValueError:
        loaded = yaml.safe_load(spec)
    if not isinstance(loaded, dict):
        raise TypeError(f"OpenAPI spec string must parse to a mapping, got {type(loaded).__name__}")
    return loaded


def _apply_base_url(spec_dict: dict[str, Any], base_url: str | None) -> dict[str, Any]:
    """Override the spec's servers with ``base_url``, preserving relative server paths."""
    if not base_url:
        return spec_dict
    override = base_url.rstrip("/")
    original_servers = spec_dict.get("servers", [])
    if original_servers:
        first_url = original_servers[0].get("url", "")
        if first_url.startswith("/") and first_url != "/":
            override = override + "/" + first_url.strip("/")
    return {**spec_dict, "servers": [{"url": override}]}


def _resolve_defer(mode: DeferLoadingMode, threshold: int, operation_count: int) -> bool:
    if mode == "all":
        return True
    if mode == "none":
        return False
    if mode == "auto":
        return operation_count > threshold
    raise ValueError(f"Invalid defer_loading mode: {mode!r} (expected 'all', 'none', or 'auto')")


def _tool_name(operation_name: str, *, prefix: str | None, taken: Any) -> str:
    """Builds a unique, valid-identifier tool name from a parsed operation name.

    ``operation_name`` is ``ParsedOperation.name`` — the unprefixed
    snake_case operation name (``allowed_operations`` matches this too).
    CodeMode sanitizes names again before rendering sandbox functions, but
    clean identifiers help native tool calling too.
    """
    name = operation_name
    if prefix:
        name = f"{prefix}_{name}" if name else prefix
    if not name:
        name = "operation"
    if not name.isidentifier():
        name = f"op_{name}"
    base = name
    counter = 2
    while name in taken:
        name = f"{base}_{counter}"
        counter += 1
    return name


def _describe(operation: Operation) -> str:
    """Tool description from the operation's summary and description."""
    summary = (operation.summary or "").strip()
    description = (operation.description or "").strip()
    if summary and description and summary != description:
        return f"{summary}\n\n{description}"
    return description or summary


def _extract_return_schema(operation: Operation) -> dict[str, Any] | None:
    """The operation's 2xx ``application/json`` response schema, or None.

    ``$ref``s were already resolved spec-wide by the parser, so the schema on
    the Operation model is self-contained. The lowest 2xx status code with a
    JSON media type wins; non-JSON-only responses yield None.
    """
    responses = operation.responses or {}

    def _code_sort_key(code: str) -> tuple[int, str]:
        try:
            return (int(code), code)
        except ValueError:  # e.g. "2XX"
            return (299, code)

    for code in sorted((c for c in responses if c.startswith("2")), key=_code_sort_key):
        content = responses[code].content
        if not content:
            continue
        media_type = next(
            (m for m in content if m.split(";")[0].strip().lower() == "application/json" or m.endswith("+json")),
            None,
        )
        if media_type is None:
            continue
        schema = content[media_type].schema_
        if schema is None:
            continue
        encoded = jsonable_encoder(schema, exclude_none=True)
        return encoded or None
    return None


def _parse_response(response: httpx.Response) -> Any:
    """Routes response parsing on the Content-Type header."""
    content_type = response.headers.get("content-type", "")
    mime = content_type.split(";")[0].strip().lower()

    if mime == "application/json" or mime.endswith("+json"):
        try:
            return response.json()
        except ValueError:
            logger.debug("Content-Type indicated JSON but parsing failed")
            return {"text": response.text}
    elif mime.startswith("text/"):
        return {"text": response.text}
    elif mime in ("application/xml", "application/xhtml+xml") or mime.endswith("+xml"):
        return {"text": response.text, "content_type": mime}
    elif mime == "application/octet-stream":
        return {"binary_length": len(response.content), "content_type": mime}
    else:
        # Fallback: try JSON first, then text
        try:
            return response.json()
        except ValueError:
            return {"text": response.text}
