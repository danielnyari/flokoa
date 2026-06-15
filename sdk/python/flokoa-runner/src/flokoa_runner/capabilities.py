"""Stage: install_capabilities — capability wheelhouse install + entrypoint load.

Each Capability artifact is delivered to ``/opt/flokoa/capabilities/<name>/``
(wheels + ``manifest.json``; runtime contract §4). The runner enforces the
manifest shape (non-empty ``wheels`` list, valid dependency pins, matching
``contractVersion``), verifies the ``requires`` tuple against its own manifest
(defense in depth — admission already checked), verifies wheelhouse integrity
(streamed sha256 of every listed wheel; unlisted wheels and non-wheel
installables fail bootstrap — this covers the delivery-to-install window),
installs the explicit pin set with ``pip install --no-index --find-links``
(never free resolution), and registers the entrypoint class for
``Agent.from_spec``.
"""

from __future__ import annotations

import hashlib
import importlib
import json
import os
import re
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

# Pinned dependency grammar (runtime contract §4, manifest.json v1):
# PEP 503-ish name, exactly `==`, PEP 440-ish version.
_PIN_PATTERN = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*==[A-Za-z0-9._+!-]+$")

# Non-wheel installables banned from wheelhouses (sdists execute setup code
# at install time; wheels do not).
_NON_WHEEL_SUFFIXES = (".tar.gz", ".zip")

# URL userinfo (user or user:password before the host) in captured pip
# output. BootstrapError details land in logs and status surfaces, so any
# credentialed index/proxy URL pip echoes back must be scrubbed first.
_URL_CREDENTIALS_PATTERN = re.compile(r"(https?://)[^/@\s]+@")


def _redact_url_credentials(text: str) -> str:
    """Replace URL userinfo (``https://user:pass@`` …) with a redaction marker."""
    return _URL_CREDENTIALS_PATTERN.sub(r"\1<redacted>@", text)


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
        manifest = _load_capability_manifest(cap_dir, runner_manifest)
        _verify_requires(cap_dir.name, manifest, runner_manifest)
        _verify_wheelhouse(cap_dir, manifest)
        _pip_install(cap_dir, manifest)
        classes.append(_load_entrypoint(cap_dir.name, manifest))
    return classes


def _load_capability_manifest(cap_dir: Path, runner: RunnerManifest) -> dict[str, Any]:
    manifest_path = cap_dir / "manifest.json"
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        raise BootstrapError(STAGE, "capability manifest missing", capability=cap_dir.name) from None
    except json.JSONDecodeError as exc:
        raise BootstrapError(STAGE, f"capability manifest invalid: {exc}", capability=cap_dir.name) from exc
    if not isinstance(manifest, dict) or "entrypoint" not in manifest:
        raise BootstrapError(STAGE, "capability manifest must declare an entrypoint", capability=cap_dir.name)

    contract = manifest.get("contractVersion")
    if contract is not None and contract != runner.contract_version:
        raise BootstrapError(
            STAGE,
            "capability manifest declares an unsupported contractVersion",
            capability=cap_dir.name,
            manifest_contract=contract,
            runner_contract=runner.contract_version,
        )

    _check_wheels_shape(cap_dir.name, manifest)
    _check_dependency_pins(cap_dir.name, manifest)
    return manifest


def _check_wheels_shape(name: str, manifest: dict[str, Any]) -> None:
    wheels = manifest.get("wheels")
    if not isinstance(wheels, list) or not wheels:
        raise BootstrapError(STAGE, "capability manifest missing wheels list", capability=name)
    for entry in wheels:
        if (
            not isinstance(entry, dict)
            or not isinstance(entry.get("file"), str)
            or not isinstance(entry.get("sha256"), str)
            or not entry["file"]
            or not entry["sha256"]
        ):
            raise BootstrapError(
                STAGE,
                "capability manifest invalid: wheels entries must be objects with file and sha256",
                capability=name,
            )
        if Path(entry["file"]).name != entry["file"]:
            raise BootstrapError(
                STAGE,
                "capability manifest invalid: wheel file entries must be bare filenames",
                capability=name,
                file=entry["file"],
            )


def _check_dependency_pins(name: str, manifest: dict[str, Any]) -> None:
    dependencies = manifest.get("dependencies", [])
    if not isinstance(dependencies, list):
        raise BootstrapError(
            STAGE,
            "capability manifest invalid: dependencies must be a list of name==version pins",
            capability=name,
        )
    for pin in dependencies:
        if not isinstance(pin, str) or not _PIN_PATTERN.match(pin):
            raise BootstrapError(STAGE, "invalid dependency pin in manifest", capability=name, pin=pin)


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


def _verify_wheelhouse(cap_dir: Path, manifest: dict[str, Any]) -> None:
    """Integrity gate between the requires check and pip install.

    The manifest's wheel hashes are the artifact's self-description; the
    digest-pinned image pull already did content addressing, so this guards
    the window after delivery (e.g. the emptyDir copy path) and rejects
    anything pip could be tricked into installing that the manifest never
    declared.
    """
    listed: set[str] = set()
    for entry in manifest["wheels"]:
        file, expected = entry["file"], entry["sha256"]
        listed.add(file)
        wheel_path = cap_dir / file
        try:
            with wheel_path.open("rb") as fh:
                actual = hashlib.file_digest(fh, "sha256").hexdigest()
        except FileNotFoundError:
            raise BootstrapError(
                STAGE,
                "wheel listed in manifest missing from wheelhouse",
                capability=cap_dir.name,
                file=file,
            ) from None
        if actual != expected:
            raise BootstrapError(
                STAGE,
                "wheelhouse integrity check failed",
                capability=cap_dir.name,
                file=file,
                expected_sha256=expected,
                actual_sha256=actual,
            )

    for path in sorted(cap_dir.iterdir()):
        if path.name.endswith(".whl") and path.name not in listed:
            raise BootstrapError(STAGE, "unlisted wheel in wheelhouse", capability=cap_dir.name, file=path.name)
        if path.name.endswith(_NON_WHEEL_SUFFIXES) or path.name == "setup.py":
            raise BootstrapError(STAGE, "non-wheel file in wheelhouse", capability=cap_dir.name, file=path.name)


def _pip_install(cap_dir: Path, manifest: dict[str, Any]) -> None:
    """Install the explicit pin set: the capability plus its pinned closure.

    ``--no-index --find-links`` keeps resolution offline and inside the
    verified wheelhouse; passing every pin explicitly (instead of letting pip
    resolve freely from the directory) means a wheel pip would otherwise pick
    up but that isn't pinned in the manifest can never ride along.
    """
    package = manifest.get("name", cap_dir.name)
    version = manifest.get("version")
    requirement = f"{package}=={version}" if version else package
    pins = [requirement, *manifest.get("dependencies", [])]
    cmd = [
        sys.executable,
        "-m",
        "pip",
        "install",
        "--no-index",
        "--find-links",
        str(cap_dir),
        *pins,
    ]
    result = subprocess.run(cmd, capture_output=True, text=True, check=False)  # noqa: S603
    if result.returncode != 0:
        raise BootstrapError(
            STAGE,
            "wheelhouse install failed",
            capability=cap_dir.name,
            requirement=requirement,
            # Redact before truncating: truncation must never split a
            # credential out of its redactable URL context.
            pip_stderr=_redact_url_credentials(result.stderr.strip())[-2000:],
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
