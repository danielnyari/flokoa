from __future__ import annotations

import inspect
from collections.abc import Callable
from typing import Any

from flokoa_common.utils.openapi.openapi_spec_parser import ParsedOperation
from flokoa_common.utils.openapi.operation_parser import OperationParser

# Map old-style type hints (from TypeHintHelper) to modern Python syntax
# that Monty's type checker understands.
_TYPE_HINT_MODERNIZE: dict[str, str] = {
    "List": "list",
    "Dict": "dict",
    "Tuple": "tuple",
    "Set": "set",
    "FrozenSet": "frozenset",
    "Optional": "None |",
}


def _modernize_type_hint(hint: str) -> str:
    """Convert old-style type hints to modern Python 3.10+ syntax."""
    for old, new in _TYPE_HINT_MODERNIZE.items():
        hint = hint.replace(f"{old}[", f"{new}[")
    return hint


def _generate_operation_stub(operation: ParsedOperation) -> str:
    """Generate a Python async function stub from a ParsedOperation."""
    parser = OperationParser.load(operation.operation, operation.parameters, operation.return_value)

    fn_name = parser.get_function_name()
    params = parser.get_parameters()
    return_hint = _modernize_type_hint(parser.get_return_type_hint())
    docstring = parser.get_pydoc_string()

    # Build parameter list
    param_parts: list[str] = []
    for p in params:
        hint = _modernize_type_hint(p.type_hint)
        if p.required:
            param_parts.append(f"{p.py_name}: {hint}")
        else:
            param_parts.append(f"{p.py_name}: {hint} = None")

    params_str = ", ".join(param_parts)

    lines = [f"async def {fn_name}({params_str}) -> {return_hint}:"]
    if docstring:
        # Re-indent each line of the docstring to sit inside the function body
        for doc_line in docstring.splitlines():
            stripped = doc_line.strip()
            lines.append(f"    {stripped}" if stripped else "")
    lines.append("    raise NotImplementedError()")
    lines.append("")

    return "\n".join(lines)


def _generate_callable_stub(name: str, fn: Callable[..., Any]) -> str:
    """Generate a Python function stub from a callable using inspect."""
    sig = inspect.signature(fn)
    is_async = inspect.iscoroutinefunction(fn)

    param_parts: list[str] = []
    for param_name, param in sig.parameters.items():
        hint = "Any"
        if param.annotation is not inspect.Parameter.empty:
            ann = param.annotation
            hint = ann.__name__ if hasattr(ann, "__name__") else str(ann)

        if param.default is not inspect.Parameter.empty:
            param_parts.append(f"{param_name}: {hint} = ...")
        else:
            param_parts.append(f"{param_name}: {hint}")

    params_str = ", ".join(param_parts)

    return_hint = "Any"
    if sig.return_annotation is not inspect.Signature.empty:
        ann = sig.return_annotation
        return_hint = ann.__name__ if hasattr(ann, "__name__") else str(ann)

    prefix = "async def" if is_async else "def"
    doc = inspect.getdoc(fn)
    lines = [f"{prefix} {name}({params_str}) -> {return_hint}:"]
    if doc:
        lines.append(f'    """{doc}"""')
    lines.append("    raise NotImplementedError()")
    lines.append("")

    return "\n".join(lines)


def generate_stubs(
    operations: list[ParsedOperation],
    external_functions: dict[str, Callable[..., Any]] | None = None,
    external_function_stubs: dict[str, str] | None = None,
) -> str:
    """Generate Python function stubs for Monty type checking and LLM context.

    Produces a complete Python module string containing async function stubs
    for each OpenAPI operation and any additional external functions. These
    stubs serve as both type-check input for Monty and as context for the LLM
    to understand what functions are available.

    Args:
        operations: Parsed OpenAPI operations to generate stubs for.
        external_functions: Additional user-provided callables. Stubs are
            auto-generated from their signatures via inspect.
        external_function_stubs: Explicit stub strings keyed by function name.
            These take precedence over auto-generated stubs for the same name.

    Returns:
        A Python source string with all function stubs.
    """
    external_functions = external_functions or {}
    external_function_stubs = external_function_stubs or {}

    parts: list[str] = ["from typing import Any", ""]

    # Generate stubs for OpenAPI operations
    if operations:
        parts.append("# API Operations")
        parts.append("")
        for op in operations:
            parts.append(_generate_operation_stub(op))

    # Generate stubs for user-provided external functions
    if external_functions:
        parts.append("# External Functions")
        parts.append("")
        for name, fn in external_functions.items():
            if name in external_function_stubs:
                # Use explicit stub override
                parts.append(external_function_stubs[name])
                parts.append("")
            else:
                # Auto-generate from signature
                parts.append(_generate_callable_stub(name, fn))

    return "\n".join(parts)
