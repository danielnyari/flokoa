"""Stage: load_compiled_spec — the operator-compiled AgentSpec document."""

from __future__ import annotations

import os
from pathlib import Path
from typing import Any

import yaml

from flokoa_runner.errors import BootstrapError

DEFAULT_SPEC_PATH = "/etc/flokoa/agent-spec.yaml"


def load_compiled_spec(path: str | Path | None = None) -> dict[str, Any]:
    spec_path = Path(path or os.environ.get("FLOKOA_AGENT_SPEC_PATH", DEFAULT_SPEC_PATH))
    try:
        doc = yaml.safe_load(spec_path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        raise BootstrapError("load_compiled_spec", "compiled spec not found", path=str(spec_path)) from None
    except yaml.YAMLError as exc:
        raise BootstrapError(
            "load_compiled_spec", f"compiled spec is not valid YAML: {exc}", path=str(spec_path)
        ) from exc

    if not isinstance(doc, dict):
        raise BootstrapError(
            "load_compiled_spec",
            f"compiled spec must be a mapping, got {type(doc).__name__}",
            path=str(spec_path),
        )
    return doc
