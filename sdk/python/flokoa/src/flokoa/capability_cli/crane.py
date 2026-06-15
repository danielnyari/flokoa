"""crane shell-out: binary discovery + OCI-layout push with digest capture.

No registry client is vendored into the wheel — crane is discovered on PATH
(``FLOKOA_CRANE`` override) and a missing binary fails the command at startup
with an install one-liner. All invocations are explicit argv arrays.
"""

from __future__ import annotations

import os
import re
import shutil
import subprocess
import tempfile
from pathlib import Path

from flokoa.capability_cli.errors import CapabilityCliError

CRANE_ENV_VAR = "FLOKOA_CRANE"

CRANE_INSTALL_HINT = (
    "install it with: brew install crane  (or: go install github.com/google/go-containerregistry/cmd/crane@latest)"
)

_DIGEST_PATTERN = re.compile(r"^sha256:[a-f0-9]{64}$")


def find_crane() -> str:
    """Locate the crane binary (``FLOKOA_CRANE`` override, then PATH)."""
    configured = os.environ.get(CRANE_ENV_VAR)
    if configured:
        resolved = shutil.which(configured)
        if resolved is None:
            raise CapabilityCliError(f"{CRANE_ENV_VAR}={configured} is not an executable — {CRANE_INSTALL_HINT}")
        return resolved
    resolved = shutil.which("crane")
    if resolved is None:
        raise CapabilityCliError(
            f"crane is not on PATH — `flokoa capability push` needs it to push the artifact; {CRANE_INSTALL_HINT}"
        )
    return resolved


def _run(argv: list[str], *, step: str) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(argv, capture_output=True, text=True, check=False)  # noqa: S603
    if result.returncode != 0:
        output = (result.stdout + "\n" + result.stderr).strip()[-4000:]
        raise CapabilityCliError(f"{step} failed ({' '.join(argv[:2])}):\n{output}")
    return result


def _extract_digest(text: str) -> str | None:
    """Pull the last ``@sha256:<hex>`` digest out of crane output."""
    matches = re.findall(r"@(sha256:[a-f0-9]{64})\b", text)
    return matches[-1] if matches else None


def digest_of(ref: str, *, crane: str) -> str:
    """``crane digest REF`` — the registry's digest for a reference."""
    result = _run([crane, "digest", ref], step="digest lookup")
    digest = result.stdout.strip().splitlines()[-1].strip() if result.stdout.strip() else ""
    if not _DIGEST_PATTERN.match(digest):
        raise CapabilityCliError(f"crane digest returned an unexpected value for {ref}: {digest!r}")
    return digest


def push_oci_archive(tar: Path, ref: str, *, crane: str) -> str:
    """Push an OCI-layout tarball to ``ref``; return the pushed ``sha256:`` digest.

    ``--index`` is always passed: the build emits OCI layouts whose top-level
    index is the artifact (docker buildx writes an index even single-platform,
    and multi-arch layouts contain several images) — pushing the index keeps
    the recorded digest the one agents will pull through.

    Digest capture is deterministic via ``--image-refs`` (crane writes the
    published references to a file); ``crane digest REF`` is the fallback if
    that file is unparsable.
    """
    refs_fd, refs_name = tempfile.mkstemp(prefix="flokoa-crane-refs-", suffix=".txt")
    os.close(refs_fd)
    refs_path = Path(refs_name)
    try:
        _run(
            [crane, "push", str(tar), ref, "--index", "--image-refs", str(refs_path)],
            step="artifact push",
        )
        digest = _extract_digest(refs_path.read_text(encoding="utf-8"))
    finally:
        refs_path.unlink(missing_ok=True)
    if digest is None:
        digest = digest_of(ref, crane=crane)
    return digest
