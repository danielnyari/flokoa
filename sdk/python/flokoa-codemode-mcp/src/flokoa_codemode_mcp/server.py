from __future__ import annotations

import json
import logging
from collections.abc import Callable
from typing import Any, Literal

import yaml
from fastmcp import FastMCP
from flokoa_common.auth.auth_credential import AuthCredential
from flokoa_common.auth.auth_schemes import AuthScheme
from flokoa_common.utils.openapi.openapi_spec_parser import OpenApiSpecParser
from flokoa_common.utils.openapi.operation_parser import OperationParser
from pydantic_monty import ResourceLimits

from .executor import CodeExecutor, ExecutionResult
from .stub_generator import generate_stubs

logger = logging.getLogger("flokoa.codemode.server")


class CodemodeServer:
    """MCP server implementing Code Mode for Flokoa.

    Instead of exposing OpenAPI operations as individual MCP tools, this
    server generates typed Python function stubs from the spec and lets
    the LLM write code that calls them. The code is executed in a secure
    pydantic-monty sandbox, with function calls intercepted and routed
    to actual HTTP API requests.

    Usage::

        server = CodemodeServer(
            name="Petstore",
            openapi_spec=petstore_spec_dict,
        )
        server.run()

    The server exposes:
    - Resource ``codemode://api-stubs``: Python function stubs for context
    - Tool ``execute_code``: Runs code in the Monty sandbox
    """

    def __init__(
        self,
        *,
        name: str = "Flokoa Codemode",
        openapi_spec: dict[str, Any] | None = None,
        openapi_spec_str: str | None = None,
        openapi_spec_str_type: Literal["json", "yaml"] = "json",
        auth_scheme: AuthScheme | None = None,
        auth_credential: AuthCredential | None = None,
        allow_internal: bool = False,
        external_functions: dict[str, Callable[..., Any]] | None = None,
        external_function_stubs: dict[str, str] | None = None,
        resource_limits: ResourceLimits | None = None,
    ):
        """Initialize the Codemode MCP server.

        Args:
            name: Server name shown to MCP clients.
            openapi_spec: OpenAPI spec as a dictionary.
            openapi_spec_str: OpenAPI spec as a JSON or YAML string.
            openapi_spec_str_type: Format of openapi_spec_str.
            auth_scheme: Auth scheme applied to all API operations.
            auth_credential: Auth credential applied to all API operations
                (exchanged per call via flokoa_common.auth.exchangers).
            allow_internal: Skip SSRF protection for API request URLs,
                permitting private/internal addresses (for trusted
                in-cluster APIs).
            external_functions: Additional callables available in the sandbox.
            external_function_stubs: Explicit type stubs for external functions,
                keyed by name. Overrides auto-generated stubs.
            resource_limits: Monty resource limits (max time, memory, etc.).
        """
        self._external_functions = external_functions or {}
        self._auth_scheme = auth_scheme
        self._auth_credential = auth_credential
        self._allow_internal = allow_internal
        self._resource_limits = resource_limits

        # Parse the OpenAPI spec
        if openapi_spec is None and openapi_spec_str is not None:
            openapi_spec = self._load_spec(openapi_spec_str, openapi_spec_str_type)

        operations = []
        if openapi_spec is not None:
            operations = OpenApiSpecParser().parse(openapi_spec)

        # Build operations map keyed by function name
        self._operations: dict[str, Any] = {}
        for op in operations:
            parser = OperationParser.load(op.operation, op.parameters, op.return_value)
            fn_name = parser.get_function_name()
            self._operations[fn_name] = op

        # Generate stubs
        self._stubs = generate_stubs(
            operations,
            external_functions=self._external_functions,
            external_function_stubs=external_function_stubs,
        )

        # Build the executor
        self._executor = CodeExecutor(
            operations=self._operations,
            external_functions=self._external_functions,
            stubs=self._stubs,
            auth_scheme=self._auth_scheme,
            auth_credential=self._auth_credential,
            allow_internal=self._allow_internal,
            resource_limits=self._resource_limits,
        )

        # Create the FastMCP server
        self._mcp = FastMCP(name)
        self._register_resource()
        self._register_tools()

        logger.info(
            "CodemodeServer initialized: %d API operations, %d external functions",
            len(self._operations),
            len(self._external_functions),
        )

    @property
    def mcp(self) -> FastMCP:
        """The underlying FastMCP server instance."""
        return self._mcp

    @property
    def stubs(self) -> str:
        """The generated Python function stubs."""
        return self._stubs

    def run(self) -> None:
        """Start the MCP server."""
        self._mcp.run()

    def _register_resource(self) -> None:
        @self._mcp.resource("codemode://api-stubs")
        def api_stubs() -> str:
            """Python function stubs for all available API operations and external functions.

            Include this in your context to understand the available functions,
            their parameters, and return types before writing code.
            """
            return self._stubs

    def _register_tools(self) -> None:
        executor = self._executor

        @self._mcp.tool
        async def execute_code(code: str) -> str:
            """Execute Python code in a secure sandbox with access to API functions.

            Write Python code that calls the available API functions (see the
            codemode://api-stubs resource for available functions and their
            signatures). The code runs in a sandboxed interpreter — function
            calls are intercepted and routed to actual HTTP API requests.

            Use print() to output intermediate results. The final expression
            value is returned as the output.

            Args:
                code: Python source code to execute.

            Returns:
                JSON string with output, stdout, and any errors.
            """
            result: ExecutionResult = await executor.execute(code)
            return json.dumps(
                {
                    "output": _serialize_output(result.output),
                    "stdout": result.stdout,
                    "error": result.error,
                },
                indent=2,
                default=str,
            )

    @staticmethod
    def _load_spec(spec_str: str, spec_type: Literal["json", "yaml"]) -> dict[str, Any]:
        if spec_type == "json":
            return json.loads(spec_str)
        elif spec_type == "yaml":
            return yaml.safe_load(spec_str)
        else:
            raise ValueError(f"Unsupported spec type: {spec_type}")


def _serialize_output(value: Any) -> Any:
    """Best-effort serialization of Monty output to JSON-safe types."""
    if value is None or isinstance(value, (str, int, float, bool)):
        return value
    if isinstance(value, (list, tuple)):
        return [_serialize_output(v) for v in value]
    if isinstance(value, dict):
        return {str(k): _serialize_output(v) for k, v in value.items()}
    return str(value)
