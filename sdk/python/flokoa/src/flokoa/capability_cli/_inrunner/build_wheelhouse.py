"""Build the capability wheelhouse (runs inside the runner image).

Pipeline:
  1. ``pip wheel --no-deps`` the target — a local project bind-mounted at
     /src, or ``pkg==version`` from PyPI — producing exactly one wheel.
  2. Resolve the full dependency closure with the baseline freeze as
     constraints and ``--only-binary :all:``: any sdist-only dependency is
     refused (wheels-only is the artifact boundary; the error names the
     custom-agent-image escape hatch).
  3. Delete wheels whose name==version matches the baseline — the wheelhouse
     carries exactly the non-baseline closure (full resolution minus what the
     runner image already ships).

Imports: stdlib + ``packaging`` (part of the runner baseline — flokoa-runner
itself depends on it).

Output (in the work dir): wheelhouse/*.whl and wheelhouse-report.json
  {"name", "version", "wheels": [...], "dependencies": ["name==ver", ...]}.
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from pathlib import Path

from packaging.utils import canonicalize_name, parse_wheel_filename

SDIST_GUIDANCE = (
    "a dependency ships no wheel for this environment (sdist-only). Capability artifacts carry "
    "wheels only — system-level or sdist-only dependencies belong in a custom agent image "
    "(Agent spec.runtime.image), the loudly-documented escape hatch."
)


def ensure_pip() -> None:
    """The runner venv is uv-managed and may ship without pip; seed it once."""
    probe = subprocess.run([sys.executable, "-m", "pip", "--version"], capture_output=True, text=True, check=False)
    if probe.returncode != 0:
        subprocess.run([sys.executable, "-m", "ensurepip", "--upgrade"], check=True, capture_output=True)


def run_pip(args: list[str]) -> subprocess.CompletedProcess[str]:
    return subprocess.run([sys.executable, "-m", "pip", *args], capture_output=True, text=True, check=False)


def load_baseline(constraints: Path) -> dict[str, str]:
    baseline: dict[str, str] = {}
    for line in constraints.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "==" not in line:
            continue
        name, _, version = line.partition("==")
        baseline[canonicalize_name(name)] = version
    return baseline


def wheel_name_version(wheel: Path) -> tuple[str, str]:
    name, version, _build, _tags = parse_wheel_filename(wheel.name)
    return canonicalize_name(name), str(version)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--work", required=True, type=Path)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--src", type=Path, help="bind-mounted local project directory")
    source.add_argument("--from-pypi", dest="from_pypi", help="PyPI requirement, pkg or pkg==version")
    args = parser.parse_args()

    ensure_pip()

    wheelhouse = args.work / "wheelhouse"
    wheelhouse.mkdir(parents=True, exist_ok=True)
    constraints = args.work / "constraints.txt"
    baseline = load_baseline(constraints)

    # 1. Build the target wheel (exactly one: --no-deps).
    target_req = str(args.src) if args.src else args.from_pypi
    result = run_pip(["wheel", "--no-deps", "--wheel-dir", str(wheelhouse), target_req])
    if result.returncode != 0:
        print(result.stdout, file=sys.stderr)
        print(f"ERROR: could not build a wheel for {target_req}:\n{result.stderr}", file=sys.stderr)
        return 1
    target_wheels = sorted(wheelhouse.glob("*.whl"))
    if len(target_wheels) != 1:
        print(f"ERROR: expected exactly one target wheel, found {len(target_wheels)}", file=sys.stderr)
        return 1
    name, version = wheel_name_version(target_wheels[0])

    if name in baseline:
        print(
            f"ERROR: {name} is part of the runner baseline (=={baseline[name]}) — "
            "baseline packages cannot be capability artifacts",
            file=sys.stderr,
        )
        return 1

    # 2. Resolve the closure: baseline-constrained, wheels only. The target's
    #    own wheel is found via --find-links; every dependency must be
    #    downloadable as a wheel (--only-binary refuses the sdist fallback).
    result = run_pip(
        [
            "wheel",
            "--wheel-dir",
            str(wheelhouse),
            "--find-links",
            str(wheelhouse),
            "--constraint",
            str(constraints),
            "--only-binary",
            ":all:",
            f"{name}=={version}",
        ]
    )
    if result.returncode != 0:
        print(result.stdout, file=sys.stderr)
        print(
            f"ERROR: dependency closure resolution failed for {name}=={version}.\n"
            f"{result.stderr}\n"
            f"If pip reported a requirement with no matching wheel distribution: {SDIST_GUIDANCE}",
            file=sys.stderr,
        )
        return 1

    # 3. Drop baseline wheels — keep exactly the non-baseline closure.
    dependencies: list[str] = []
    for wheel in sorted(wheelhouse.glob("*.whl")):
        wheel_name, wheel_version = wheel_name_version(wheel)
        if wheel_name == name:
            continue
        baseline_version = baseline.get(wheel_name)
        if baseline_version is not None:
            if wheel_version != baseline_version:
                # Impossible under constraints; guard against silent drift.
                print(
                    f"ERROR: resolved {wheel_name}=={wheel_version} but the baseline pins "
                    f"=={baseline_version} — constraint resolution is inconsistent",
                    file=sys.stderr,
                )
                return 1
            wheel.unlink()
            continue
        dependencies.append(f"{wheel_name}=={wheel_version}")

    # Wheels-only boundary: pip wheel only emits wheels, but guard anyway.
    for path in sorted(wheelhouse.iterdir()):
        if not path.name.endswith(".whl"):
            print(f"ERROR: non-wheel file in wheelhouse: {path.name} — {SDIST_GUIDANCE}", file=sys.stderr)
            return 1

    report = {
        "name": name,
        "version": version,
        "wheels": sorted(p.name for p in wheelhouse.glob("*.whl")),
        "dependencies": sorted(dependencies),
    }
    (args.work / "wheelhouse-report.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    sys.exit(main())
