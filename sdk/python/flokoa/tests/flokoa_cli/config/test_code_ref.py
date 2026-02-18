"""Tests for flokoa.config.code_ref — CodeRef model and resolution."""

import pytest
from pydantic import ValidationError

from flokoa.config.code_ref import (
    Argument,
    CodeRef,
    is_tool_class,
    is_tool_instance,
    resolve_code_ref,
    resolve_qualified_name,
)


class TestCodeRefModel:
    def test_valid_dotted_path(self):
        ref = CodeRef(name="os.path.join")
        assert ref.name == "os.path.join"
        assert ref.args is None

    def test_rejects_non_dotted_path(self):
        with pytest.raises(ValidationError, match="fully-qualified dotted path"):
            CodeRef(name="join")

    def test_rejects_empty_name(self):
        with pytest.raises(ValidationError):
            CodeRef(name="")

    def test_with_args(self):
        ref = CodeRef(
            name="os.path.join",
            args=[
                Argument(name="a", value="/home"),
                Argument(value="user"),  # positional
            ],
        )
        assert len(ref.args) == 2
        assert ref.args[0].name == "a"
        assert ref.args[1].name is None

    def test_extra_fields_forbidden(self):
        with pytest.raises(ValidationError):
            CodeRef(name="os.path.join", unknown_field="x")


class TestArgument:
    def test_keyword_arg(self):
        arg = Argument(name="key", value="val")
        assert arg.name == "key"
        assert arg.value == "val"

    def test_positional_arg(self):
        arg = Argument(value=42)
        assert arg.name is None
        assert arg.value == 42


class TestResolveQualifiedName:
    def test_resolves_stdlib(self):
        result = resolve_qualified_name("os.path.join")
        import os.path
        assert result is os.path.join

    def test_resolves_class(self):
        result = resolve_qualified_name("collections.OrderedDict")
        from collections import OrderedDict
        assert result is OrderedDict

    def test_raises_import_error_for_bad_module(self):
        with pytest.raises(ImportError):
            resolve_qualified_name("nonexistent_module.something")

    def test_raises_attribute_error_for_bad_attr(self):
        with pytest.raises(AttributeError):
            resolve_qualified_name("os.path.nonexistent_function")

    def test_raises_import_error_for_no_dots(self):
        with pytest.raises(ImportError, match="no module path"):
            resolve_qualified_name("nodots")


class TestResolveCodeRef:
    def test_resolves_function(self):
        ref = CodeRef(name="os.path.join")
        result = resolve_code_ref(ref)
        import os.path
        assert result is os.path.join

    def test_resolves_and_calls_with_args(self):
        ref = CodeRef(
            name="os.path.join",
            args=[Argument(value="/home"), Argument(value="user")],
        )
        result = resolve_code_ref(ref)
        assert result == "/home/user"

    def test_resolves_with_keyword_args(self):
        # Use dict() as a callable that accepts keyword args
        ref = CodeRef(
            name="builtins.dict",
            args=[Argument(name="a", value=1), Argument(name="b", value=2)],
        )
        result = resolve_code_ref(ref)
        assert result == {"a": 1, "b": 2}

    def test_raises_type_error_for_non_callable_with_args(self):
        ref = CodeRef(
            name="os.sep",
            args=[Argument(value="x")],
        )
        with pytest.raises(TypeError, match="not callable"):
            resolve_code_ref(ref)


class TestIntrospectionHelpers:
    def test_is_tool_class(self):
        assert is_tool_class(dict) is True
        assert is_tool_class(list) is True
        assert is_tool_class(lambda: None) is False
        assert is_tool_class("string") is False

    def test_is_tool_instance(self):
        assert is_tool_instance({}) is True
        assert is_tool_instance("string") is True
        assert is_tool_instance(dict) is False
        assert is_tool_instance(lambda: None) is False
