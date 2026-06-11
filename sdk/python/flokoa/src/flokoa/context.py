"""Runtime context accessors for capability authors.

The flokoa runner populates these from the environment and the active A2A
request, so capabilities (including the platform capabilities) can read the
agent's identity and the current session without depending on serving
internals. The A2A ``contextId`` is the session identity end-to-end: the
trigger path mints it, the session router routes on it, and the sessions
backend keys on it.
"""

from __future__ import annotations

import os
from contextvars import ContextVar

_context_id: ContextVar[str | None] = ContextVar("flokoa_context_id", default=None)
_task_id: ContextVar[str | None] = ContextVar("flokoa_task_id", default=None)


def agent_name() -> str | None:
    """The Agent CR name (from OTEL_SERVICE_NAME, set by the operator)."""
    return os.environ.get("FLOKOA_AGENT_NAME") or os.environ.get("OTEL_SERVICE_NAME")


def agent_namespace() -> str | None:
    """The Agent CR namespace (parsed from OTEL_RESOURCE_ATTRIBUTES)."""
    explicit = os.environ.get("FLOKOA_AGENT_NAMESPACE")
    if explicit:
        return explicit
    for pair in os.environ.get("OTEL_RESOURCE_ATTRIBUTES", "").split(","):
        key, _, value = pair.partition("=")
        if key.strip() == "k8s.namespace.name" and value:
            return value.strip()
    return None


def public_url() -> str | None:
    """The agent's published endpoint (FLOKOA_PUBLIC_URL)."""
    return os.environ.get("FLOKOA_PUBLIC_URL")


def context_id() -> str | None:
    """The A2A contextId of the in-flight request (the session identity)."""
    return _context_id.get()


def task_id() -> str | None:
    """The A2A task id of the in-flight request."""
    return _task_id.get()


def bind_request(context_id_value: str | None, task_id_value: str | None) -> None:
    """Bind the active A2A request identifiers (called by the serving layer)."""
    _context_id.set(context_id_value)
    _task_id.set(task_id_value)
