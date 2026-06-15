"""Generate built-in capability metadata + their source: builtin Capability CRs.

Built-in capabilities (source: builtin) are first-party capabilities baked into
the runner image — nothing is downloaded at deploy/run time (runtime contract,
architecture §2). This generator introspects each built-in's entrypoint class
*inside the runner image's environment* (the same env a runner pod has) and
emits, per built-in:

1. A built-in metadata entry merged into the runner manifest
   (/etc/flokoa/runner-manifest.json) and the operator-embedded baseline
   (operator/internal/spec/baselines/runner-baseline-<v>.json) — the offline
   source of truth admission validates builtin CRs against (§2.4).
2. A `source: builtin` Capability CR YAML for the Helm chart + config samples
   (§2.2) so the capabilities exist the moment the chart is installed.

Schema derivation reuses flokoa's in-runner machinery
(flokoa.capability_cli._inrunner.derive_schema) and the canonical schema-digest
helper (flokoa.capability_cli.artifact.canonical_schema_digest) so a built-in's
schemaDigest is computed identically to an artifact-backed capability's.

The requires tuple is pinned to the runner's own versions: the capability *is*
the runner, so it requires the runner's exact Python minor, the runner's
pydantic-ai pin, and the runner version itself. dependencies is always [] —
everything a built-in needs is baseline, so it never seeds a conflict against
the baseline it is part of.

Usage: python hack/gen_builtin_capabilities.py   # writes the chart/samples CRs
       (the metadata map is also consumed by gen_runner_manifest.py)
"""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any

import yaml
from flokoa.capability_cli._inrunner.derive_schema import (
    derive_config_schema,
    import_entrypoint,
)
from flokoa.capability_cli.artifact import canonical_schema_digest
from flokoa_runner import RUNNER_VERSION

PYTHON_MINOR = "3.13"

# The built-in capability set baked into the runner image. Each entry names the
# distribution (for documentation) and its entrypoint (module:Class). Only
# flokoa-openapi is built in for now: flokoa-codemode-mcp is an MCP server, not
# an AbstractCapability subclass (architecture §2.7), so it is NOT a built-in
# capability — it stays a separately-deployed MCP server fronted by an AgentTool
# until it grows a capability-class entrypoint.
BUILTIN_CAPABILITIES: list[dict[str, str]] = [
    {
        "name": "flokoa-openapi",
        "entrypoint": "flokoa_openapi.capability:OpenAPI",
    },
]


def _serialization_name(cls: type) -> str:
    """The capability's spec-entry name (the class's get_serialization_name())."""
    get_name = getattr(cls, "get_serialization_name", None)
    if get_name is None:
        return cls.__name__
    name = get_name()
    if name is None:
        raise SystemExit(
            f"built-in {cls.__module__}:{cls.__name__} returns None from "
            "get_serialization_name() — it cannot be a capability"
        )
    return name


def _requires() -> dict[str, str]:
    """The requires tuple pinned to the runner's own versions (§2.4)."""
    return {
        "python": PYTHON_MINOR,
        "pydantic-ai": f"=={_pydantic_ai_version()}",
        "flokoa-runner": f"=={RUNNER_VERSION}",
    }


def _pydantic_ai_version() -> str:
    import importlib.metadata

    return importlib.metadata.version("pydantic-ai")


def build_builtin_metadata() -> dict[str, dict[str, Any]]:
    """Introspect every built-in's entrypoint and emit the metadata map.

    Runs inside the runner env: the built-in classes are importable from the
    venv (no wheelhouse, no pip), exactly as the runner resolves them at
    bootstrap (capabilities.resolve_builtin_capabilities).
    """
    metadata: dict[str, dict[str, Any]] = {}
    for entry in BUILTIN_CAPABILITIES:
        name = entry["name"]
        entrypoint = entry["entrypoint"]
        cls = import_entrypoint(entrypoint)
        attr = entrypoint.partition(":")[2]
        serialization_name = _serialization_name(cls)

        schema, reason = derive_config_schema(cls)
        if schema is None:
            raise SystemExit(f"built-in {name} ({entrypoint}) has an underivable config schema: {reason}")

        info: dict[str, Any] = {
            "entrypoint": entrypoint,
            "requires": _requires(),
            # Built-ins are part of the runner baseline; nothing is delivered,
            # so the dependency closure is empty for conflict detection.
            "dependencies": [],
            "schemaDigest": canonical_schema_digest(schema),
            "configSchema": schema,
        }
        # Only record serializationName when the class overrides the default
        # (pydantic-ai's default is the class name == the entrypoint attr), so
        # the CR mirror stays minimal and matches the build pipeline's rule.
        if serialization_name != attr:
            info["serializationName"] = serialization_name
        metadata[name] = info
    return metadata


def render_builtin_cr(name: str, info: dict[str, Any]) -> dict[str, Any]:
    """Render the source: builtin Capability CR for one built-in (no artifact)."""
    requires = info["requires"]
    spec: dict[str, Any] = {
        "source": "builtin",
        # Built-in version tracks the runner version, not a separate cadence.
        "version": RUNNER_VERSION,
        "entrypoint": info["entrypoint"],
        "requires": {
            "python": requires["python"],
            "pydanticAI": requires["pydantic-ai"],
            "flokoaRunner": requires["flokoa-runner"],
        },
        "configSchema": info["configSchema"],
    }
    if "serializationName" in info:
        spec["serializationName"] = info["serializationName"]
    return {
        "apiVersion": "agent.flokoa.ai/v1alpha1",
        "kind": "Capability",
        "metadata": {"name": name},
        "spec": spec,
    }


_GENERATED_HEADER = (
    "# Generated by flokoa-runner/hack/gen_builtin_capabilities.py — DO NOT EDIT.\n"
    "# A source: builtin Capability CR: the capability is baked into the runner\n"
    "# image, so it carries no artifact (architecture §2.2). Regenerate with\n"
    "# `make builtin-capabilities` (sdk/python).\n"
)


def write_cr_yaml(out_path: Path, doc: dict[str, Any]) -> None:
    out_path.parent.mkdir(parents=True, exist_ok=True)
    body = yaml.safe_dump(doc, sort_keys=False, default_flow_style=False)
    out_path.write_text(_GENERATED_HEADER + body, encoding="utf-8")
    print(f"wrote {out_path}")


def main(metadata_dump: Path | None = None) -> None:
    pkg_root = Path(__file__).resolve().parents[1]
    repo_root = pkg_root.parents[2]
    chart_dir = repo_root / "operator" / "charts" / "flokoa" / "files" / "builtin-capabilities"
    samples_dir = repo_root / "operator" / "config" / "samples"

    metadata = build_builtin_metadata()

    # Optional: dump the metadata map for inspection / debugging.
    if metadata_dump is not None:
        metadata_dump.write_text(json.dumps(metadata, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print(f"wrote {metadata_dump}")

    for name, info in metadata.items():
        doc = render_builtin_cr(name, info)
        write_cr_yaml(chart_dir / f"{name}.yaml", doc)
        write_cr_yaml(samples_dir / f"builtin_capability_{name.replace('-', '_')}.yaml", doc)


if __name__ == "__main__":
    main(Path(sys.argv[1]) if len(sys.argv) > 1 else None)
