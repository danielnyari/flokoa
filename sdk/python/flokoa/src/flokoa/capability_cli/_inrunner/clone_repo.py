"""Clone a git repo inside the build container, before the wheelhouse build.

Runs inside the disposable build image. It produces a local checkout that the
existing ``build_wheelhouse.py`` then treats exactly like a ``PATH`` (``--src``)
build — git fetching is build-time only; nothing is fetched at deploy/run time.

Credential handling (§4.3/§8.2) — the token never leaks:
  * The token (if any) arrives ONLY as the ephemeral ``GIT_ASKPASS_TOKEN`` env
    var on this step's ``exec`` (the host set it on the exec subprocess env, so
    it was never in any argv). It is consumed via a ``GIT_ASKPASS`` helper
    script, so it is never interpolated into the remote URL git records.
  * The clone uses the **clean** URL (no ``user:token@``); the recorded
    ``origin`` is reset to the clean URL defensively after cloning.
  * SSH uses the forwarded agent socket with strict host-key checking.
  * Any captured git stderr is redacted before it is printed, so a credentialed
    URL git might echo on error is scrubbed before it reaches a log line.

Output (in the work dir):
  clone/                 — the checkout (build_wheelhouse --src points at clone/[subdir])
  clone-report.json      — {"url" (clean), "ref", "commit", "subdirectory"}
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path

# The clone subdir name inside the work dir.
_CHECKOUT_DIRNAME = "clone"

# A GIT_ASKPASS helper that echoes the token git asks for. git invokes
# GIT_ASKPASS with the prompt as argv[1]; we ignore it and print the token
# from the environment. Username prompts get a throwaway (the token is a PAT,
# used as the password). The token is read from the env, never from argv.
_ASKPASS_HELPER = """\
#!/bin/sh
case "$1" in
  Username*) printf '%s' "x-access-token" ;;
  *) printf '%s' "$GIT_ASKPASS_TOKEN" ;;
esac
"""

# URL userinfo in captured git output — scrub before printing (mirrors the
# CLI-side redactor; this script runs in the container with only stdlib).
_URL_CREDENTIALS_PATTERN = re.compile(r"(https?://)[^/@\s]+@")


def _redact(text: str) -> str:
    return _URL_CREDENTIALS_PATTERN.sub(r"\1<redacted>@", text)


def _run(argv: list[str], *, env: dict[str, str] | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(argv, capture_output=True, text=True, check=False, env=env)


def _write_askpass(work: Path) -> Path:
    helper = work / "git-askpass.sh"
    helper.write_text(_ASKPASS_HELPER, encoding="utf-8")
    helper.chmod(0o700)
    return helper


def _clone_env(scheme: str, askpass: Path) -> dict[str, str]:
    """Build the clone environment: GIT_ASKPASS for https, strict ssh otherwise.

    The token value stays in the ambient ``GIT_ASKPASS_TOKEN`` (injected by the
    host on this exec); the helper reads it. For ssh, strict host-key checking
    is enforced and BatchMode avoids interactive prompts hanging the build.
    """
    env = dict(os.environ)
    # Never let git fall back to a terminal prompt — a missing credential must
    # fail loudly, not block.
    env["GIT_TERMINAL_PROMPT"] = "0"
    if scheme == "https":
        env["GIT_ASKPASS"] = str(askpass)
    else:  # ssh
        env.setdefault(
            "GIT_SSH_COMMAND",
            "ssh -o StrictHostKeyChecking=yes -o BatchMode=yes",
        )
    return env


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--work", required=True, type=Path)
    parser.add_argument("--url", required=True, help="clean clone URL (no git+ prefix, no credentials)")
    parser.add_argument("--scheme", required=True, choices=("https", "ssh"))
    parser.add_argument("--ref", default=None, help="branch, tag, or commit SHA to check out")
    parser.add_argument("--subdirectory", default=None, help="path within the repo to the Python project")
    args = parser.parse_args()

    checkout = args.work / _CHECKOUT_DIRNAME
    askpass = _write_askpass(args.work)
    env = _clone_env(args.scheme, askpass)

    # Shallow-clone (single commit) by default; if a ref is given, clone the
    # default branch then check out the ref (works for branch/tag/sha).
    clone_argv = ["git", "clone", "--quiet", args.url, str(checkout)]
    result = _run(clone_argv, env=env)
    if result.returncode != 0:
        print(
            f"ERROR: git clone of {_redact(args.url)} failed:\n{_redact(result.stderr.strip())[-2000:]}",
            file=sys.stderr,
        )
        return 1

    if args.ref:
        checkout_result = _run(["git", "-C", str(checkout), "checkout", "--quiet", args.ref], env=env)
        if checkout_result.returncode != 0:
            print(
                f"ERROR: git checkout {args.ref!r} failed:\n{_redact(checkout_result.stderr.strip())[-2000:]}",
                file=sys.stderr,
            )
            return 1

    # Defensive: ensure the recorded origin is the clean URL (no credentials).
    _run(["git", "-C", str(checkout), "remote", "set-url", "origin", args.url], env=env)

    rev = _run(["git", "-C", str(checkout), "rev-parse", "HEAD"], env=env)
    if rev.returncode != 0:
        print(f"ERROR: could not resolve HEAD commit:\n{_redact(rev.stderr.strip())[-2000:]}", file=sys.stderr)
        return 1
    commit = rev.stdout.strip()

    if args.subdirectory:
        # Defense in depth: the host already rejects `..` in the subdirectory
        # (git_auth.parse_from_git), but the value crosses the host→container
        # boundary, so re-bound-check it here. Resolve the join and confirm it
        # stays inside the checkout before the is_dir() existence check — a
        # crafted `../` must not point the build at a path outside the clone.
        target = (checkout / args.subdirectory).resolve()
        if not target.is_relative_to(checkout.resolve()):
            print(
                f"ERROR: subdirectory {args.subdirectory!r} escapes the checkout directory",
                file=sys.stderr,
            )
            return 1
        if not target.is_dir():
            print(
                f"ERROR: subdirectory {args.subdirectory!r} does not exist in the checkout",
                file=sys.stderr,
            )
            return 1

    report = {
        "url": args.url,
        "ref": args.ref,
        "commit": commit,
        "subdirectory": args.subdirectory,
    }
    (args.work / "clone-report.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    sys.exit(main())
