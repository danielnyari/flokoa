"""Stage: install_capabilities — capability wheelhouse install + entrypoint load.

Each Capability artifact is delivered to ``/opt/flokoa/capabilities/<name>/``
(wheels + ``manifest.json``; runtime contract §4). The runner verifies the
``requires`` tuple against its own manifest (defense in depth — admission
already checked), installs with ``pip install --no-index --find-links``, and
registers the entrypoint class for ``Agent.from_spec``.
"""

from __future__ import annotations

import importlib
import json
import os
import subprocess
import sys
from pathlib import Path
from typing import TYPE_CHECKING, Any

from packaging.specifiers import InvalidSpecifier, SpecifierSet
from packaging.version import Version

from flokoa_runner.errors import BootstrapError
from flokoa_runner.manifest import RunnerManifest

if TYPE_CHECKING:
    from pydantic_ai.capabilities.abstract import AbstractCapability

DEFAULT_CAPABILITIES_ROOT = "/opt/flokoa/capabilities"

STAGE = "install_capabilities"


def install_capabilities(
    runner_manifest: RunnerManifest,
    root: str | Path | None = None,
) -> list[type[AbstractCapability[Any]]]:
    """Install every delivered wheelhouse and return the entrypoint classes."""
    cap_root = Path(root or os.environ.get("FLOKOA_CAPABILITIES_PATH", DEFAULT_CAPABILITIES_ROOT))
    if not cap_root.is_dir():
        return []

    classes: list[type[AbstractCapability[Any]]] = []
    for cap_dir in sorted(p for p in cap_root.iterdir() if p.is_dir()):
        manifest = _load_capability_manifest(cap_dir)
        _verify_requires(cap_dir.name, manifest, runner_manifest)
        _pip_install(cap_dir, manifest)
        classes.append(_load_entrypoint(cap_dir.name, manifest))
    return classes


def _load_capability_manifest(cap_dir: Path) -> dict[str, Any]:
    manifest_path = cap_dir / "manifest.json"
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        raise BootstrapError(STAGE, "capability manifest missing", capability=cap_dir.name) from None
    except json.JSONDecodeError as exc:
        raise BootstrapError(STAGE, f"capability manifest invalid: {exc}", capability=cap_dir.name) from exc
    if not isinstance(manifest, dict) or "entrypoint" not in manifest:
        raise BootstrapError(STAGE, "capability manifest must declare an entrypoint", capability=cap_dir.name)
    return manifest


def _verify_requires(name: str, manifest: dict[str, Any], runner: RunnerManifest) -> None:
    requires = manifest.get("requires", {})

    python_req = requires.get("python")
    if python_req and python_req != runner.python:
        raise BootstrapError(
            STAGE,
            "capability requires a different Python minor",
            capability=name,
            requires=python_req,
            runner=runner.python,
        )

    for key, runner_version in (
        ("pydantic-ai", runner.pydantic_ai),
        ("flokoa-runner", runner.runner_version),
    ):
        specifier = requires.get(key)
        if not specifier:
            continue
        try:
            if Version(runner_version) not in SpecifierSet(specifier):
                raise BootstrapError(
                    STAGE,
                    f"capability is incompatible with the runner's {key}",
                    capability=name,
                    requires={key: specifier},
                    runner={key: runner_version},
                )
        except InvalidSpecifier as exc:
            raise BootstrapError(
                STAGE,
                f"capability declares an invalid {key} specifier: {exc}",
                capability=name,
            ) from exc


def _pip_install(cap_dir: Path, manifest: dict[str, Any]) -> None:
    package = manifest.get("name", cap_dir.name)
    version = manifest.get("version")
    requirement = f"{package}=={version}" if version else package
    cmd = [
        sys.executable,
        "-m",
        "pip",
        "install",
        "--no-index",
        "--find-links",
        str(cap_dir),
        requirement,
    ]
    result = subprocess.run(cmd, capture_output=True, text=True, check=False)  # noqa: S603
    if result.returncode != 0:
        raise BootstrapError(
            STAGE,
            "wheelhouse install failed",
            capability=cap_dir.name,
            requirement=requirement,
            pip_stderr=result.stderr.strip()[-2000:],
        )


def _load_entrypoint(name: str, manifest: dict[str, Any]) -> type[AbstractCapability[Any]]:
    entrypoint = manifest["entrypoint"]
    module_name, _, attr = entrypoint.partition(":")
    if not module_name or not attr:
        raise BootstrapError(STAGE, "entrypoint must be module:attr", capability=name, entrypoint=entrypoint)
    try:
        module = importlib.import_module(module_name)
        cls = getattr(module, attr)
    except (ImportError, AttributeError) as exc:
        raise BootstrapError(
            STAGE,
            f"capability entrypoint failed to import: {exc}",
            capability=name,
            entrypoint=entrypoint,
        ) from exc
    return cls
