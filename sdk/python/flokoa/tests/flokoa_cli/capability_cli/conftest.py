"""Shared gating for capability CLI tests.

Integration tests (marker ``integration``) run ONLY when explicitly selected
(``pytest -m integration``) — never as a side effect of a plain test run —
and additionally require a working docker daemon. This keeps the default
package job fast everywhere (per-package CI, workspace-root runs, make test).
"""

from __future__ import annotations

import shutil
import subprocess

import pytest


def docker_available() -> bool:
    if shutil.which("docker") is None:
        return False
    result = subprocess.run(["docker", "info"], capture_output=True, text=True, check=False)
    return result.returncode == 0


def pytest_collection_modifyitems(config: pytest.Config, items: list[pytest.Item]) -> None:
    markexpr = config.getoption("-m", default="") or ""
    explicitly_selected = "integration" in markexpr and "not integration" not in markexpr
    for item in items:
        if item.get_closest_marker("integration") is None:
            continue
        if not explicitly_selected:
            item.add_marker(pytest.mark.skip(reason="integration test; opt in with -m integration"))
        elif not docker_available():
            item.add_marker(pytest.mark.skip(reason="integration test requires a working docker daemon"))
