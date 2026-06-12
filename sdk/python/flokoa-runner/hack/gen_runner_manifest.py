"""Generate runner-manifest.json from runner.lock and the generated AgentSpec schema.

The manifest is the runner's machine-readable identity (runtime contract §2):
baked into the image at /etc/flokoa/runner-manifest.json and committed next to
the package so the operator build and CI can cross-check pin/lock/schema
agreement without Python.

Usage: python hack/gen_runner_manifest.py [lockfile] [schema] [output]
"""

from __future__ import annotations

import hashlib
import json
import re
import sys
from pathlib import Path

from flokoa_runner import CONTRACT_VERSION, RUNNER_VERSION
from packaging.markers import Marker

PYTHON_MINOR = "3.13"

BASELINE_PACKAGES = ("httpx", "starlette", "pydantic", "opentelemetry-sdk")

PLATFORM_CAPABILITIES_RESERVED = {
    "flokoa.platform/telemetry": "reserved",
    "flokoa.platform/session-persistence": "reserved",
    "flokoa.platform/budget-guardrail": "reserved",
}

# The runner image is a multi-arch linux container. A pinned package belongs in
# the baseline only if it would actually install for at least one supported
# arch — otherwise platform-gated entries (pywin32, win32 colorama) would seed
# false-positive dependency conflicts at admission. Markers are evaluated
# against these target environments.
_RUNNER_TARGET_ENVS = [
    {
        "os_name": "posix",
        "sys_platform": "linux",
        "platform_system": "Linux",
        "platform_machine": machine,
        "platform_python_implementation": "CPython",
        "implementation_name": "cpython",
        "python_version": PYTHON_MINOR,
        "python_full_version": f"{PYTHON_MINOR}.0",
        "implementation_version": f"{PYTHON_MINOR}.0",
        "platform_release": "",
        "platform_version": "",
    }
    for machine in ("x86_64", "aarch64")
]

# PEP 503 normalization, matching the Go conflict detector
# (internal/domain/capability.NormalizePackageName): lowercase, runs of
# -, _, . collapsed to a single -.
_NAME_SEPARATORS = re.compile(r"[-_.]+")


def normalize_name(name: str) -> str:
    return _NAME_SEPARATORS.sub("-", name).lower()


def _marker_holds_on_runner(marker: str) -> bool:
    parsed = Marker(marker)
    return any(parsed.evaluate(env) for env in _RUNNER_TARGET_ENVS)


def parse_lock_versions(lock_text: str) -> dict[str, str]:
    """Extract `name==version` pins from an exported requirements-format lockfile.

    Packages whose environment marker excludes every supported runner arch are
    dropped: they are not installed in the runner, so they are not part of the
    baseline against which capability dependencies are checked.
    """
    versions: dict[str, str] = {}
    for line in lock_text.splitlines():
        stripped = line.strip()
        m = re.match(r"^([A-Za-z0-9._-]+)==([^ ;\\]+)\s*(?:;\s*(.+?))?\s*$", stripped)
        if not m:
            continue
        name, version, marker = m.group(1), m.group(2), m.group(3)
        if marker and not _marker_holds_on_runner(marker):
            continue
        versions[normalize_name(name)] = version
    return versions


def build_manifest(lock_path: Path, schema_path: Path) -> dict:
    versions = parse_lock_versions(lock_path.read_text(encoding="utf-8"))

    harness = sorted(name for name in versions if "pydantic-ai-harness" in name)
    if harness:
        raise SystemExit(
            f"runner baseline must not contain pydantic-ai-harness packages (found {harness}); "
            "harness capabilities ship only as Capability artifacts (product brief §4)"
        )

    if "pydantic-ai" not in versions:
        raise SystemExit(f"pydantic-ai pin not found in {lock_path}")

    schema_digest = "sha256:" + hashlib.sha256(schema_path.read_bytes()).hexdigest()

    from flokoa_runner.platform_capabilities import PLATFORM_CAPABILITY_TYPES

    platform_capabilities = dict(PLATFORM_CAPABILITIES_RESERVED)
    for name in PLATFORM_CAPABILITY_TYPES:
        platform_capabilities[name] = RUNNER_VERSION

    return {
        "contractVersion": CONTRACT_VERSION,
        "runnerVersion": RUNNER_VERSION,
        "python": PYTHON_MINOR,
        "pydantic-ai": versions["pydantic-ai"],
        "baseline": {name: versions[name] for name in BASELINE_PACKAGES if name in versions},
        "platformCapabilities": platform_capabilities,
        "agentSpecSchemaDigest": schema_digest,
    }


def build_operator_baseline(lock_path: Path) -> dict:
    """The full pinned closure, embedded in the operator for admission-time
    dependency-conflict detection (roadmap 08): a Capability pin colliding
    with any baseline package — not just the headline libraries — must be
    caught before anything deploys.
    """
    versions = parse_lock_versions(lock_path.read_text(encoding="utf-8"))
    return {
        "contractVersion": CONTRACT_VERSION,
        "runnerVersion": RUNNER_VERSION,
        "python": PYTHON_MINOR,
        "pydantic-ai": versions["pydantic-ai"],
        "packages": versions,
    }


def main() -> None:
    pkg_root = Path(__file__).resolve().parents[1]
    lock_path = Path(sys.argv[1]) if len(sys.argv) > 1 else pkg_root / "runner.lock"
    operator_spec_dir = pkg_root.parents[2] / "operator" / "internal" / "spec"
    schema_path = (
        Path(sys.argv[2]) if len(sys.argv) > 2 else operator_spec_dir / "schemas" / f"agentspec-{RUNNER_VERSION}.json"
    )
    out_path = Path(sys.argv[3]) if len(sys.argv) > 3 else pkg_root / "runner-manifest.json"

    manifest = build_manifest(lock_path, schema_path)
    out_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(f"wrote {out_path}")

    baseline = build_operator_baseline(lock_path)
    baseline_path = operator_spec_dir / "baselines" / f"runner-baseline-{RUNNER_VERSION}.json"
    baseline_path.parent.mkdir(parents=True, exist_ok=True)
    baseline_path.write_text(json.dumps(baseline, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(f"wrote {baseline_path}")


if __name__ == "__main__":
    main()
