from __future__ import annotations

import logging
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any

import httpx
import pydantic_monty
from flokoa_common.auth.auth_credential import AuthCredential
from flokoa_common.auth.auth_schemes import AuthScheme
from flokoa_common.utils.openapi.openapi_spec_parser import ParsedOperation
from flokoa_common.utils.openapi.operation_parser import OperationParser
from flokoa_common.utils.openapi.request_builder import prepare_request_params

logger = logging.getLogger("flokoa.codemode.executor")


@dataclass
class ExecutionResult:
    """Result of executing code in the Monty sandbox."""

    output: Any = None
    stdout: str = ""
    error: str | None = None


def _create_api_callable(
    operation: ParsedOperation,
    client: httpx.AsyncClient,
    auth_scheme: AuthScheme | None = None,
    auth_credential: AuthCredential | None = None,
) -> Callable[..., Any]:
    """Create an async callable that executes an API operation via HTTP.

    When Monty code calls the function, this callable intercepts it,
    builds the HTTP request from the ParsedOperation, executes it,
    and returns the parsed response.
    """
    parser = OperationParser.load(operation.operation, operation.parameters, operation.return_value)
    parameters = parser.get_parameters()

    async def api_call(**kwargs: Any) -> Any:
        request_params = prepare_request_params(
            endpoint=operation.endpoint,
            operation=operation.operation,
            default_headers={},
            tool_name=parser.get_function_name(),
            parameters=parameters,
            kwargs=kwargs,
        )

        response = await client.request(**request_params)

        try:
            response.raise_for_status()
        except httpx.HTTPStatusError:
            raw = response.content.decode("utf-8", errors="replace")
            truncated = (raw[:500] + "...") if len(raw) > 500 else raw
            logger.warning(
                "API call failed for %s: Status %d - %s",
                parser.get_function_name(),
                response.status_code,
                truncated,
            )
            return {"error": f"HTTP {response.status_code}", "status_code": response.status_code}

        content_type = response.headers.get("content-type", "")
        mime = content_type.split(";")[0].strip().lower()

        if mime == "application/json" or mime.endswith("+json"):
            try:
                return response.json()
            except ValueError:
                return {"text": response.text}
        elif mime.startswith("text/"):
            return {"text": response.text}
        else:
            try:
                return response.json()
            except ValueError:
                return {"text": response.text}

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
    resource_limits: pydantic_monty.ResourceLimits | None = None

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
                runtime_functions[name] = _create_api_callable(op, client, self.auth_scheme, self.auth_credential)

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
