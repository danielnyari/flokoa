"""Code reference system for resolving Python objects from dotted paths.

Enables declarative agent configs to reference Python callables, classes, and
instances by fully-qualified name (e.g., ``"my_package.tools.search"``).

Inspired by Google ADK's ``CodeConfig`` pattern, adapted for Flokoa's
Kubernetes-native context where referenced code is made available via
volume mounts.
"""

from __future__ import annotations

import importlib
import inspect
import logging
from typing import Any

from pydantic import BaseModel, ConfigDict, Field, model_validator

logger = logging.getLogger(__name__)


class Argument(BaseModel):
    """An argument passed to a callable resolved from a code reference.

    When ``name`` is provided the argument is passed as a keyword argument.
    When ``name`` is ``None`` the argument is positional.
    """

    model_config = ConfigDict(extra="forbid")

    name: str | None = Field(
        default=None,
        description="Keyword argument name. None for positional arguments.",
    )
    value: Any = Field(description="The argument value.")


class CodeRef(BaseModel):
    """Reference to a Python object by fully-qualified dotted path.

    The ``name`` field holds a dotted import path such as
    ``"my_package.module.my_function"``.  When ``args`` are provided and
    the resolved object is callable, it will be called with those arguments
    and the *return value* is used.

    Examples::

        # Reference a plain function (used as-is)
        CodeRef(name="my_app.tools.calculate_price")

        # Reference a class and call its constructor
        CodeRef(
            name="my_app.tools.WebSearchTool",
            args=[Argument(name="api_key", value="sk-...")],
        )
    """

    model_config = ConfigDict(extra="forbid")

    name: str = Field(
        description="Fully-qualified dotted Python path to the object.",
        min_length=1,
    )
    args: list[Argument] | None = Field(
        default=None,
        description="Arguments to pass if the resolved object is callable.",
    )

    @model_validator(mode="after")
    def _validate_name_has_dot(self) -> CodeRef:
        if "." not in self.name:
            raise ValueError(
                f"CodeRef.name must be a fully-qualified dotted path "
                f"(e.g., 'my_package.module.obj'), got: {self.name!r}"
            )
        return self


def resolve_code_ref(ref: CodeRef) -> Any:
    """Resolve a :class:`CodeRef` to a live Python object.

    1. Import the module from the dotted path.
    2. Retrieve the attribute.
    3. If ``ref.args`` is provided and the object is callable, call it with the
       given positional and keyword arguments.

    Args:
        ref: The code reference to resolve.

    Returns:
        The resolved Python object (or the return value of calling it).

    Raises:
        ImportError: If the module cannot be imported.
        AttributeError: If the object is not found on the module.
        TypeError: If args are provided but the object is not callable.
    """
    obj = resolve_qualified_name(ref.name)

    if ref.args is not None:
        if not callable(obj):
            raise TypeError(
                f"CodeRef '{ref.name}' has args but the resolved object is not callable: {type(obj)}"
            )
        kwargs = {a.name: a.value for a in ref.args if a.name is not None}
        positional = [a.value for a in ref.args if a.name is None]
        return obj(*positional, **kwargs)

    return obj


def resolve_qualified_name(name: str) -> Any:
    """Import and return a Python object from a fully-qualified dotted path.

    For a path like ``"my_package.module.MyClass"`` this will
    ``import my_package.module`` and return ``my_package.module.MyClass``.

    Args:
        name: Fully-qualified dotted path (e.g., ``"os.path.join"``).

    Returns:
        The resolved Python object.

    Raises:
        ImportError: If the module cannot be imported.
        AttributeError: If the attribute does not exist on the module.
    """
    module_path, _, obj_name = name.rpartition(".")
    if not module_path:
        raise ImportError(f"Cannot resolve '{name}': no module path (need at least one dot)")
    module = importlib.import_module(module_path)
    return getattr(module, obj_name)


def resolve_callbacks(callback_refs: list[CodeRef]) -> list[Any]:
    """Resolve a list of :class:`CodeRef` items to live Python objects.

    Convenience wrapper around :func:`resolve_code_ref` for callback lists.

    Args:
        callback_refs: List of code references.

    Returns:
        List of resolved Python objects.
    """
    return [resolve_code_ref(ref) for ref in callback_refs]


def is_tool_class(obj: Any) -> bool:
    """Check whether *obj* looks like a tool class (not an instance).

    Returns ``True`` if *obj* is a class that is callable (i.e., can be
    instantiated), as opposed to a plain function or an already-instantiated
    tool object.
    """
    return inspect.isclass(obj)


def is_tool_instance(obj: Any) -> bool:
    """Check whether *obj* is an already-instantiated tool object.

    Returns ``True`` if *obj* is neither a class nor a plain function.
    """
    return not inspect.isclass(obj) and not inspect.isfunction(obj) and not inspect.isbuiltin(obj)
