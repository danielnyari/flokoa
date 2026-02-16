"""Tests for the Pydantic AI OpenAPI tool integration.

Covers:
- OpenAPIToolset: parsing, filtering, prefixing, spec loading
- RestApiToolConfig: construction from ParsedOperation
- _prepare_request_params: URL building, param routing, body construction
- create_rest_api_tool / create_rest_api_callable: Tool creation, HTTP execution
- Auth credential exchange and injection
- OpenAPIDeps: header_provider, SSL verify
"""

import json
from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest
import yaml
from pydantic_ai import FunctionToolset, RunContext, Tool

from flokoa.tools.openapi import (
    OpenAPIDeps,
    OpenAPIToolset,
    RestApiToolConfig,
    create_rest_api_callable,
    create_rest_api_tool,
)
from flokoa.tools.openapi.auth.auth_helpers import token_to_scheme_credential
from flokoa.tools.openapi.common import ApiParameter
from flokoa.tools.openapi.openapi_spec_parser import OpenApiSpecParser, OperationEndpoint
from flokoa.tools.openapi.operation_parser import OperationParser
from flokoa.tools.openapi.rest_api_tool import _prepare_request_params

pytestmark = pytest.mark.anyio


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_run_context(deps: OpenAPIDeps) -> RunContext[OpenAPIDeps]:
    """Build a minimal RunContext for testing tool callables."""
    ctx = MagicMock(spec=RunContext)
    ctx.deps = deps
    return ctx


def _make_media_type(schema):
    """Build a MediaType with the correct alias-aware construction."""
    from fastapi.openapi.models import MediaType

    return MediaType.model_validate({"schema": schema.model_dump()})


# ---------------------------------------------------------------------------
# OpenAPIToolset — parsing
# ---------------------------------------------------------------------------


class TestOpenAPIToolsetParsing:
    """Tests that the toolset correctly parses the Petstore spec into tools."""

    def test_parse_creates_tools_for_all_operations(self, openapi_spec):
        toolset = OpenAPIToolset(spec_dict=openapi_spec)
        tools = toolset.get_tools()
        # Count operations in the spec
        expected_count = sum(
            len(methods) for methods in openapi_spec["paths"].values()
        )
        assert len(tools) == expected_count

    def test_tools_are_pydantic_ai_tool_instances(self, openapi_spec):
        toolset = OpenAPIToolset(spec_dict=openapi_spec)
        for tool in toolset.get_tools():
            assert isinstance(tool, Tool)

    def test_tool_names_are_snake_case(self, openapi_spec):
        toolset = OpenAPIToolset(spec_dict=openapi_spec)
        tools = toolset.get_tools()
        names = [t.name for t in tools]
        for name in names:
            assert name == name.lower(), f"Tool name {name!r} is not lowercase"
            assert " " not in name, f"Tool name {name!r} contains spaces"

    def test_known_operation_ids_present(self, openapi_spec):
        toolset = OpenAPIToolset(spec_dict=openapi_spec)
        names = {t.name for t in toolset.get_tools()}
        expected = {
            "update_pet",
            "add_pet",
            "find_pets_by_status",
            "find_pets_by_tags",
            "get_pet_by_id",
            "update_pet_with_form",
            "delete_pet",
            "upload_file",
            "get_inventory",
            "place_order",
            "get_order_by_id",
            "delete_order",
            "create_user",
            "create_users_with_list_input",
            "login_user",
            "logout_user",
            "get_user_by_name",
            "update_user",
            "delete_user",
        }
        assert expected.issubset(names), f"Missing tools: {expected - names}"

    def test_tool_descriptions_are_populated(self, openapi_spec):
        toolset = OpenAPIToolset(spec_dict=openapi_spec)
        tools = toolset.get_tools()
        descriptions = [t.description for t in tools]
        non_empty = [d for d in descriptions if d]
        assert len(non_empty) > 0


# ---------------------------------------------------------------------------
# OpenAPIToolset — spec loading (JSON / YAML)
# ---------------------------------------------------------------------------


class TestOpenAPIToolsetSpecLoading:
    def test_load_from_json_string(self, openapi_spec):
        spec_str = json.dumps(openapi_spec)
        toolset = OpenAPIToolset(spec_str=spec_str, spec_str_type="json")
        assert len(toolset.get_tools()) > 0

    def test_load_from_yaml_string(self, openapi_spec):
        spec_str = yaml.dump(openapi_spec)
        toolset = OpenAPIToolset(spec_str=spec_str, spec_str_type="yaml")
        assert len(toolset.get_tools()) > 0

    def test_spec_dict_takes_precedence_over_spec_str(self, openapi_spec):
        toolset = OpenAPIToolset(
            spec_dict=openapi_spec,
            spec_str="this is not valid json",
            spec_str_type="json",
        )
        assert len(toolset.get_tools()) > 0

    def test_invalid_spec_type_raises(self):
        with pytest.raises(ValueError, match="Unsupported spec type"):
            OpenAPIToolset(spec_str="{}", spec_str_type="toml")  # type: ignore[arg-type]

    def test_invalid_json_string_raises(self):
        with pytest.raises(json.JSONDecodeError):
            OpenAPIToolset(spec_str="not json", spec_str_type="json")


# ---------------------------------------------------------------------------
# OpenAPIToolset — filtering
# ---------------------------------------------------------------------------


class TestOpenAPIToolsetFiltering:
    def test_tool_filter_limits_tools(self, openapi_spec):
        toolset = OpenAPIToolset(
            spec_dict=openapi_spec,
            tool_filter=["get_pet_by_id", "delete_pet"],
        )
        tools = toolset.get_tools()
        names = {t.name for t in tools}
        assert names == {"get_pet_by_id", "delete_pet"}

    def test_tool_filter_empty_list_returns_no_tools(self, openapi_spec):
        toolset = OpenAPIToolset(spec_dict=openapi_spec, tool_filter=[])
        assert toolset.get_tools() == []

    def test_tool_filter_none_returns_all(self, openapi_spec):
        toolset_all = OpenAPIToolset(spec_dict=openapi_spec, tool_filter=None)
        toolset_unfiltered = OpenAPIToolset(spec_dict=openapi_spec)
        assert len(toolset_all.get_tools()) == len(toolset_unfiltered.get_tools())

    def test_tool_filter_nonexistent_name(self, openapi_spec):
        toolset = OpenAPIToolset(
            spec_dict=openapi_spec,
            tool_filter=["this_tool_does_not_exist"],
        )
        assert toolset.get_tools() == []


# ---------------------------------------------------------------------------
# OpenAPIToolset — get_tool
# ---------------------------------------------------------------------------


class TestOpenAPIToolsetGetTool:
    def test_get_tool_by_name(self, openapi_spec):
        toolset = OpenAPIToolset(spec_dict=openapi_spec)
        tool = toolset.get_tool("get_pet_by_id")
        assert tool is not None
        assert tool.name == "get_pet_by_id"

    def test_get_tool_not_found_returns_none(self, openapi_spec):
        toolset = OpenAPIToolset(spec_dict=openapi_spec)
        assert toolset.get_tool("nonexistent") is None


# ---------------------------------------------------------------------------
# OpenAPIToolset — to_function_toolset / prefixing
# ---------------------------------------------------------------------------


class TestOpenAPIToolsetFunctionToolset:
    def test_to_function_toolset_returns_function_toolset(self, openapi_spec):
        toolset = OpenAPIToolset(spec_dict=openapi_spec)
        fs = toolset.to_function_toolset()
        assert isinstance(fs, FunctionToolset)

    def test_prefix_is_applied(self, openapi_spec):
        toolset = OpenAPIToolset(
            spec_dict=openapi_spec,
            tool_name_prefix="petstore",
            tool_filter=["get_pet_by_id"],
        )
        fs = toolset.to_function_toolset()
        # The prefixed toolset wraps the FunctionToolset
        assert fs is not None
        # It should NOT be a plain FunctionToolset since prefix was applied
        assert not isinstance(fs, FunctionToolset)

    def test_no_prefix_returns_plain_function_toolset(self, openapi_spec):
        toolset = OpenAPIToolset(spec_dict=openapi_spec)
        fs = toolset.to_function_toolset()
        assert isinstance(fs, FunctionToolset)


# ---------------------------------------------------------------------------
# OpenAPIToolset — auth propagation
# ---------------------------------------------------------------------------


class TestOpenAPIToolsetAuth:
    def test_global_auth_propagates_to_configs(self, openapi_spec):
        auth_scheme, auth_credential = token_to_scheme_credential(
            "apikey", "header", "X-API-Key", "test-key-123"
        )
        toolset = OpenAPIToolset(
            spec_dict=openapi_spec,
            auth_scheme=auth_scheme,
            auth_credential=auth_credential,
        )
        for config in toolset._configs:
            assert config.auth_scheme is not None
            assert config.auth_credential is not None

    def test_ssl_verify_propagates_to_configs(self, openapi_spec):
        toolset = OpenAPIToolset(spec_dict=openapi_spec, ssl_verify=False)
        for config in toolset._configs:
            assert config.ssl_verify is False


# ---------------------------------------------------------------------------
# RestApiToolConfig — from_parsed_operation
# ---------------------------------------------------------------------------


class TestRestApiToolConfig:
    def test_from_parsed_operation(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        assert len(operations) > 0

        config = RestApiToolConfig.from_parsed_operation(operations[0])
        assert config.name
        assert config.description is not None
        assert config.endpoint is not None
        assert config.operation is not None
        assert config.operation_parser is not None

    def test_name_truncated_to_60_chars(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        for op in operations:
            config = RestApiToolConfig.from_parsed_operation(op)
            assert len(config.name) <= 60

    def test_ssl_verify_passed_through(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        config = RestApiToolConfig.from_parsed_operation(
            operations[0], ssl_verify="/path/to/ca.pem"
        )
        assert config.ssl_verify == "/path/to/ca.pem"


# ---------------------------------------------------------------------------
# _prepare_request_params
# ---------------------------------------------------------------------------


class TestPrepareRequestParams:
    def _make_endpoint(self, base_url="https://api.example.com", path="/pets/{petId}", method="GET"):
        return OperationEndpoint(base_url=base_url, path=path, method=method)

    def _make_param(self, name, location, py_name=None, required=False):
        from fastapi.openapi.models import Schema

        return ApiParameter(
            original_name=name,
            param_location=location,
            param_schema=Schema(type="string"),
            py_name=py_name or name,
            required=required,
        )

    def test_path_params_substituted(self):
        from fastapi.openapi.models import Operation

        endpoint = self._make_endpoint()
        param = self._make_param("petId", "path")
        result = _prepare_request_params(
            endpoint=endpoint,
            operation=Operation(),
            default_headers={},
            tool_name="get_pet",
            parameters=[param],
            kwargs={"petId": 42},
        )
        assert result["url"] == "https://api.example.com/pets/42"

    def test_query_params_included(self):
        from fastapi.openapi.models import Operation

        endpoint = self._make_endpoint(path="/pets", method="GET")
        param = self._make_param("status", "query")
        result = _prepare_request_params(
            endpoint=endpoint,
            operation=Operation(),
            default_headers={},
            tool_name="find_pets",
            parameters=[param],
            kwargs={"status": "available"},
        )
        assert result["params"] == {"status": "available"}

    def test_header_params_included(self):
        from fastapi.openapi.models import Operation

        endpoint = self._make_endpoint(path="/pets", method="GET")
        param = self._make_param("X-Custom", "header", py_name="x_custom")
        result = _prepare_request_params(
            endpoint=endpoint,
            operation=Operation(),
            default_headers={},
            tool_name="test",
            parameters=[param],
            kwargs={"x_custom": "my-value"},
        )
        assert result["headers"]["X-Custom"] == "my-value"

    def test_cookie_params_included(self):
        from fastapi.openapi.models import Operation

        endpoint = self._make_endpoint(path="/pets", method="GET")
        param = self._make_param("session", "cookie")
        result = _prepare_request_params(
            endpoint=endpoint,
            operation=Operation(),
            default_headers={},
            tool_name="test",
            parameters=[param],
            kwargs={"session": "abc123"},
        )
        assert result["cookies"] == {"session": "abc123"}

    def test_user_agent_set(self):
        from fastapi.openapi.models import Operation

        endpoint = self._make_endpoint(path="/test", method="GET")
        result = _prepare_request_params(
            endpoint=endpoint,
            operation=Operation(),
            default_headers={},
            tool_name="my_tool",
            parameters=[],
            kwargs={},
        )
        assert "flokoa" in result["headers"]["User-Agent"]
        assert "my_tool" in result["headers"]["User-Agent"]

    def test_default_headers_applied(self):
        from fastapi.openapi.models import Operation

        endpoint = self._make_endpoint(path="/test", method="GET")
        result = _prepare_request_params(
            endpoint=endpoint,
            operation=Operation(),
            default_headers={"X-Default": "yes"},
            tool_name="test",
            parameters=[],
            kwargs={},
        )
        assert result["headers"]["X-Default"] == "yes"

    def test_default_headers_dont_overwrite_explicit(self):
        from fastapi.openapi.models import Operation

        endpoint = self._make_endpoint(path="/test", method="GET")
        param = self._make_param("X-Default", "header", py_name="x_default")
        result = _prepare_request_params(
            endpoint=endpoint,
            operation=Operation(),
            default_headers={"X-Default": "default-val"},
            tool_name="test",
            parameters=[param],
            kwargs={"x_default": "explicit-val"},
        )
        assert result["headers"]["X-Default"] == "explicit-val"

    def test_auth_additional_headers_applied(self):
        from fastapi.openapi.models import Operation

        endpoint = self._make_endpoint(path="/test", method="GET")
        result = _prepare_request_params(
            endpoint=endpoint,
            operation=Operation(),
            default_headers={},
            tool_name="test",
            parameters=[],
            kwargs={},
            auth_additional_headers={"Authorization": "Bearer tok"},
        )
        assert result["headers"]["Authorization"] == "Bearer tok"

    def test_method_lowercased(self):
        from fastapi.openapi.models import Operation

        endpoint = self._make_endpoint(path="/test", method="POST")
        result = _prepare_request_params(
            endpoint=endpoint,
            operation=Operation(),
            default_headers={},
            tool_name="test",
            parameters=[],
            kwargs={},
        )
        assert result["method"] == "post"

    def test_empty_method_raises(self):
        from fastapi.openapi.models import Operation

        endpoint = OperationEndpoint(base_url="https://example.com", path="/test", method="")
        with pytest.raises(ValueError, match="Operation method not found"):
            _prepare_request_params(
                endpoint=endpoint,
                operation=Operation(),
                default_headers={},
                tool_name="test",
                parameters=[],
                kwargs={},
            )

    def test_trailing_slash_stripped_from_base_url(self):
        from fastapi.openapi.models import Operation

        endpoint = self._make_endpoint(
            base_url="https://api.example.com/",
            path="/pets",
            method="GET",
        )
        result = _prepare_request_params(
            endpoint=endpoint,
            operation=Operation(),
            default_headers={},
            tool_name="test",
            parameters=[],
            kwargs={},
        )
        assert result["url"] == "https://api.example.com/pets"

    def test_unknown_kwargs_ignored(self):
        from fastapi.openapi.models import Operation

        endpoint = self._make_endpoint(path="/test", method="GET")
        result = _prepare_request_params(
            endpoint=endpoint,
            operation=Operation(),
            default_headers={},
            tool_name="test",
            parameters=[],
            kwargs={"unknown_param": "value"},
        )
        assert result["params"] == {}

    def test_none_query_params_filtered(self):
        from fastapi.openapi.models import Operation

        endpoint = self._make_endpoint(path="/test", method="GET")
        param = self._make_param("q", "query")
        result = _prepare_request_params(
            endpoint=endpoint,
            operation=Operation(),
            default_headers={},
            tool_name="test",
            parameters=[param],
            kwargs={"q": None},
        )
        assert result["params"] == {}


# ---------------------------------------------------------------------------
# _prepare_request_params — request body
# ---------------------------------------------------------------------------


class TestPrepareRequestParamsBody:
    def test_json_body(self):
        from fastapi.openapi.models import Operation, RequestBody, Schema

        body_schema = Schema(
            type="object",
            properties={
                "name": Schema(type="string"),
                "status": Schema(type="string"),
            },
        )
        operation = Operation(
            requestBody=RequestBody(
                content={"application/json": _make_media_type(body_schema)}
            )
        )
        endpoint = OperationEndpoint(base_url="https://api.example.com", path="/pets", method="POST")
        name_param = ApiParameter(
            original_name="name",
            param_location="body",
            param_schema=Schema(type="string"),
            py_name="name",
        )
        status_param = ApiParameter(
            original_name="status",
            param_location="body",
            param_schema=Schema(type="string"),
            py_name="status",
        )
        result = _prepare_request_params(
            endpoint=endpoint,
            operation=operation,
            default_headers={},
            tool_name="add_pet",
            parameters=[name_param, status_param],
            kwargs={"name": "Buddy", "status": "available"},
        )
        assert result["json"] == {"name": "Buddy", "status": "available"}
        assert result["headers"]["Content-Type"] == "application/json"

    def test_form_urlencoded_body(self):
        from fastapi.openapi.models import Operation, RequestBody, Schema

        body_schema = Schema(
            type="object",
            properties={"field": Schema(type="string")},
        )
        operation = Operation(
            requestBody=RequestBody(
                content={"application/x-www-form-urlencoded": _make_media_type(body_schema)}
            )
        )
        endpoint = OperationEndpoint(base_url="https://api.example.com", path="/form", method="POST")
        param = ApiParameter(
            original_name="field",
            param_location="body",
            param_schema=Schema(type="string"),
            py_name="field",
        )
        result = _prepare_request_params(
            endpoint=endpoint,
            operation=operation,
            default_headers={},
            tool_name="submit_form",
            parameters=[param],
            kwargs={"field": "value"},
        )
        assert result["data"] == {"field": "value"}


# ---------------------------------------------------------------------------
# create_rest_api_tool
# ---------------------------------------------------------------------------


class TestCreateRestApiTool:
    def test_returns_tool_instance(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        config = RestApiToolConfig.from_parsed_operation(operations[0])
        tool = create_rest_api_tool(config)
        assert isinstance(tool, Tool)
        assert tool.name == config.name
        assert tool.description == config.description

    def test_tool_has_correct_name(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        get_pet_ops = [o for o in operations if o.name == "get_pet_by_id"]
        assert len(get_pet_ops) == 1
        config = RestApiToolConfig.from_parsed_operation(get_pet_ops[0])
        tool = create_rest_api_tool(config)
        assert tool.name == "get_pet_by_id"


# ---------------------------------------------------------------------------
# create_rest_api_callable — HTTP execution
# ---------------------------------------------------------------------------


class TestCreateRestApiCallable:
    @pytest.fixture
    def get_pet_config(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        get_pet_ops = [o for o in operations if o.name == "get_pet_by_id"]
        return RestApiToolConfig.from_parsed_operation(get_pet_ops[0])

    @pytest.fixture
    def find_pets_config(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        ops = [o for o in operations if o.name == "find_pets_by_status"]
        return RestApiToolConfig.from_parsed_operation(ops[0])

    async def test_successful_json_response(self, get_pet_config):
        mock_response = httpx.Response(
            200,
            json={"id": 1, "name": "Buddy", "status": "available"},
            request=httpx.Request("GET", "https://example.com"),
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(get_pet_config)
        result = await callable_fn(ctx, pet_id=1)

        assert result == {"id": 1, "name": "Buddy", "status": "available"}
        mock_client.request.assert_called_once()

    async def test_http_error_returns_error_dict(self, get_pet_config):
        mock_response = httpx.Response(
            404,
            content=b"Pet not found",
            request=httpx.Request("GET", "https://example.com"),
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(get_pet_config)
        result = await callable_fn(ctx, pet_id=999)

        assert "error" in result
        assert "404" in result["error"]
        assert "Pet not found" in result["error"]

    async def test_non_json_response_returns_text(self, get_pet_config):
        mock_response = httpx.Response(
            200,
            text="plain text response",
            request=httpx.Request("GET", "https://example.com"),
            headers={"content-type": "text/plain"},
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(get_pet_config)
        result = await callable_fn(ctx, pet_id=1)

        assert result == {"text": "plain text response"}

    async def test_header_provider_called(self, get_pet_config):
        mock_response = httpx.Response(
            200,
            json={"id": 1},
            request=httpx.Request("GET", "https://example.com"),
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        provider = MagicMock(return_value={"X-Trace-Id": "abc-123"})
        deps = OpenAPIDeps(client=mock_client, header_provider=provider)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(get_pet_config)
        await callable_fn(ctx, pet_id=1)

        provider.assert_called_once()
        call_kwargs = mock_client.request.call_args
        headers = call_kwargs.kwargs.get("headers") or call_kwargs[1].get("headers", {})
        assert headers.get("X-Trace-Id") == "abc-123"

    async def test_ssl_verify_uses_separate_client(self, get_pet_config):
        get_pet_config.ssl_verify = False

        mock_response = httpx.Response(
            200,
            json={"id": 1},
            request=httpx.Request("GET", "https://example.com"),
        )

        # Create deps_client before patching httpx.AsyncClient
        deps_client = AsyncMock()
        deps_client.request = AsyncMock()

        with patch("flokoa.tools.openapi.rest_api_tool.httpx.AsyncClient") as MockClientClass:
            mock_ssl_client = AsyncMock()
            mock_ssl_client.request = AsyncMock(return_value=mock_response)
            mock_ssl_client.__aenter__ = AsyncMock(return_value=mock_ssl_client)
            mock_ssl_client.__aexit__ = AsyncMock(return_value=False)
            MockClientClass.return_value = mock_ssl_client

            deps = OpenAPIDeps(client=deps_client)
            ctx = _make_run_context(deps)

            callable_fn = create_rest_api_callable(get_pet_config)
            await callable_fn(ctx, pet_id=1)

            MockClientClass.assert_called_once_with(verify=False)
            mock_ssl_client.request.assert_called_once()
            deps_client.request.assert_not_called()

    async def test_default_headers_passed(self, get_pet_config):
        get_pet_config.default_headers = {"X-Default": "default-value"}

        mock_response = httpx.Response(
            200,
            json={"id": 1},
            request=httpx.Request("GET", "https://example.com"),
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(get_pet_config)
        await callable_fn(ctx, pet_id=1)

        call_kwargs = mock_client.request.call_args
        headers = call_kwargs.kwargs.get("headers") or call_kwargs[1].get("headers", {})
        assert headers.get("X-Default") == "default-value"

    async def test_query_params_in_request(self, find_pets_config):
        mock_response = httpx.Response(
            200,
            json=[{"id": 1, "name": "Buddy"}],
            request=httpx.Request("GET", "https://example.com"),
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(find_pets_config)
        await callable_fn(ctx, status="available")

        call_kwargs = mock_client.request.call_args
        params = call_kwargs.kwargs.get("params") or call_kwargs[1].get("params", {})
        assert params.get("status") == "available"


# ---------------------------------------------------------------------------
# Auth credential injection
# ---------------------------------------------------------------------------


class TestAuthCredentialInjection:
    async def test_api_key_auth_injected_as_header(self, openapi_spec):
        # get_inventory uses the api_key security scheme from the spec
        # (apiKey in header named "api_key"), so we provide a matching credential
        auth_scheme, auth_credential = token_to_scheme_credential(
            "apikey", "header", "api_key", "secret-key"
        )

        toolset = OpenAPIToolset(
            spec_dict=openapi_spec,
            auth_scheme=auth_scheme,
            auth_credential=auth_credential,
            tool_filter=["get_inventory"],
        )
        # Use _filtered_configs to get only the filtered tool
        filtered = toolset._filtered_configs()
        assert len(filtered) == 1
        config = filtered[0]
        assert config.name == "get_inventory"

        mock_response = httpx.Response(
            200,
            json={"available": 10},
            request=httpx.Request("GET", "https://example.com"),
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(config)
        await callable_fn(ctx)

        call_kwargs = mock_client.request.call_args
        headers = call_kwargs.kwargs.get("headers") or call_kwargs[1].get("headers", {})
        assert headers.get("api_key") == "secret-key"

    async def test_bearer_token_auth_injected(self, openapi_spec):
        auth_scheme, auth_credential = token_to_scheme_credential(
            "oauth2Token", "header", "Authorization", "my-access-token"
        )

        toolset = OpenAPIToolset(
            spec_dict=openapi_spec,
            auth_scheme=auth_scheme,
            auth_credential=auth_credential,
            tool_filter=["get_pet_by_id"],
        )
        config = toolset._configs[0]

        mock_response = httpx.Response(
            200,
            json={"id": 1, "name": "Buddy"},
            request=httpx.Request("GET", "https://example.com"),
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(config)
        await callable_fn(ctx, pet_id=1)

        call_kwargs = mock_client.request.call_args
        headers = call_kwargs.kwargs.get("headers") or call_kwargs[1].get("headers", {})
        assert "Bearer my-access-token" in headers.get("Authorization", "")

    async def test_no_auth_means_no_auth_header(self, openapi_spec):
        toolset = OpenAPIToolset(
            spec_dict=openapi_spec,
            tool_filter=["get_pet_by_id"],
        )
        config = toolset._configs[0]

        mock_response = httpx.Response(
            200,
            json={"id": 1},
            request=httpx.Request("GET", "https://example.com"),
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(config)
        await callable_fn(ctx, pet_id=1)

        call_kwargs = mock_client.request.call_args
        headers = call_kwargs.kwargs.get("headers") or call_kwargs[1].get("headers", {})
        assert "Authorization" not in headers


# ---------------------------------------------------------------------------
# OpenAPIDeps
# ---------------------------------------------------------------------------


class TestOpenAPIDeps:
    def test_deps_construction(self):
        client = MagicMock(spec=httpx.AsyncClient)
        deps = OpenAPIDeps(client=client)
        assert deps.client is client
        assert deps.header_provider is None

    def test_deps_with_header_provider(self):
        client = MagicMock(spec=httpx.AsyncClient)
        provider = lambda: {"X-Custom": "val"}
        deps = OpenAPIDeps(client=client, header_provider=provider)
        assert deps.header_provider is provider
        assert deps.header_provider() == {"X-Custom": "val"}


# ---------------------------------------------------------------------------
# End-to-end: full spec parse → tool call cycle
# ---------------------------------------------------------------------------


class TestEndToEnd:
    async def test_petstore_get_pet_by_id(self, openapi_spec):
        """Full cycle: parse spec -> create toolset -> get config -> call."""
        toolset = OpenAPIToolset(spec_dict=openapi_spec)
        config = next(c for c in toolset._configs if c.name == "get_pet_by_id")

        pet_data = {"id": 42, "name": "Rex", "status": "available"}
        mock_response = httpx.Response(
            200,
            json=pet_data,
            request=httpx.Request("GET", "https://example.com"),
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(config)
        result = await callable_fn(ctx, pet_id=42)

        assert result == pet_data
        call_kwargs = mock_client.request.call_args
        assert (call_kwargs.kwargs.get("method") or call_kwargs[1].get("method")) == "get"
        assert "/pet/42" in (call_kwargs.kwargs.get("url") or call_kwargs[1].get("url", ""))

    async def test_petstore_add_pet_with_body(self, openapi_spec):
        """Test a POST operation with JSON body."""
        toolset = OpenAPIToolset(spec_dict=openapi_spec)
        config = next(c for c in toolset._configs if c.name == "add_pet")

        mock_response = httpx.Response(
            200,
            json={"id": 1, "name": "Buddy", "status": "available"},
            request=httpx.Request("POST", "https://example.com"),
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(config)
        result = await callable_fn(
            ctx,
            name="Buddy",
            photo_urls=["https://example.com/buddy.jpg"],
            status="available",
        )

        assert result["name"] == "Buddy"
        call_kwargs = mock_client.request.call_args
        assert (call_kwargs.kwargs.get("method") or call_kwargs[1].get("method")) == "post"
        json_body = call_kwargs.kwargs.get("json") or call_kwargs[1].get("json")
        assert json_body is not None
        assert json_body["name"] == "Buddy"

    async def test_petstore_find_pets_by_status_with_default(self, openapi_spec):
        """Test that required params with defaults are filled in."""
        toolset = OpenAPIToolset(spec_dict=openapi_spec)
        config = next(c for c in toolset._configs if c.name == "find_pets_by_status")

        mock_response = httpx.Response(
            200,
            json=[],
            request=httpx.Request("GET", "https://example.com"),
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(config)
        # Call without providing status — should use default "available"
        result = await callable_fn(ctx)

        call_kwargs = mock_client.request.call_args
        assert (call_kwargs.kwargs.get("method") or call_kwargs[1].get("method")) == "get"
        params = call_kwargs.kwargs.get("params") or call_kwargs[1].get("params", {})
        assert params.get("status") == "available"

    async def test_petstore_delete_pet(self, openapi_spec):
        """Test a DELETE operation with path param."""
        toolset = OpenAPIToolset(spec_dict=openapi_spec)
        config = next(c for c in toolset._configs if c.name == "delete_pet")

        mock_response = httpx.Response(
            200,
            json={"message": "Pet deleted"},
            request=httpx.Request("DELETE", "https://example.com"),
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(config)
        result = await callable_fn(ctx, pet_id=42)

        call_kwargs = mock_client.request.call_args
        assert (call_kwargs.kwargs.get("method") or call_kwargs[1].get("method")) == "delete"
        assert "/pet/42" in (call_kwargs.kwargs.get("url") or call_kwargs[1].get("url", ""))

    async def test_petstore_login_user_with_query_params(self, openapi_spec):
        """Test a GET operation with multiple query params."""
        toolset = OpenAPIToolset(spec_dict=openapi_spec)
        config = next(c for c in toolset._configs if c.name == "login_user")

        mock_response = httpx.Response(
            200,
            json="session-token-xyz",
            request=httpx.Request("GET", "https://example.com"),
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(config)
        await callable_fn(ctx, username="admin", password="secret")

        call_kwargs = mock_client.request.call_args
        assert (call_kwargs.kwargs.get("method") or call_kwargs[1].get("method")) == "get"
        params = call_kwargs.kwargs.get("params") or call_kwargs[1].get("params", {})
        assert params.get("username") == "admin"
        assert params.get("password") == "secret"

    async def test_petstore_get_user_by_name_url(self, openapi_spec):
        """Test path param substitution for string path params."""
        toolset = OpenAPIToolset(spec_dict=openapi_spec)
        config = next(c for c in toolset._configs if c.name == "get_user_by_name")

        mock_response = httpx.Response(
            200,
            json={"username": "john"},
            request=httpx.Request("GET", "https://example.com"),
        )
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.request = AsyncMock(return_value=mock_response)

        deps = OpenAPIDeps(client=mock_client)
        ctx = _make_run_context(deps)

        callable_fn = create_rest_api_callable(config)
        await callable_fn(ctx, username="john")

        call_kwargs = mock_client.request.call_args
        assert (call_kwargs.kwargs.get("method") or call_kwargs[1].get("method")) == "get"
        url = call_kwargs.kwargs.get("url") or call_kwargs[1].get("url", "")
        assert "/user/john" in url


# ---------------------------------------------------------------------------
# OpenApiSpecParser
# ---------------------------------------------------------------------------


class TestOpenApiSpecParser:
    """Tests for the spec parser that resolves $ref and produces ParsedOperations."""

    def test_parse_returns_parsed_operations(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        assert len(operations) > 0
        for op in operations:
            assert op.name
            assert op.endpoint is not None
            assert op.operation is not None

    def test_all_operations_have_endpoints(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        for op in operations:
            assert op.endpoint.base_url is not None
            assert op.endpoint.path
            assert op.endpoint.method

    def test_ref_schemas_resolved(self, openapi_spec):
        """Ensure $ref schemas are resolved into actual properties."""
        operations = OpenApiSpecParser().parse(openapi_spec)
        # addPet uses $ref to Pet schema in requestBody
        add_pet = next(o for o in operations if o.name == "add_pet")
        # The parser should have resolved the $ref and produced parameters
        param_names = {p.original_name for p in add_pet.parameters}
        # Pet schema has name, photoUrls, status, id, category, tags
        assert "name" in param_names
        assert "photoUrls" in param_names

    def test_operation_methods_correct(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        method_map = {op.name: op.endpoint.method.lower() for op in operations}
        assert method_map["add_pet"] == "post"
        assert method_map["update_pet"] == "put"
        assert method_map["get_pet_by_id"] == "get"
        assert method_map["delete_pet"] == "delete"

    def test_operation_paths_correct(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        path_map = {op.name: op.endpoint.path for op in operations}
        assert path_map["get_pet_by_id"] == "/pet/{petId}"
        assert path_map["find_pets_by_status"] == "/pet/findByStatus"
        assert path_map["get_inventory"] == "/store/inventory"
        assert path_map["get_user_by_name"] == "/user/{username}"

    def test_security_schemes_parsed(self, openapi_spec):
        """Operations with security should have auth_scheme populated."""
        operations = OpenApiSpecParser().parse(openapi_spec)
        get_inventory = next(o for o in operations if o.name == "get_inventory")
        # getInventory uses api_key security
        assert get_inventory.auth_scheme is not None
        from flokoa.auth.auth_schemes import AuthSchemeType

        assert get_inventory.auth_scheme.type_ == AuthSchemeType.apiKey

    def test_operations_without_security_have_no_auth_scheme(self, openapi_spec):
        """Operations without explicit security should have no auth_scheme."""
        operations = OpenApiSpecParser().parse(openapi_spec)
        # placeOrder has no security defined
        place_order = next(o for o in operations if o.name == "place_order")
        assert place_order.auth_scheme is None or place_order.auth_credential is None

    def test_path_parameters_parsed(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        get_pet = next(o for o in operations if o.name == "get_pet_by_id")
        path_params = [p for p in get_pet.parameters if p.param_location == "path"]
        assert len(path_params) == 1
        assert path_params[0].original_name == "petId"

    def test_query_parameters_parsed(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        find_by_status = next(o for o in operations if o.name == "find_pets_by_status")
        query_params = [p for p in find_by_status.parameters if p.param_location == "query"]
        assert len(query_params) == 1
        assert query_params[0].original_name == "status"

    def test_body_parameters_parsed(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        add_pet = next(o for o in operations if o.name == "add_pet")
        body_params = [p for p in add_pet.parameters if p.param_location == "body"]
        assert len(body_params) > 0

    def test_total_operation_count(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        expected = sum(len(methods) for methods in openapi_spec["paths"].values())
        assert len(operations) == expected

    def test_operation_names_are_snake_case(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        for op in operations:
            assert op.name == op.name.lower()
            assert " " not in op.name


# ---------------------------------------------------------------------------
# OperationParser
# ---------------------------------------------------------------------------


class TestOperationParser:
    """Tests for OperationParser JSON schema generation and parameter handling."""

    def test_get_json_schema_for_get_pet(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        get_pet = next(o for o in operations if o.name == "get_pet_by_id")
        parser = OperationParser.load(get_pet.operation, get_pet.parameters, get_pet.return_value)
        schema = parser.get_json_schema()

        assert schema["type"] == "object"
        assert "properties" in schema
        assert "pet_id" in schema["properties"]
        assert "pet_id" in schema["required"]

    def test_get_json_schema_for_find_by_status(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        find = next(o for o in operations if o.name == "find_pets_by_status")
        parser = OperationParser.load(find.operation, find.parameters, find.return_value)
        schema = parser.get_json_schema()

        assert "status" in schema["properties"]
        assert schema["properties"]["status"]["type"] == "string"

    def test_get_json_schema_for_add_pet(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        add_pet = next(o for o in operations if o.name == "add_pet")
        parser = OperationParser.load(add_pet.operation, add_pet.parameters, add_pet.return_value)
        schema = parser.get_json_schema()

        assert schema["type"] == "object"
        # Pet schema properties should be present
        assert "name" in schema["properties"]
        assert "photo_urls" in schema["properties"]

    def test_get_parameters_returns_list(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        get_pet = next(o for o in operations if o.name == "get_pet_by_id")
        parser = OperationParser.load(get_pet.operation, get_pet.parameters, get_pet.return_value)
        params = parser.get_parameters()

        assert isinstance(params, list)
        assert len(params) == 1
        assert params[0].original_name == "petId"
        assert params[0].param_location == "path"

    def test_get_function_name(self, openapi_spec):
        operations = OpenApiSpecParser().parse(openapi_spec)
        get_pet = next(o for o in operations if o.name == "get_pet_by_id")
        parser = OperationParser.load(get_pet.operation, get_pet.parameters, get_pet.return_value)
        assert parser.get_function_name() == "get_pet_by_id"

    def test_mixed_param_locations(self, openapi_spec):
        """updatePetWithForm has path and query params."""
        operations = OpenApiSpecParser().parse(openapi_spec)
        update_form = next(o for o in operations if o.name == "update_pet_with_form")
        parser = OperationParser.load(
            update_form.operation, update_form.parameters, update_form.return_value
        )
        params = parser.get_parameters()
        locations = {p.param_location for p in params}
        assert "path" in locations
        assert "query" in locations
