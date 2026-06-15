"""Resolve the capability entrypoint and derive its config schema.

Runs inside the runner image, after the wheelhouse install (it installs the
pin set itself if needed — the ``--skip-smoke-test`` path).

Entrypoint resolution: explicit ``--entrypoint`` wins; otherwise a heuristic
enumerates the classes exported by the distribution's top-level modules and
keeps the pydantic-ai ``AbstractCapability`` subclasses the dist itself
defines (matching what ``flokoa_runner.capabilities._load_entrypoint``
ultimately imports). Exactly one candidate → picked; several → listed for an
``--entrypoint`` choice.

Schema derivation (pydantic's ``TypeAdapter``/``create_model`` machinery):
dataclasses and pydantic models directly; otherwise a typed ``from_spec``
override or typed ``__init__`` signature. ``*args``/``**kwargs``/untyped
parameters make the schema underivable — classified, never guessed. The
framework base fields every AbstractCapability carries (``id``,
``description``, ``defer_loading``) are dropped from the derived schema:
they are spec-entry plumbing, not per-agent capability config.

Imports: stdlib + pydantic (+ pydantic-ai, both runner baseline).

Output (in the work dir): schema-report.json
  {"outcome": "derived"|"underivable"|"ambiguous"|"no-candidates",
   "entrypoint", "serializationName", "schema", "reason", "candidates"}.
Exit 0 unless the entrypoint cannot be imported at all — outcome policy
(refuse / --permissive) is the host orchestrator's call.
"""

from __future__ import annotations

import argparse
import dataclasses
import importlib
import importlib.metadata
import inspect
import json
import re
import subprocess
import sys
import traceback
from pathlib import Path
from typing import Any

import pydantic

_BASE_CAPABILITY_FIELDS = ("id", "description", "defer_loading")


def ensure_pip() -> None:
    """The runner venv is uv-managed and may ship without pip; seed it once."""
    probe = subprocess.run([sys.executable, "-m", "pip", "--version"], capture_output=True, text=True, check=False)
    if probe.returncode != 0:
        subprocess.run([sys.executable, "-m", "ensurepip", "--upgrade"], check=True, capture_output=True)


def normalize(name: str) -> str:
    """PEP 503 normalization."""
    return re.sub(r"[-_.]+", "-", name).lower()


def ensure_installed(wheelhouse: Path, name: str, version: str, dependencies: list[str]) -> None:
    """Install the pin set when the smoke step was skipped (idempotent)."""
    try:
        importlib.metadata.distribution(name)
        return
    except importlib.metadata.PackageNotFoundError:
        pass
    pins = [f"{name}=={version}", *dependencies]
    result = subprocess.run(
        [sys.executable, "-m", "pip", "install", "--no-index", "--find-links", str(wheelhouse), *pins],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        print(f"ERROR: wheelhouse install failed:\n{result.stderr.strip()[-2000:]}", file=sys.stderr)
        raise SystemExit(1)


# --- entrypoint resolution ---------------------------------------------------


def dist_top_level_modules(dist_name: str) -> list[str]:
    """Top-level importable modules owned by the distribution."""
    wanted = normalize(dist_name)
    return sorted(
        module
        for module, dists in importlib.metadata.packages_distributions().items()
        if any(normalize(d) == wanted for d in dists)
    )


def candidate_capability_classes(modules: list[Any]) -> list[type]:
    """Exported, concrete AbstractCapability subclasses the dist defines."""
    from pydantic_ai.capabilities.abstract import AbstractCapability

    owned_prefixes = tuple(module.__name__ for module in modules)
    seen: dict[int, type] = {}
    for module in modules:
        for obj in vars(module).values():
            if not (isinstance(obj, type) and issubclass(obj, AbstractCapability)):
                continue
            if obj is AbstractCapability or inspect.isabstract(obj):
                continue
            # Only classes the dist itself defines — not re-exported
            # pydantic-ai builtins or other libraries' capabilities.
            if not obj.__module__.startswith(owned_prefixes):
                continue
            seen[id(obj)] = obj
    return sorted(seen.values(), key=lambda cls: (cls.__module__, cls.__name__))


def entrypoint_of(cls: type) -> str:
    """Canonical module:attr — the defining module, the class's own name."""
    return f"{cls.__module__}:{cls.__name__}"


def import_entrypoint(entrypoint: str) -> type:
    module_name, _, attr = entrypoint.partition(":")
    if not module_name or not attr:
        print(f"ERROR: entrypoint must be module:attr, got {entrypoint!r}", file=sys.stderr)
        raise SystemExit(1)
    try:
        return getattr(importlib.import_module(module_name), attr)
    except Exception:
        print(
            f"ERROR: capability entrypoint failed to import: {entrypoint}\n{traceback.format_exc()}",
            file=sys.stderr,
        )
        raise SystemExit(1) from None


# --- schema derivation --------------------------------------------------------


def derive_config_schema(cls: type) -> tuple[dict[str, Any] | None, str | None]:
    """Derive the per-agent config JSON Schema for a capability class.

    Returns ``(schema, None)`` on success or ``(None, reason)`` when the
    class shape is underivable.
    """
    try:
        if dataclasses.is_dataclass(cls):
            schema = pydantic.TypeAdapter(cls).json_schema()
        elif isinstance(cls, type) and issubclass(cls, pydantic.BaseModel):
            schema = cls.model_json_schema()
        else:
            schema, reason = _schema_from_signature(cls)
            if schema is None:
                return None, reason
    except Exception as exc:
        return None, f"schema generation failed: {exc.__class__.__name__}: {exc}"
    return _strip_framework_fields(schema), None


def _constructor_callable(cls: type) -> Any:
    """The signature source: an overridden ``from_spec``, else ``__init__``."""
    from_spec = getattr(cls, "from_spec", None)
    if from_spec is not None:
        base = _abstract_capability_from_spec()
        own = getattr(from_spec, "__func__", from_spec)
        if base is None or own is not base:
            return from_spec
    return cls.__init__


def _abstract_capability_from_spec() -> Any:
    try:
        from pydantic_ai.capabilities.abstract import AbstractCapability
    except ImportError:
        return None
    return AbstractCapability.from_spec.__func__


def _schema_from_signature(cls: type) -> tuple[dict[str, Any] | None, str | None]:
    target = _constructor_callable(cls)
    try:
        # eval_str: under `from __future__ import annotations` the parameter
        # annotations are strings — resolve them against the defining module.
        signature = inspect.signature(target, eval_str=True)
    except (TypeError, ValueError, NameError) as exc:
        return None, f"constructor signature is not introspectable: {exc}"

    fields: dict[str, Any] = {}
    for name, param in signature.parameters.items():
        if name in {"self", "cls"}:
            continue
        if param.kind in (inspect.Parameter.VAR_POSITIONAL, inspect.Parameter.VAR_KEYWORD):
            return None, f"constructor takes *{name} — the config shape is open-ended"
        if param.annotation is inspect.Parameter.empty:
            return None, f"constructor parameter {name!r} has no type annotation"
        default = ... if param.default is inspect.Parameter.empty else param.default
        fields[name] = (param.annotation, default)

    model = pydantic.create_model(cls.__name__, **fields)
    return model.model_json_schema(), None


def _strip_framework_fields(schema: dict[str, Any]) -> dict[str, Any]:
    """Drop AbstractCapability's own fields — framework plumbing, not config."""
    properties = schema.get("properties")
    if isinstance(properties, dict):
        for field in _BASE_CAPABILITY_FIELDS:
            properties.pop(field, None)
    required = schema.get("required")
    if isinstance(required, list):
        schema["required"] = [name for name in required if name not in _BASE_CAPABILITY_FIELDS]
        if not schema["required"]:
            del schema["required"]
    return schema


# --- main ---------------------------------------------------------------------


def resolve_and_derive(
    dist_name: str,
    explicit_entrypoint: str | None,
) -> dict[str, Any]:
    report: dict[str, Any] = {
        "outcome": None,
        "entrypoint": None,
        "serializationName": None,
        "schema": None,
        "reason": None,
        "candidates": [],
    }

    if explicit_entrypoint:
        cls = import_entrypoint(explicit_entrypoint)
        report["entrypoint"] = explicit_entrypoint
    else:
        modules = []
        for module_name in dist_top_level_modules(dist_name):
            try:
                modules.append(importlib.import_module(module_name))
            except Exception:
                print(
                    f"ERROR: top-level module {module_name!r} of {dist_name} failed to import:\n"
                    f"{traceback.format_exc()}",
                    file=sys.stderr,
                )
                raise SystemExit(1) from None
        candidates = candidate_capability_classes(modules)
        report["candidates"] = [entrypoint_of(c) for c in candidates]
        if not candidates:
            report["outcome"] = "no-candidates"
            report["reason"] = (
                f"no concrete pydantic-ai AbstractCapability subclass exported by {dist_name} — "
                "pass --entrypoint module:Class"
            )
            return report
        if len(candidates) > 1:
            report["outcome"] = "ambiguous"
            report["reason"] = "multiple capability classes exported — pick one with --entrypoint"
            return report
        cls = candidates[0]
        report["entrypoint"] = entrypoint_of(cls)

    attr = report["entrypoint"].partition(":")[2]
    serialization_name = _serialization_name(cls)
    if serialization_name is None:
        print(
            f"ERROR: {report['entrypoint']} opts out of spec-based construction "
            "(get_serialization_name() returned None) — it cannot be a capability artifact",
            file=sys.stderr,
        )
        raise SystemExit(1)
    if serialization_name != attr:
        report["serializationName"] = serialization_name

    schema, reason = derive_config_schema(cls)
    if schema is None:
        report["outcome"] = "underivable"
        report["reason"] = reason
    else:
        report["outcome"] = "derived"
        report["schema"] = schema
    return report


def _serialization_name(cls: type) -> str | None:
    get_name = getattr(cls, "get_serialization_name", None)
    if get_name is None:
        return cls.__name__
    return get_name()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--work", required=True, type=Path)
    parser.add_argument("--name", required=True, help="PEP 503-normalized distribution name")
    parser.add_argument("--version", required=True)
    parser.add_argument("--dependency", action="append", default=[])
    parser.add_argument("--entrypoint", default=None)
    args = parser.parse_args()

    ensure_pip()
    ensure_installed(args.work / "wheelhouse", args.name, args.version, args.dependency)

    report = resolve_and_derive(args.name, args.entrypoint)
    (args.work / "schema-report.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    sys.exit(main())
