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

from __future__ import annotations

import logging
import ssl
from dataclasses import dataclass, field
from typing import Any, Callable, Dict, List, Optional, Union

import httpx
from fastapi.openapi.models import Operation, Schema
from pydantic_ai import RunContext, Tool

from ...auth.auth_credential import AuthCredential
from ...auth.auth_schemes import AuthScheme
from ..utils import _to_snake_case
from .auth.auth_helpers import credential_to_param
from .auth.credential_exchangers.auto_auth_credential_exchanger import AutoAuthCredentialExchanger
from .common import ApiParameter
from .openapi_spec_parser import OperationEndpoint, ParsedOperation
from .operation_parser import OperationParser

logger = logging.getLogger("flokoa." + __name__)


@dataclass
class OpenAPIDeps:
    """Dependencies injected at agent.run() time for OpenAPI tools.

    Users compose this into their own Deps dataclass, or use it directly
    as the agent's deps_type.

    Attributes:
        client: An httpx.AsyncClient for making HTTP requests. Enables
            connection pooling across tool calls.
        header_provider: Optional callable that returns dynamic headers.
            Called on each request to inject headers like correlation IDs
            or authentication tokens.
    """

    client: httpx.AsyncClient
    header_provider: Callable[[], Dict[str, str]] | None = None


@dataclass
class RestApiToolConfig:
    """Static configuration for a single REST API operation.

    Holds everything needed to build an HTTP request for one OpenAPI operation.
    Created from a ParsedOperation (parsed from an OpenAPI spec) and used by
    create_rest_api_tool() to produce a Pydantic AI Tool.
    """

    name: str
    description: str
    endpoint: OperationEndpoint
    operation: Operation
    operation_parser: OperationParser
    auth_scheme: AuthScheme | None = None
    auth_credential: AuthCredential | None = None
    ssl_verify: bool | str | ssl.SSLContext | None = None
    default_headers: Dict[str, str] = field(default_factory=dict)
    credential_exchanger: AutoAuthCredentialExchanger = field(
        default_factory=AutoAuthCredentialExchanger
    )

    @classmethod
    def from_parsed_operation(
        cls,
        parsed: ParsedOperation,
        ssl_verify: Optional[Union[bool, str, ssl.SSLContext]] = None,
    ) -> RestApiToolConfig:
        """Create a RestApiToolConfig from a ParsedOperation.

        Args:
            parsed: A ParsedOperation from the OpenAPI spec parser.
            ssl_verify: SSL certificate verification option.

        Returns:
            A RestApiToolConfig instance.
        """
        operation_parser = OperationParser.load(
            parsed.operation, parsed.parameters, parsed.return_value
        )
        tool_name = _to_snake_case(operation_parser.get_function_name())[:60]

        return cls(
            name=tool_name,
            description=parsed.operation.description or parsed.operation.summary or "",
            endpoint=parsed.endpoint,
            operation=parsed.operation,
            operation_parser=operation_parser,
            auth_scheme=parsed.auth_scheme,
            auth_credential=parsed.auth_credential,
            ssl_verify=ssl_verify,
        )


def _prepare_request_params(
    endpoint: OperationEndpoint,
    operation: Operation,
    default_headers: Dict[str, str],
    tool_name: str,
    parameters: List[ApiParameter],
    kwargs: Dict[str, Any],
    auth_additional_headers: Dict[str, str] | None = None,
) -> Dict[str, Any]:
    """Build httpx request parameters from operation config and call arguments.

    This is a pure function extracted from the original RestApiTool method.
    It maps OpenAPI parameters to their HTTP locations (path, query, header,
    cookie) and constructs the request body from the operation's requestBody.

    Args:
        endpoint: The operation endpoint (base_url, path, method).
        operation: The OpenAPI Operation object.
        default_headers: Headers to include in every request.
        tool_name: Name of the tool (for User-Agent).
        parameters: List of ApiParameter objects for this operation.
        kwargs: Keyword arguments from the tool call.
        auth_additional_headers: Extra headers from auth credentials.

    Returns:
        A dict suitable for httpx.AsyncClient.request(**params).
    """
    method = endpoint.method.lower()
    if not method:
        raise ValueError("Operation method not found.")

    path_params: Dict[str, Any] = {}
    query_params: Dict[str, Any] = {}
    header_params: Dict[str, Any] = {}
    cookie_params: Dict[str, Any] = {}

    header_params["User-Agent"] = f"flokoa (tool: {tool_name})"

    if auth_additional_headers:
        header_params.update(auth_additional_headers)

    params_map: Dict[str, ApiParameter] = {p.py_name: p for p in parameters}

    for param_k, v in kwargs.items():
        param_obj = params_map.get(param_k)
        if not param_obj:
            continue

        original_k = param_obj.original_name
        param_location = param_obj.param_location

        if param_location == "path":
            path_params[original_k] = v
        elif param_location == "query":
            if v:
                query_params[original_k] = v
        elif param_location == "header":
            header_params[original_k] = v
        elif param_location == "cookie":
            cookie_params[original_k] = v

    # Construct URL
    base_url = endpoint.base_url or ""
    base_url = base_url[:-1] if base_url.endswith("/") else base_url
    url = f"{base_url}{endpoint.path.format(**path_params)}"

    # Construct body
    body_kwargs: Dict[str, Any] = {}
    request_body = operation.requestBody
    if request_body:
        for mime_type, media_type_object in request_body.content.items():
            schema = media_type_object.schema_
            body_data = None

            if schema.type == "object":
                body_data = {}
                for param in parameters:
                    if param.param_location == "body" and param.py_name in kwargs:
                        body_data[param.original_name] = kwargs[param.py_name]

            elif schema.type == "array":
                for param in parameters:
                    if param.param_location == "body" and param.py_name == "array":
                        body_data = kwargs.get("array")
                        break
            else:
                for param in parameters:
                    if param.param_location == "body" and not param.original_name:
                        body_data = kwargs.get(param.py_name) if param.py_name in kwargs else None
                        break

            if mime_type == "application/json" or mime_type.endswith("+json"):
                if body_data is not None:
                    body_kwargs["json"] = body_data
            elif mime_type == "application/x-www-form-urlencoded":
                body_kwargs["data"] = body_data
            elif mime_type == "multipart/form-data":
                body_kwargs["files"] = body_data
            elif mime_type in ("application/octet-stream", "text/plain"):
                body_kwargs["data"] = body_data

            if mime_type:
                header_params["Content-Type"] = mime_type
            break  # Process only the first mime_type

    filtered_query_params: Dict[str, Any] = {
        k: v for k, v in query_params.items() if v is not None
    }

    for key, value in default_headers.items():
        header_params.setdefault(key, value)

    return {
        "method": method,
        "url": url,
        "params": filtered_query_params,
        "headers": header_params,
        "cookies": cookie_params,
        **body_kwargs,
    }


def create_rest_api_callable(config: RestApiToolConfig) -> Callable:
    """Create an async callable that executes a REST API operation.

    The returned callable accepts RunContext[OpenAPIDeps] as its first argument
    and **kwargs from the tool schema. It handles auth credential exchange,
    request preparation, and HTTP execution via the deps' httpx client.

    Args:
        config: Static configuration for the REST API operation.

    Returns:
        An async function suitable for use with Tool.from_schema(takes_ctx=True).
    """

    async def rest_api_call(ctx: RunContext[OpenAPIDeps], **kwargs: Any) -> Dict[str, Any]:
        # Exchange auth credentials (e.g. OAuth2 token refresh, service account)
        auth_scheme = config.auth_scheme
        auth_credential = config.auth_credential

        if auth_credential and auth_scheme:
            auth_credential = config.credential_exchanger.exchange_credential(
                auth_scheme, auth_credential
            )

        # Build parameter list from operation parser
        api_params = config.operation_parser.get_parameters().copy()
        api_args = dict(kwargs)

        # Fill in missing required args with defaults
        for api_param in api_params:
            if api_param.py_name not in api_args:
                if (
                    api_param.required
                    and isinstance(api_param.param_schema, Schema)
                    and api_param.param_schema.default is not None
                ):
                    api_args[api_param.py_name] = api_param.param_schema.default

        # Collect auth additional headers
        auth_additional_headers: Dict[str, str] | None = None
        if auth_credential and auth_credential.http and auth_credential.http.additional_headers:
            auth_additional_headers = dict(auth_credential.http.additional_headers)

        # Attach auth parameters
        if auth_credential and auth_scheme:
            auth_param, auth_args = credential_to_param(auth_scheme, auth_credential)
            if auth_param and auth_args:
                api_params = [auth_param] + api_params
                api_args.update(auth_args)

        # Build request params
        request_params = _prepare_request_params(
            endpoint=config.endpoint,
            operation=config.operation,
            default_headers=config.default_headers,
            tool_name=config.name,
            parameters=api_params,
            kwargs=api_args,
            auth_additional_headers=auth_additional_headers,
        )

        # Apply SSL verification
        if config.ssl_verify is not None:
            request_params["verify"] = config.ssl_verify

        # Apply dynamic headers from deps
        if ctx.deps.header_provider is not None:
            provider_headers = ctx.deps.header_provider()
            if provider_headers:
                request_params.setdefault("headers", {}).update(provider_headers)

        # Execute request via deps client
        client = ctx.deps.client
        verify = request_params.pop("verify", None)
        if verify is not None:
            async with httpx.AsyncClient(verify=verify) as ssl_client:
                response = await ssl_client.request(**request_params)
        else:
            response = await client.request(**request_params)

        # Log the response
        logger.debug(
            "API Response: %s %s - Status: %d",
            request_params.get("method", "").upper(),
            request_params.get("url", ""),
            response.status_code,
        )

        # Parse response
        try:
            response.raise_for_status()
        except httpx.HTTPStatusError:
            error_details = response.content.decode("utf-8")
            logger.warning(
                "API call failed for tool %s: Status %d - %s",
                config.name,
                response.status_code,
                error_details,
            )
            return {
                "error": (
                    f"Tool {config.name} execution failed."
                    f" Status Code: {response.status_code}, {error_details}"
                )
            }

        # Route based on Content-Type header
        content_type = response.headers.get("content-type", "")
        mime = content_type.split(";")[0].strip().lower()

        if mime == "application/json" or mime.endswith("+json"):
            try:
                return response.json()
            except ValueError:
                logger.debug("Content-Type indicated JSON but parsing failed: %s", response.text)
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
                logger.debug("API Response (non-JSON): %s", response.text)
                return {"text": response.text}

    return rest_api_call


def create_rest_api_tool(config: RestApiToolConfig) -> Tool:
    """Create a Pydantic AI Tool from a RestApiToolConfig.

    The tool uses Tool.from_schema with takes_ctx=True so that the callable
    receives RunContext[OpenAPIDeps] as its first argument. The JSON schema
    for arguments is derived from the operation parser.

    Args:
        config: Static configuration for the REST API operation.

    Returns:
        A Pydantic AI Tool instance.
    """
    callable_fn = create_rest_api_callable(config)

    return Tool.from_schema(
        function=callable_fn,
        name=config.name,
        description=config.description,
        json_schema=config.operation_parser.get_json_schema(),
        takes_ctx=True,
        sequential=False,
    )
