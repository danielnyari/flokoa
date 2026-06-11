"""Stage: resolve_secrets — ${secret:NAME} placeholder resolution.

Placeholders resolve from FLOKOA_SECRET_* environment variables the operator
projected via ``valueFrom.secretKeyRef`` (runtime contract §3). Resolution is
all-or-nothing: every missing reference is reported at once. Secret values
are never logged and never written back to disk.
"""

from __future__ import annotations

import os
import re
from typing import Any

from flokoa_runner.errors import BootstrapError

PLACEHOLDER_RE = re.compile(r"\$\{secret:([A-Za-z0-9._-]+)\}")

_NON_ALNUM = re.compile(r"[^A-Z0-9]")


def secret_env_name(name: str) -> str:
    """The contract's normalization rule (§3), mirrored by the Go compiler:
    uppercase, every character outside [A-Z0-9] becomes "_", prefixed
    FLOKOA_SECRET_. Golden pairs are asserted against the Go side."""
    return "FLOKOA_SECRET_" + _NON_ALNUM.sub("_", name.upper())


def resolve_secrets(doc: Any, env: dict[str, str] | None = None) -> Any:
    """Replace every placeholder in every string value of the document."""
    environ = os.environ if env is None else env

    missing: set[str] = set()

    def substitute(value: Any) -> Any:
        if isinstance(value, str):

            def replace(match: re.Match[str]) -> str:
                name = match.group(1)
                env_name = secret_env_name(name)
                resolved = environ.get(env_name)
                if resolved is None:
                    missing.add(name)
                    return match.group(0)
                return resolved

            return PLACEHOLDER_RE.sub(replace, value)
        if isinstance(value, dict):
            return {k: substitute(v) for k, v in value.items()}
        if isinstance(value, list):
            return [substitute(v) for v in value]
        return value

    resolved = substitute(doc)

    if missing:
        raise BootstrapError(
            "resolve_secrets",
            "missing secret refs",
            missing=sorted(missing),
        )
    return resolved
