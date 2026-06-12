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

from typing import Any
from urllib.parse import quote

from fastapi.openapi.models import Operation

from .common import ApiParameter
from .openapi_spec_parser import OperationEndpoint


def _route_kwargs_to_locations(
    parameters: list[ApiParameter],
    kwargs: dict[str, Any],
) -> tuple[dict[str, Any], dict[str, Any], dict[str, Any], dict[str, Any]]:
    """Route keyword arguments to their HTTP parameter locations."""
    path_params: dict[str, Any] = {}
    query_params: dict[str, Any] = {}
    header_params: dict[str, Any] = {}
    cookie_params: dict[str, Any] = {}

    params_map: dict[str, ApiParameter] = {p.py_name: p for p in parameters if p.py_name}

    for param_k, v in kwargs.items():
        param_obj = params_map.get(param_k)
        if not param_obj:
            continue

        original_k = param_obj.original_name
        location = param_obj.param_location

        if location == "path":
            path_params[original_k] = v
        elif location == "query":
            if v:
                query_params[original_k] = v
        elif location == "header":
            # Strip CR/LF so model-provided values cannot smuggle extra
            # headers (header injection); httpx/h11 would reject them at the
            # wire level, but the raw value would still echo into error logs.
            if isinstance(v, str):
                v = v.replace("\r", "").replace("\n", "")
            header_params[original_k] = v
        elif location == "cookie":
            cookie_params[original_k] = v

    return path_params, query_params, header_params, cookie_params


def _build_request_body(
    operation: Operation,
    parameters: list[ApiParameter],
    kwargs: dict[str, Any],
    header_params: dict[str, Any],
) -> dict[str, Any]:
    """Build body kwargs from the operation's requestBody."""
    body_kwargs: dict[str, Any] = {}
    request_body = operation.requestBody
    if not request_body or not hasattr(request_body, "content"):
        return body_kwargs

    for mime_type, media_type_object in request_body.content.items():
        schema = media_type_object.schema_
        body_data = _extract_body_data(schema, parameters, kwargs)

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

    return body_kwargs


def _extract_body_data(schema: Any, parameters: list[ApiParameter], kwargs: dict[str, Any]) -> Any:
    """Extract body data from kwargs based on schema type."""
    if schema.type == "object":
        return {
            param.original_name: kwargs[param.py_name]
            for param in parameters
            if param.param_location == "body" and param.py_name in kwargs
        }

    if schema.type == "array":
        for param in parameters:
            if param.param_location == "body" and param.py_name == "array":
                return kwargs.get("array")
        return None

    for param in parameters:
        if param.param_location == "body" and not param.original_name:
            return kwargs.get(param.py_name) if param.py_name in kwargs else None
    return None


def prepare_request_params(
    endpoint: OperationEndpoint,
    operation: Operation,
    default_headers: dict[str, str],
    tool_name: str,
    parameters: list[ApiParameter],
    kwargs: dict[str, Any],
    auth_additional_headers: dict[str, str] | None = None,
) -> dict[str, Any]:
    """Build httpx request parameters from operation config and call arguments.

    This is a pure function that maps OpenAPI parameters to their HTTP
    locations (path, query, header, cookie) and constructs the request body
    from the operation's requestBody.

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

    path_params, query_params, header_params, cookie_params = _route_kwargs_to_locations(parameters, kwargs)

    header_params["User-Agent"] = f"flokoa (tool: {tool_name})"
    if auth_additional_headers:
        header_params.update(auth_additional_headers)

    # Construct URL. Path param values are percent-encoded (no characters
    # left "safe") so model-provided values like `../admin` or embedded `/`
    # cannot traverse outside the templated path segment.
    base_url = endpoint.base_url or ""
    base_url = base_url[:-1] if base_url.endswith("/") else base_url
    url = f"{base_url}{endpoint.path.format(**{k: quote(str(v), safe='') for k, v in path_params.items()})}"

    # Construct body
    body_kwargs = _build_request_body(operation, parameters, kwargs, header_params)

    filtered_query_params: dict[str, Any] = {k: v for k, v in query_params.items() if v is not None}

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
