"""derive_schema: the derivation table + entrypoint heuristic.

The _inrunner script normally executes inside the runner image; the dev venv
carries the same pydantic + pydantic-ai surface, so its pure functions are
unit-testable on the host (the table per architecture §6 / risk register:
pydantic model / dataclass / typed __init__ / untyped → refused /
``**kwargs`` → refused).
"""

from __future__ import annotations

import types
from dataclasses import dataclass
from typing import Any

import pytest
from pydantic import BaseModel
from pydantic_ai.capabilities.abstract import AbstractCapability

from flokoa.capability_cli._inrunner import derive_schema

# --- table fixtures -----------------------------------------------------------


@dataclass
class DataclassCap(AbstractCapability[Any]):
    prefix: str = "echo"
    count: int = 1


class ModelConfigCap(BaseModel):
    """A plain pydantic config class (e.g. a from_spec target)."""

    endpoint: str
    timeout: float = 5.0


class TypedInitCap:
    def __init__(self, prefix: str = "echo", retries: int = 3) -> None:
        self.prefix = prefix
        self.retries = retries


class RequiredTypedInitCap:
    def __init__(self, endpoint: str) -> None:
        self.endpoint = endpoint


class UntypedInitCap:
    def __init__(self, prefix="echo"):
        self.prefix = prefix


class KwargsCap:
    def __init__(self, **kwargs: Any) -> None:
        self.kwargs = kwargs


class FromSpecCap:
    def __init__(self, opaque: object) -> None:
        self.opaque = opaque

    @classmethod
    def from_spec(cls, dsn: str, pool_size: int = 4) -> FromSpecCap:
        return cls((dsn, pool_size))


class TestDeriveConfigSchema:
    def test_dataclass_derives_and_drops_framework_fields(self) -> None:
        schema, reason = derive_schema.derive_config_schema(DataclassCap)
        assert reason is None
        assert schema is not None
        assert set(schema["properties"]) == {"prefix", "count"}
        # AbstractCapability's own fields are plumbing, not per-agent config.
        for framework_field in ("id", "description", "defer_loading"):
            assert framework_field not in schema["properties"]

    def test_pydantic_model_derives(self) -> None:
        schema, reason = derive_schema.derive_config_schema(ModelConfigCap)
        assert reason is None
        assert schema is not None
        assert set(schema["properties"]) == {"endpoint", "timeout"}
        assert schema["required"] == ["endpoint"]

    def test_typed_init_derives(self) -> None:
        schema, reason = derive_schema.derive_config_schema(TypedInitCap)
        assert reason is None
        assert schema is not None
        assert set(schema["properties"]) == {"prefix", "retries"}
        assert schema["properties"]["prefix"]["default"] == "echo"

    def test_typed_init_required_params_marked_required(self) -> None:
        schema, _ = derive_schema.derive_config_schema(RequiredTypedInitCap)
        assert schema is not None
        assert schema["required"] == ["endpoint"]

    def test_untyped_init_underivable(self) -> None:
        schema, reason = derive_schema.derive_config_schema(UntypedInitCap)
        assert schema is None
        assert reason is not None and "no type annotation" in reason

    def test_kwargs_underivable(self) -> None:
        schema, reason = derive_schema.derive_config_schema(KwargsCap)
        assert schema is None
        assert reason is not None and "open-ended" in reason

    def test_overridden_from_spec_signature_wins(self) -> None:
        schema, reason = derive_schema.derive_config_schema(FromSpecCap)
        assert reason is None
        assert schema is not None
        assert set(schema["properties"]) == {"dsn", "pool_size"}

    def test_strip_framework_fields_cleans_required_list(self) -> None:
        schema = {
            "properties": {"id": {}, "prefix": {}},
            "required": ["id", "prefix"],
            "type": "object",
        }
        cleaned = derive_schema._strip_framework_fields(schema)
        assert "id" not in cleaned["properties"]
        assert cleaned["required"] == ["prefix"]


# --- entrypoint heuristic -----------------------------------------------------


def _module_with(name: str, **attrs: Any) -> types.ModuleType:
    module = types.ModuleType(name)
    for attr_name, value in attrs.items():
        setattr(module, attr_name, value)
    return module


def _capability_class(class_name: str, module_name: str) -> type:
    cls = types.new_class(class_name, (AbstractCapability[Any],))
    cls = dataclass(cls)
    cls.__module__ = module_name
    return cls


class TestEntrypointHeuristic:
    def test_single_candidate_selected(self) -> None:
        cap = _capability_class("EchoCapability", "flokoa_cap_echo")
        module = _module_with("flokoa_cap_echo", EchoCapability=cap)
        candidates = derive_schema.candidate_capability_classes([module])
        assert [derive_schema.entrypoint_of(c) for c in candidates] == ["flokoa_cap_echo:EchoCapability"]

    def test_multiple_candidates_listed(self) -> None:
        first = _capability_class("AlphaCapability", "pkg")
        second = _capability_class("BetaCapability", "pkg")
        module = _module_with("pkg", AlphaCapability=first, BetaCapability=second)
        candidates = derive_schema.candidate_capability_classes([module])
        assert [c.__name__ for c in candidates] == ["AlphaCapability", "BetaCapability"]

    def test_reexported_foreign_capability_ignored(self) -> None:
        """pydantic-ai builtins re-exported by the dist are not candidates."""
        foreign = _capability_class("ForeignCapability", "other_library.caps")
        own = _capability_class("OwnCapability", "pkg.core")
        module = _module_with("pkg", ForeignCapability=foreign, OwnCapability=own)
        candidates = derive_schema.candidate_capability_classes([module])
        assert [c.__name__ for c in candidates] == ["OwnCapability"]

    def test_abstract_base_and_non_capability_classes_ignored(self) -> None:
        module = _module_with("pkg", AbstractCapability=AbstractCapability, Plain=type("Plain", (), {}))
        assert derive_schema.candidate_capability_classes([module]) == []

    def test_duplicate_export_deduplicated(self) -> None:
        cap = _capability_class("EchoCapability", "pkg.core")
        top = _module_with("pkg", EchoCapability=cap)
        core = _module_with("pkg.core", EchoCapability=cap)
        candidates = derive_schema.candidate_capability_classes([top, core])
        assert len(candidates) == 1
        # Canonical entrypoint: the defining module, the class's own name.
        assert derive_schema.entrypoint_of(candidates[0]) == "pkg.core:EchoCapability"


class TestNormalize:
    @pytest.mark.parametrize(
        ("raw", "expected"),
        [("Flokoa_Cap.Echo", "flokoa-cap-echo"), ("inflection", "inflection")],
    )
    def test_pep503(self, raw: str, expected: str) -> None:
        assert derive_schema.normalize(raw) == expected
