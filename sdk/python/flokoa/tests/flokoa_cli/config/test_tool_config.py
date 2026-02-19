"""Tests for flokoa.config.tool_config — ToolConfig model."""

import pytest
from pydantic import ValidationError

from flokoa.config.code_ref import CodeRef
from flokoa.config.tool_config import ToolConfig, ToolRefType


class TestToolRefType:
    def test_enum_values(self):
        assert ToolRefType.OPENAPI == "openapi"
        assert ToolRefType.FUNCTION == "function"
        assert ToolRefType.CLASS == "class"


class TestToolConfig:
    def test_openapi_tool(self):
        tc = ToolConfig(
            name="petstore",
            type="openapi",
            open_api={"openApiSchema": {"value": {"openapi": "3.0.0"}}},
        )
        assert tc.type == ToolRefType.OPENAPI
        assert tc.open_api is not None
        assert tc.code is None

    def test_openapi_tool_with_alias(self):
        tc = ToolConfig.model_validate({
            "name": "petstore",
            "type": "openapi",
            "openApi": {"openApiSchema": {"value": {"openapi": "3.0.0"}}},
        })
        assert tc.type == ToolRefType.OPENAPI
        assert tc.open_api is not None

    def test_function_tool(self):
        tc = ToolConfig(
            name="calculate",
            type="function",
            code=CodeRef(name="my_app.tools.calculate"),
        )
        assert tc.type == ToolRefType.FUNCTION
        assert tc.code is not None
        assert tc.code.name == "my_app.tools.calculate"

    def test_class_tool(self):
        tc = ToolConfig(
            name="search",
            type="class",
            code=CodeRef(name="my_app.tools.SearchTool"),
        )
        assert tc.type == ToolRefType.CLASS

    def test_class_tool_with_args(self):
        tc = ToolConfig(
            name="search",
            type="class",
            code=CodeRef(
                name="my_app.tools.SearchTool",
                args=[{"name": "max_results", "value": 10}],
            ),
        )
        assert tc.code.args[0].name == "max_results"
        assert tc.code.args[0].value == 10

    def test_default_type_is_openapi(self):
        tc = ToolConfig(
            name="test",
            open_api={"spec": {}},
        )
        assert tc.type == ToolRefType.OPENAPI

    def test_openapi_without_spec_raises(self):
        with pytest.raises(ValidationError, match="openApi"):
            ToolConfig(name="test", type="openapi")

    def test_function_without_code_raises(self):
        with pytest.raises(ValidationError, match="code"):
            ToolConfig(name="test", type="function")

    def test_class_without_code_raises(self):
        with pytest.raises(ValidationError, match="code"):
            ToolConfig(name="test", type="class")

    def test_extra_fields_forbidden(self):
        with pytest.raises(ValidationError):
            ToolConfig(name="test", type="function", code=CodeRef(name="a.b"), extra="bad")
