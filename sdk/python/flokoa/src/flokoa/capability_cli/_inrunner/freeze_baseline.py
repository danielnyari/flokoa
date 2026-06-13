"""Freeze the runner baseline (runs inside the runner image).

``pip list --format=freeze`` of the runner venv IS the baseline by
construction — more robust than parsing ``runner.lock`` externally and
identical to it by definition. Also copies the runner manifest out so the
host can derive the capability's ``requires`` tuple.

Outputs (in the work dir):
  constraints.txt       — the baseline freeze, used as pip constraints
  runner-manifest.json  — copy of /etc/flokoa/runner-manifest.json
"""

from __future__ import annotations

import argparse
import shutil
import subprocess
import sys
from pathlib import Path

RUNNER_MANIFEST_PATH = "/etc/flokoa/runner-manifest.json"


def ensure_pip() -> None:
    """The runner venv is uv-managed and may ship without pip; seed it once."""
    probe = subprocess.run([sys.executable, "-m", "pip", "--version"], capture_output=True, text=True, check=False)
    if probe.returncode != 0:
        subprocess.run([sys.executable, "-m", "ensurepip", "--upgrade"], check=True, capture_output=True)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--work", required=True, type=Path, help="bind-mounted work directory")
    args = parser.parse_args()

    ensure_pip()

    freeze = subprocess.run(
        [sys.executable, "-m", "pip", "list", "--format=freeze"],
        capture_output=True,
        text=True,
        check=True,
    )
    (args.work / "constraints.txt").write_text(freeze.stdout, encoding="utf-8")

    manifest = Path(RUNNER_MANIFEST_PATH)
    if not manifest.is_file():
        print(
            f"ERROR: {RUNNER_MANIFEST_PATH} not found — the build image is not a flokoa runner image",
            file=sys.stderr,
        )
        return 1
    shutil.copyfile(manifest, args.work / "runner-manifest.json")
    return 0


if __name__ == "__main__":
    sys.exit(main())
