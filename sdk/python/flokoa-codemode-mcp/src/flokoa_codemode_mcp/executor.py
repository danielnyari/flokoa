from __future__ import annotations

import asyncio
import logging
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any

import httpx
import pydantic_monty
from flokoa_common.auth.auth_credential import AuthCredential
from flokoa_common.auth.auth_schemes import AuthScheme
from flokoa_common.auth.exchangers import AutoAuthCredentialExchanger
from flokoa_common.auth.helpers import credential_to_param
from flokoa_common.utils.openapi.common import ApiParameter
from flokoa_common.utils.openapi.openapi_spec_parser import ParsedOperation
from flokoa_common.utils.openapi.operation_parser import OperationParser
from flokoa_common.utils.openapi.request_builder import prepare_request_params
from flokoa_common.utils.url_validation import SSRFError, validate_url

logger = logging.getLogger("flokoa.codemode.executor")


@dataclass
class ExecutionResult:
    """Result of executing code in the Monty sandbox."""

    output: Any = None
    stdout: str = ""
    error: str | None = None


async def _apply_auth(
    auth_scheme: AuthScheme | None,
    auth_credential: AuthCredential | None,
    credential_exchanger: AutoAuthCredentialExchanger,
    parameters: list[ApiParameter],
    kwargs: dict[str, Any],
    fn_name: str,
) -> tuple[list[ApiParameter], dict[str, Any], dict[str, str] | None]:
    """Exchange the configured credential and attach it to the call parameters.

    Returns the (possibly extended) parameter list, the call kwargs with the
    credential value merged in, and any extra headers carried by the
    exchanged credential.
    """
    call_kwargs = dict(kwargs)
    if not (auth_scheme and auth_credential):
        return parameters, call_kwargs, None

    # Exchange auth credentials (e.g. OAuth2 token refresh, service account)
    credential = await credential_exchanger.exchange_credential(auth_scheme, auth_credential)

    auth_additional_headers: dict[str, str] | None = None
    if credential and credential.http and credential.http.additional_headers:
        auth_additional_headers = dict(credential.http.additional_headers)

    call_parameters = parameters
    if credential:
        auth_param, auth_args = credential_to_param(auth_scheme, credential)
        if auth_param and auth_args:
            call_parameters = [auth_param, *parameters]
            call_kwargs.update(auth_args)

    if call_parameters is parameters and not auth_additional_headers:
        logger.warning(
            "Auth was configured for %s but no usable credential could be derived; sending unauthenticated request.",
            fn_name,
        )

    return call_parameters, call_kwargs, auth_additional_headers


def _parse_response(response: httpx.Response) -> Any:
    """Parse a successful HTTP response by content type."""
    content_type = response.headers.get("content-type", "")
    mime = content_type.split(";")[0].strip().lower()

    if mime.startswith("text/"):
        return {"text": response.text}
    try:
        return response.json()
    except ValueError:
        return {"text": response.text}


def _create_api_callable(
    operation: ParsedOperation,
    client: httpx.AsyncClient,
    auth_scheme: AuthScheme | None = None,
    auth_credential: AuthCredential | None = None,
    allow_internal: bool = False,
    credential_exchanger: AutoAuthCredentialExchanger | None = None,
) -> Callable[..., Any]:
    """Create an async callable that executes an API operation via HTTP.

    When Monty code calls the function, this callable intercepts it,
    builds the HTTP request from the ParsedOperation, executes it,
    and returns the parsed response.

    Pass a shared ``credential_exchanger`` (as :class:`CodeExecutor` does) so
    token caches survive across operations; when omitted, a fresh exchanger
    is created for this callable only.
    """
    parser = OperationParser.load(operation.operation, operation.parameters, operation.return_value)
    parameters = parser.get_parameters()
    if credential_exchanger is None:
        credential_exchanger = AutoAuthCredentialExchanger()

    async def api_call(**kwargs: Any) -> Any:
        fn_name = parser.get_function_name()

        try:
            call_parameters, call_kwargs, auth_additional_headers = await _apply_auth(
                auth_scheme, auth_credential, credential_exchanger, parameters, kwargs, fn_name
            )
        except Exception as e:
            # Sanitized envelope: never propagate raw exchanger exceptions
            # into the Monty sandbox — log the exception type server-side
            # only, since exchanger errors can carry credential material.
            logger.warning("Credential exchange failed for %s: %s", fn_name, type(e).__name__)
            return {"error": f"{fn_name} credential exchange failed"}

        request_params = prepare_request_params(
            endpoint=operation.endpoint,
            operation=operation.operation,
            default_headers={},
            tool_name=fn_name,
            parameters=call_parameters,
            kwargs=call_kwargs,
            auth_additional_headers=auth_additional_headers,
        )

        # Validate the constructed URL against SSRF attacks. Runs in a worker
        # thread: validation does blocking DNS resolution, which must not
        # stall the event loop on every sandboxed API call.
        # Skip for relative URLs (no scheme) — these are resolved by the
        # httpx client against its base_url and don't target hosts directly.
        constructed_url = request_params["url"]
        if constructed_url.startswith(("http://", "https://")):
            try:
                await asyncio.to_thread(validate_url, constructed_url, allow_internal=allow_internal)
            except SSRFError as e:
                logger.warning("SSRF validation failed for %s: %s", fn_name, e)
                return {"error": f"{fn_name} request blocked: URL failed security validation"}

        response = await client.request(**request_params)

        try:
            response.raise_for_status()
        except httpx.HTTPStatusError:
            raw = response.content.decode("utf-8", errors="replace")
            truncated = (raw[:500] + "...") if len(raw) > 500 else raw
            logger.warning(
                "API call failed for %s: Status %d - %s",
                fn_name,
                response.status_code,
                truncated,
            )
            return {"error": f"HTTP {response.status_code}", "status_code": response.status_code}

        return _parse_response(response)

    return api_call


@dataclass
class CodeExecutor:
    """Executes LLM-generated Python code in a Monty sandbox.

    Bridges external function calls from the sandbox to actual HTTP API
    requests (for OpenAPI operations) or user-provided callables.
    """

    operations: dict[str, ParsedOperation] = field(default_factory=dict)
    external_functions: dict[str, Callable[..., Any]] = field(default_factory=dict)
    stubs: str = ""
    auth_scheme: AuthScheme | None = None
    auth_credential: AuthCredential | None = None
    allow_internal: bool = False
    resource_limits: pydantic_monty.ResourceLimits | None = None
    # One exchanger per executor (not per operation) so in-memory token
    # caches — e.g. the service account exchanger's — are shared across all
    # operations and executions.
    _credential_exchanger: AutoAuthCredentialExchanger = field(
        default_factory=AutoAuthCredentialExchanger, init=False, repr=False
    )

    async def execute(self, code: str) -> ExecutionResult:
        """Execute Python code in the Monty sandbox.

        Args:
            code: Python source code to execute. The code can call any
                function defined in the stubs (API operations and external
                functions).

        Returns:
            ExecutionResult with output, captured stdout, and any error.
        """
        all_external_names = list(self.operations.keys()) + list(self.external_functions.keys())

        stdout_lines: list[str] = []

        def print_callback(_stream: str, text: str) -> None:
            stdout_lines.append(text)

        try:
            monty = pydantic_monty.Monty(
                code,
                external_functions=all_external_names,
                script_name="codemode.py",
                type_check=True,
                type_check_stubs=self.stubs,
            )
        except pydantic_monty.MontySyntaxError as e:
            return ExecutionResult(error=f"Syntax error: {e.display()}")
        except pydantic_monty.MontyTypingError as e:
            return ExecutionResult(error=f"Type error: {e.display('concise')}")

        # Build the combined external functions dict for Monty execution
        async with httpx.AsyncClient() as client:
            runtime_functions: dict[str, Callable[..., Any]] = {}

            # Register API operation callables
            for name, op in self.operations.items():
                runtime_functions[name] = _create_api_callable(
                    op,
                    client,
                    self.auth_scheme,
                    self.auth_credential,
                    self.allow_internal,
                    credential_exchanger=self._credential_exchanger,
                )

            # Register user-provided external functions
            runtime_functions.update(self.external_functions)

            try:
                output = await pydantic_monty.run_monty_async(
                    monty,
                    external_functions=runtime_functions,
                    limits=self.resource_limits,
                    print_callback=print_callback,
                )
            except pydantic_monty.MontyRuntimeError as e:
                return ExecutionResult(
                    stdout="\n".join(stdout_lines),
                    error=f"Runtime error: {e.display()}",
                )

        return ExecutionResult(
            output=output,
            stdout="\n".join(stdout_lines),
        )
