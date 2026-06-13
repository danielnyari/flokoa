"""kubectl shell-out: CR apply (``push --apply``) and in-cluster listing.

Two postures, matching how each caller treats the cluster:

* ``push --apply`` asked for a cluster write — a missing binary or a failed
  apply is a hard error (with an install one-liner).
* ``search``/``list`` merely enrich results — a missing binary, unreachable
  cluster, or uninstalled CRD degrades to a skip with a clear reason.

All invocations are explicit argv arrays.
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from flokoa.capability_cli.errors import CapabilityCliError

KUBECTL_ENV_VAR = "FLOKOA_KUBECTL"

KUBECTL_INSTALL_HINT = "install it with: brew install kubectl  (or see https://kubernetes.io/docs/tasks/tools/)"


def find_kubectl() -> str | None:
    """Locate kubectl (``FLOKOA_KUBECTL`` override, then PATH); None if absent."""
    configured = os.environ.get(KUBECTL_ENV_VAR)
    if configured:
        return shutil.which(configured)
    return shutil.which("kubectl")


def require_kubectl() -> str:
    """kubectl for an explicitly requested cluster write (``--apply``)."""
    resolved = find_kubectl()
    if resolved is None:
        raise CapabilityCliError(f"--apply needs kubectl on PATH — {KUBECTL_INSTALL_HINT}")
    return resolved


def apply_manifest(path: Path, *, kubectl: str, namespace: str | None = None) -> str:
    """``kubectl apply -f path`` (optionally namespaced); returns kubectl's output."""
    argv = [kubectl, "apply", "-f", str(path)]
    if namespace:
        argv += ["--namespace", namespace]
    result = subprocess.run(argv, capture_output=True, text=True, check=False)  # noqa: S603
    if result.returncode != 0:
        output = (result.stdout + "\n" + result.stderr).strip()[-4000:]
        raise CapabilityCliError(f"kubectl apply of {path.name} failed:\n{output}")
    return result.stdout.strip()


@dataclass
class ClusterCapabilities:
    """In-cluster Capability CRs, or the reason the lookup was skipped."""

    items: list[dict[str, Any]] = field(default_factory=list)
    skipped_reason: str | None = None


def list_capabilities() -> ClusterCapabilities:
    """``kubectl get capabilities -A -o json`` with graceful degradation."""
    kubectl = find_kubectl()
    if kubectl is None:
        return ClusterCapabilities(skipped_reason="kubectl is not on PATH")
    argv = [kubectl, "get", "capabilities", "-A", "-o", "json"]
    result = subprocess.run(argv, capture_output=True, text=True, check=False)  # noqa: S603
    if result.returncode != 0:
        detail = result.stderr.strip().splitlines()
        reason = detail[-1] if detail else f"kubectl exited {result.returncode}"
        return ClusterCapabilities(skipped_reason=f"cluster lookup failed: {reason}")
    try:
        payload = json.loads(result.stdout)
    except json.JSONDecodeError:
        return ClusterCapabilities(skipped_reason="cluster lookup returned unparsable JSON")
    items = payload.get("items")
    if not isinstance(items, list):
        return ClusterCapabilities(skipped_reason="cluster lookup returned an unexpected shape")
    return ClusterCapabilities(items=items)
