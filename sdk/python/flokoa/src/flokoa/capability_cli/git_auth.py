"""Host-side git auth resolution for ``flokoa capability build --from-git``.

Credential resolution happens **on the host** (it has the user's ambient
creds); only the minimum is injected into the disposable build container, and
only ever as an ephemeral env var on the ``exec`` (never argv, never a mount
file, never the image — §4.3/§8.2).

Precedence is LOCKED by requirements (ambient first, token fallback):

  git+https
    1. ambient: ``git credential fill`` for the host, else ``gh auth token``
    2. token fallback: ``GITHUB_TOKEN`` then ``GH_TOKEN`` from the environment
    3. public repos need no credential — auth only matters on a 401/403

  git+ssh
    the host SSH agent: the agent *socket* (``$SSH_AUTH_SOCK``) is mounted into
    the build container (no private-key material is copied), with strict
    host-key checking. No token is involved.

Everything here is subprocess-driven (``git``/``gh``) so it is fully mockable
in unit tests; no real network or tokens are needed to exercise the
precedence ladder.
"""

from __future__ import annotations

import os
import re
import shutil
import subprocess
from dataclasses import dataclass

from flokoa.capability_cli.container import redact_url_credentials
from flokoa.capability_cli.errors import CapabilityCliError

# --from-git accepts uv/pip-style VCS URLs and nothing else: a git+https or
# git+ssh scheme, a host, a repo path, an optional @ref, and an optional
# #subdirectory=… fragment. The shape is validated on the HOST before the
# value reaches the build container (mirroring _FROM_PYPI_PATTERN's posture):
# extras, environment markers, smuggled pip options, and a bare https:// (no
# git+) are all rejected. Userinfo (user:pass@) is forbidden — credentials
# never travel in the typed URL, they are resolved separately (§4.3).
_FROM_GIT_PATTERN = re.compile(
    r"""
    ^git\+
    (?P<scheme>https|ssh)://
    (?P<userhost>[A-Za-z0-9._~%-]+@)?          # optional ssh user (git@); no password
    (?P<host>[A-Za-z0-9.-]+)                   # host
    (?::(?P<port>\d+))?                         # optional :port (captured so it survives URL reconstruction)
    /(?P<path>[A-Za-z0-9._~/-]+?)               # repo path (non-greedy)
    (?:\.git)?
    (?:@(?P<ref>[A-Za-z0-9._/-]+))?            # optional @ref (branch/tag/sha)
    (?:\#subdirectory=(?P<subdirectory>[A-Za-z0-9._/-]+))?  # optional subdir
    $
    """,
    re.VERBOSE,
)

#: An ``ssh://user:pass@host`` userinfo with a colon (i.e. a password) — never
#: accepted; the grammar above only allows a bare ``user@`` for ssh.
_USERINFO_WITH_PASSWORD = re.compile(r"^[^/@]+:[^/@]+@")

_GITHUB_TOKEN_VARS = ("GITHUB_TOKEN", "GH_TOKEN")


@dataclass(frozen=True)
class ParsedGitSource:
    """A validated ``--from-git`` value, decomposed for the clone step.

    ``url`` is the **clean** clone URL (scheme + host + path, no ``git+``
    prefix, no ref, no fragment, no credentials) — exactly what the recorded
    provenance and the container clone use.
    """

    scheme: str  # "https" | "ssh"
    host: str
    url: str
    ref: str | None = None
    subdirectory: str | None = None
    #: The non-standard ``:port`` (``None`` for the default 80/443). git's
    #: credential helpers key creds per ``host[:port]``, so the credential
    #: lookup must send ``host=HOST:PORT`` when a port is present.
    port: str | None = None


def parse_from_git(value: str) -> ParsedGitSource:
    """Validate the ``--from-git`` grammar and decompose it.

    Raises :class:`CapabilityCliError` (a user-facing usage failure) with
    guidance when the shape is wrong — before anything reaches the container.
    """
    match = _FROM_GIT_PATTERN.match(value)
    if match is None:
        raise CapabilityCliError(
            f"--from-git must be git+https://HOST/ORG/REPO[@ref][#subdirectory=DIR] or "
            f"git+ssh://git@HOST/ORG/REPO[…], got {value!r} "
            "(a bare https:// without git+, extras, environment markers, pip options, and "
            "embedded credentials are not accepted)"
        )
    scheme = match.group("scheme")
    host = match.group("host")
    port = match.group("port")  # None for standard ports (80/443)
    userhost = match.group("userhost") or ""
    if _USERINFO_WITH_PASSWORD.match(userhost):  # pragma: no cover — grammar already forbids it
        raise CapabilityCliError("--from-git must not embed credentials in the URL; auth is resolved separately")
    # The grammar accepts a bare `user@` for BOTH schemes, but for git+https the
    # username is silently dropped (credentials are resolved separately), so a
    # `user@` there is misleading — reject it rather than honor a username that
    # has no effect. ssh keeps its `git@` user.
    if scheme == "https" and userhost:
        raise CapabilityCliError(
            "--from-git https URLs must not include a 'user@' prefix; credentials are resolved "
            "separately — use git+ssh://git@HOST/... if you need an ssh user"
        )
    path = match.group("path")
    ref = match.group("ref")
    subdirectory = match.group("subdirectory")
    # The grammar allows `.` and `/` in the @ref and #subdirectory= groups, which
    # admits `..` path components. A `..` in the subdirectory can escape the
    # checkout when joined to the clone dir in the container; a `..` in the ref
    # is meaningless and a hand-smuggling smell. Reject both on the HOST, before
    # anything reaches the container (the clone step bound-checks defensively too).
    if ref is not None and ".." in ref.split("/"):
        raise CapabilityCliError(f"--from-git @ref must not contain a '..' path component, got {ref!r}")
    if subdirectory is not None and ".." in subdirectory.split("/"):
        raise CapabilityCliError(
            f"--from-git #subdirectory= must not contain a '..' path component (it would escape the "
            f"checkout), got {subdirectory!r}"
        )
    user_prefix = userhost if scheme == "ssh" else ""
    host_port = f"{host}:{port}" if port else host
    clean_url = f"{scheme}://{user_prefix}{host_port}/{path}"
    return ParsedGitSource(
        scheme=scheme,
        host=host,
        url=clean_url,
        ref=ref,
        subdirectory=subdirectory,
        port=port,
    )


@dataclass
class GitAuth:
    """The resolved credential plan handed to the clone step.

    Exactly one of ``token`` (https) or ``ssh_auth_sock`` (ssh) is set for a
    private repo; both are ``None`` for a public https clone that needs nothing.
    The ``source`` string is human-readable for the up-front error/logging and
    carries no secret.
    """

    token: str | None = None
    ssh_auth_sock: str | None = None
    source: str = "none"


def _run(argv: list[str], *, input_text: str | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(  # noqa: S603
        argv, input=input_text, capture_output=True, text=True, check=False
    )


def _git_credential_fill(host: str) -> str | None:
    """Ask the host's git credential helper for a password for ``host``.

    ``git credential fill`` reads a key=value request on stdin and prints the
    resolved ``password=…`` (the token) on stdout. Returns ``None`` when git is
    absent, no helper is configured, or no credential is produced.

    ``host`` carries the ``:port`` for a non-standard port (git keys creds per
    ``host[:port]``, and its credential ``host`` attribute includes the port).
    """
    if shutil.which("git") is None:
        return None
    request = f"protocol=https\nhost={host}\n\n"
    result = _run(["git", "credential", "fill"], input_text=request)
    if result.returncode != 0:
        return None
    for line in result.stdout.splitlines():
        key, _, value = line.partition("=")
        if key == "password" and value:
            return value
    return None


def _gh_auth_token() -> str | None:
    """``gh auth token`` — the GitHub CLI's stored token, if gh is installed."""
    if shutil.which("gh") is None:
        return None
    result = _run(["gh", "auth", "token"])
    if result.returncode != 0:
        return None
    token = result.stdout.strip()
    return token or None


def _env_token() -> tuple[str, str] | None:
    """``GITHUB_TOKEN`` then ``GH_TOKEN`` from the environment."""
    for var in _GITHUB_TOKEN_VARS:
        value = os.environ.get(var)
        if value:
            return value, var
    return None


def resolve_https_auth(host: str, port: str | None = None) -> GitAuth:
    """Resolve an https credential, ambient first then env-var fallback.

    Returns ``GitAuth(token=None, source="none")`` when nothing resolves — a
    public-repo clone proceeds with no credential; auth only matters if the
    clone hits a 401/403, in which case :func:`auth_failure_message` names the
    precedence the user can fix.

    ``port`` (when set) is appended to the host for the credential lookup so
    git's per-``host:port`` credential store resolves creds for self-hosted
    instances on a non-standard port.
    """
    lookup_host = f"{host}:{port}" if port else host
    token = _git_credential_fill(lookup_host)
    if token:
        return GitAuth(token=token, source="ambient git credential helper")
    token = _gh_auth_token()
    if token:
        return GitAuth(token=token, source="ambient gh auth token")
    env = _env_token()
    if env is not None:
        return GitAuth(token=env[0], source=f"${env[1]}")
    return GitAuth(token=None, source="none")


def resolve_ssh_auth() -> GitAuth:
    """Resolve ssh auth: forward the host SSH agent socket if one is running.

    The agent *socket* is forwarded (not private keys), so no key material is
    copied into the container. A missing ``$SSH_AUTH_SOCK`` is not fatal here —
    a public repo clones fine; a private one will fail in the clone step with a
    clear ssh error, which the redactor leaves untouched (it carries no token).
    """
    sock = os.environ.get("SSH_AUTH_SOCK")
    if sock:
        return GitAuth(ssh_auth_sock=sock, source="host SSH agent ($SSH_AUTH_SOCK)")
    return GitAuth(ssh_auth_sock=None, source="none")


def resolve_auth(parsed: ParsedGitSource) -> GitAuth:
    """Resolve the credential plan for a parsed ``--from-git`` source."""
    if parsed.scheme == "ssh":
        return resolve_ssh_auth()
    return resolve_https_auth(parsed.host, parsed.port)


def auth_failure_message(parsed: ParsedGitSource) -> str:
    """The up-front error naming the precedence when a private clone needs auth."""
    if parsed.scheme == "ssh":
        return (
            f"cloning {redact_url_credentials(parsed.url)} over ssh needs a running SSH agent "
            "($SSH_AUTH_SOCK) with a key authorized for the repo — start ssh-agent and ssh-add "
            "your key, or use a git+https URL with a credential helper / GITHUB_TOKEN"
        )
    return (
        f"cloning {redact_url_credentials(parsed.url)} requires credentials and none resolved — "
        "tried the ambient git credential helper and gh auth token, then $GITHUB_TOKEN / $GH_TOKEN; "
        "set one to build from a private repo"
    )
