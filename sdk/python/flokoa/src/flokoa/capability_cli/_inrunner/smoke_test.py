"""Smoke-test the wheelhouse (runs inside the runner image).

Installs the explicit pin set exactly the way the runner does
(``pip install --no-index --find-links``), then — when the entrypoint is
known — imports it and instantiates with ``{}`` (class defaults). A
capability that can't import never gets an artifact; instantiation that
fails only because the capability requires config is a warning, not a
refusal.

Output (in the work dir): smoke-report.json
  {"installed", "imported", "instantiated", "warning"}.
"""

from __future__ import annotations

import argparse
import importlib
import json
import subprocess
import sys
import traceback
from pathlib import Path
from typing import Any


def ensure_pip() -> None:
    """The runner venv is uv-managed and may ship without pip; seed it once."""
    probe = subprocess.run([sys.executable, "-m", "pip", "--version"], capture_output=True, text=True, check=False)
    if probe.returncode != 0:
        subprocess.run([sys.executable, "-m", "ensurepip", "--upgrade"], check=True, capture_output=True)


def install_pin_set(wheelhouse: Path, name: str, version: str, dependencies: list[str]) -> None:
    """Mirror flokoa_runner.capabilities._pip_install: explicit pins, offline."""
    pins = [f"{name}=={version}", *dependencies]
    result = subprocess.run(
        [sys.executable, "-m", "pip", "install", "--no-index", "--find-links", str(wheelhouse), *pins],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        print(f"ERROR: wheelhouse install failed:\n{result.stderr.strip()[-2000:]}", file=sys.stderr)
        raise SystemExit(1)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--work", required=True, type=Path)
    parser.add_argument("--name", required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--dependency", action="append", default=[], help="pinned dependency (repeatable)")
    parser.add_argument("--entrypoint", default=None, help="module:attr to import and instantiate")
    args = parser.parse_args()

    ensure_pip()
    install_pin_set(args.work / "wheelhouse", args.name, args.version, args.dependency)

    report: dict[str, Any] = {"installed": True, "imported": None, "instantiated": False, "warning": None}

    if args.entrypoint:
        module_name, _, attr = args.entrypoint.partition(":")
        if not module_name or not attr:
            print(f"ERROR: entrypoint must be module:attr, got {args.entrypoint!r}", file=sys.stderr)
            return 1
        try:
            cls = getattr(importlib.import_module(module_name), attr)
        except Exception:
            print(
                f"ERROR: capability entrypoint failed to import: {args.entrypoint}\n{traceback.format_exc()}",
                file=sys.stderr,
            )
            return 1
        report["imported"] = args.entrypoint

        # Instantiate with {} — class defaults stand in for schema defaults.
        try:
            from_spec = getattr(cls, "from_spec", cls)
            from_spec()
            report["instantiated"] = True
        except TypeError as exc:
            # Missing required config is legitimate: warn, don't refuse.
            report["warning"] = f"instantiation with defaults skipped (requires config): {exc}"
        except Exception:
            print(
                f"ERROR: capability entrypoint failed to instantiate with defaults: "
                f"{args.entrypoint}\n{traceback.format_exc()}",
                file=sys.stderr,
            )
            return 1

    (args.work / "smoke-report.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    sys.exit(main())
