"""cosign shell-out: binary discovery + digest signing.

The CLI signs (key-based or keyless via ambient OIDC); the operator verifies
with sigstore-go. cosign is discovered on PATH (``FLOKOA_COSIGN`` override)
and a missing binary fails ``--sign`` at startup with an install one-liner.
All invocations are explicit argv arrays.
"""

from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

from flokoa.capability_cli.errors import CapabilityCliError

COSIGN_ENV_VAR = "FLOKOA_COSIGN"

COSIGN_INSTALL_HINT = (
    "install it with: brew install cosign  (or: go install github.com/sigstore/cosign/v2/cmd/cosign@latest)"
)


def find_cosign() -> str:
    """Locate the cosign binary (``FLOKOA_COSIGN`` override, then PATH)."""
    configured = os.environ.get(COSIGN_ENV_VAR)
    if configured:
        resolved = shutil.which(configured)
        if resolved is None:
            raise CapabilityCliError(f"{COSIGN_ENV_VAR}={configured} is not an executable — {COSIGN_INSTALL_HINT}")
        return resolved
    resolved = shutil.which("cosign")
    if resolved is None:
        raise CapabilityCliError(f"--sign needs cosign on PATH — {COSIGN_INSTALL_HINT}")
    return resolved


def sign_digest(pinned_ref: str, *, cosign: str, key: Path | None = None) -> None:
    """``cosign sign`` the pushed ``REF@sha256:...`` reference.

    Key-based when ``key`` is given; otherwise keyless via ambient OIDC
    (workload identity in CI, browser flow interactively — cosign drives it).
    ``--yes`` skips cosign's transparency-log upload confirmation, which would
    otherwise dead-lock non-interactive runs.
    """
    argv = [cosign, "sign", "--yes"]
    if key is not None:
        argv += ["--key", str(key)]
    argv.append(pinned_ref)
    result = subprocess.run(argv, capture_output=True, text=True, check=False)  # noqa: S603
    if result.returncode != 0:
        output = (result.stdout + "\n" + result.stderr).strip()[-4000:]
        mode = "key-based" if key is not None else "keyless (ambient OIDC)"
        raise CapabilityCliError(f"cosign {mode} signing of {pinned_ref} failed:\n{output}")
