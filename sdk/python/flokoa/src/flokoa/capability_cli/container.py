"""Container execution for the ``flokoa capability`` CLI.

docker/podman detection (honoring the repo-wide ``CONTAINER_TOOL``
convention), build image resolution, and a single disposable container
session the build pipeline's ``_inrunner/`` scripts execute in.

All subprocess invocations are explicit argv arrays — never shell strings.
"""

from __future__ import annotations

import os
import re
import shutil
import subprocess
import uuid
from dataclasses import dataclass, field
from pathlib import Path

from flokoa.capability_cli.errors import CapabilityCliError

# URL userinfo (``user`` or ``user:password`` before the host) in captured
# git/pip output. CapabilityCliError messages and logs surface captured
# stderr, so any credentialed URL the tool echoes back on error must be
# scrubbed first. Mirrors flokoa_runner.capabilities._redact_url_credentials
# (the same posture, replicated CLI-side per the design — §4.3).
_URL_CREDENTIALS_PATTERN = re.compile(r"(https?://)[^/@\s]+@")


def redact_url_credentials(text: str) -> str:
    """Replace URL userinfo (``https://user:tok@`` …) with a redaction marker.

    Used on every captured stderr stream that may echo a credentialed git
    remote (the ephemeral token, or a fallback ``user:token@host`` URL) before
    it reaches a :class:`CapabilityCliError` message or a log line.
    """
    return _URL_CREDENTIALS_PATTERN.sub(r"\1<redacted>@", text)


# The default *build* environment: the capability base image (the pinned runner
# baseline + the build front-end, pip/wheel/setuptools, seeded at image-build
# time). `flokoa capability build` runs its _inrunner/ pipeline inside this by
# default — see flokoa-capability-base/Dockerfile.
DEFAULT_CAPABILITY_BASE_REPOSITORY = "ghcr.io/danielnyari/flokoa-capability-base"

# Retained for back-compat: a bare runner image still works as a build
# environment (the CLI's ensure_pip() fallback seeds pip per build there), but
# the happy path now resolves to the base image above.
DEFAULT_RUNNER_REPOSITORY = "ghcr.io/danielnyari/flokoa-runner"

# The runner release this SDK pairs with by default. Aligned with the
# operator's spec.DefaultRunnerVersion (operator/internal/spec/spec.go) by the
# release process: release.yml derives the base image tag, the runner image
# tag, and this value from the release tag, so don't hand-bump one without the
# others. The base image is tagged to match this runner version.
DEFAULT_RUNNER_VERSION = "0.2.0"

_SUPPORTED_TOOLS = ("docker", "podman")


def detect_container_tool() -> str:
    """Return the container tool executable to use.

    ``CONTAINER_TOOL`` (the repo-wide Makefile convention) wins when set;
    otherwise docker is preferred, then podman.
    """
    configured = os.environ.get("CONTAINER_TOOL")
    if configured:
        if shutil.which(configured) is None:
            raise CapabilityCliError(f"CONTAINER_TOOL={configured} is not on PATH — install it or unset CONTAINER_TOOL")
        return configured
    for tool in _SUPPORTED_TOOLS:
        if shutil.which(tool) is not None:
            return tool
    raise CapabilityCliError(
        "no container tool found — `flokoa capability build` needs docker or podman on PATH "
        "(or CONTAINER_TOOL pointing at one)"
    )


def resolve_build_image(
    *,
    base_image: str | None = None,
    base_version: str | None = None,
    runner_image: str | None = None,
    runner_version: str | None = None,
) -> str:
    """Resolve the build image the ``_inrunner/`` pipeline runs inside.

    The default is the capability **base** image (pinned runner baseline + the
    build front-end). ``--runner-image`` / ``--runner-version`` are retained as
    back-compat aliases for the build environment — an explicit full override
    or version override, respectively — but the default no longer points at a
    bare runner.

    Precedence (highest first):

      1. ``--base-image`` (full override) — or the ``--runner-image`` alias.
      2. ``--base-version`` (composed with the base repository) — or the
         ``--runner-version`` alias.
      3. ``FLOKOA_CAPABILITY_BASE_IMAGE`` env (full override).
      4. base repository (``FLOKOA_CAPABILITY_BASE_REPOSITORY`` env override)
         + ``DEFAULT_RUNNER_VERSION`` (the base image is tagged to the runner
         version).

    A ``--runner-image`` override pointing at a bare runner image still works:
    the in-runner ``ensure_pip()`` fallback seeds pip for that case.
    """
    # 1. Full override — the new flag wins, the legacy alias is honored too.
    full_override = base_image or runner_image
    if full_override:
        return full_override

    repository = os.environ.get("FLOKOA_CAPABILITY_BASE_REPOSITORY") or DEFAULT_CAPABILITY_BASE_REPOSITORY

    # 2. Version-only override, composed with the base repository.
    version_override = base_version or runner_version
    if version_override:
        return f"{repository}:{version_override}"

    # 3. Environment full override.
    env_image = os.environ.get("FLOKOA_CAPABILITY_BASE_IMAGE")
    if env_image:
        return env_image

    # 4. Default: base repository + the SDK-pinned runner version.
    return f"{repository}:{DEFAULT_RUNNER_VERSION}"


def resolve_runner_image(runner_image: str | None = None, runner_version: str | None = None) -> str:
    """Back-compat shim: resolve the build image from the legacy runner flags.

    Retained so existing callers keep working; new code should call
    :func:`resolve_build_image`. The default now resolves to the capability
    base image, not a bare runner.
    """
    return resolve_build_image(runner_image=runner_image, runner_version=runner_version)


@dataclass
class Mount:
    """A bind mount into the build container."""

    host: Path
    container: str
    read_only: bool = True

    def to_arg(self) -> str:
        suffix = ":ro" if self.read_only else ""
        return f"{self.host}:{self.container}{suffix}"


@dataclass
class ContainerSession:
    """One disposable build-image container the whole build executes in.

    The container idles on ``sleep infinity``; each pipeline step is a
    ``<tool> exec``. One session means the venv state carries across steps (the
    smoke install is visible to schema derivation), exactly like a runner pod's
    single venv.

    The default build image is the capability base image, which already ships
    pip + wheel + setuptools (seeded at image-build time), so the happy path
    runs no per-build ``ensurepip``. The in-runner ``ensure_pip()`` fallback
    survives only for someone overriding ``--runner-image`` to a bare runner.

    Runs as root with ``HOME=/tmp`` (matching the fixture ``build.sh``): the
    build container is a throwaway compiler writing to bind mounts the host
    user owns — it is not the pod path.
    """

    tool: str
    image: str
    mounts: list[Mount] = field(default_factory=list)
    _name: str | None = field(default=None, init=False)

    @property
    def name(self) -> str:
        if self._name is None:
            raise CapabilityCliError("container session is not running")
        return self._name

    def start_argv(self, name: str) -> list[str]:
        argv = [
            self.tool,
            "run",
            "--detach",
            "--name",
            name,
            "--user",
            "0",
            "-e",
            "HOME=/tmp",
            "-e",
            "PIP_DISABLE_PIP_VERSION_CHECK=1",
            "-e",
            "PIP_ROOT_USER_ACTION=ignore",
        ]
        for mount in self.mounts:
            argv += ["-v", mount.to_arg()]
        argv += ["--entrypoint", "sleep", self.image, "infinity"]
        return argv

    def exec_argv(self, argv: list[str], *, secret_env_keys: tuple[str, ...] = ()) -> list[str]:
        """Assemble the ``<tool> exec`` argv.

        Secret env keys are passed as bare ``-e KEY`` flags (NOT ``KEY=VALUE``):
        ``docker``/``podman exec -e KEY`` copies ``KEY`` from the *tool
        process's* environment into the container, so the value lives only in
        the subprocess ``env=`` (§4.3/§8.2) — never on the command line, never
        in ``ps``, never in a mount file or the image.
        """
        secret_flags = [flag for key in secret_env_keys for flag in ("-e", key)]
        return [self.tool, "exec", *secret_flags, self.name, *argv]

    def __enter__(self) -> ContainerSession:
        name = f"flokoa-capability-build-{uuid.uuid4().hex[:12]}"
        result = subprocess.run(  # noqa: S603
            self.start_argv(name), capture_output=True, text=True, check=False
        )
        if result.returncode != 0:
            raise CapabilityCliError(
                f"could not start the build container from {self.image}:\n{result.stderr.strip()[-2000:]}"
            )
        self._name = name
        return self

    def exec(
        self,
        argv: list[str],
        *,
        step: str,
        secret_env: dict[str, str] | None = None,
    ) -> subprocess.CompletedProcess[str]:
        """Run one pipeline step inside the session; raise with output on failure.

        ``secret_env`` injects ephemeral credentials (the git token, §4.3) into
        the container step. Each key is forwarded as a bare ``-e KEY`` flag and
        the value is set on this ``exec`` subprocess's environment only — it
        never appears in the argv, in any other step, or in the image. The
        process env is a fresh copy of ``os.environ`` plus the secrets, so the
        value vanishes when the subprocess exits.

        Captured stderr/stdout is routed through the URL-credential redactor
        before it reaches a :class:`CapabilityCliError`, so a credentialed URL
        the tool might echo on error is scrubbed first.
        """
        secret_env = secret_env or {}
        run_env: dict[str, str] | None = None
        if secret_env:
            run_env = {**os.environ, **secret_env}
        result = subprocess.run(  # noqa: S603
            self.exec_argv(argv, secret_env_keys=tuple(secret_env)),
            capture_output=True,
            text=True,
            check=False,
            env=run_env,
        )
        if result.returncode != 0:
            output = redact_url_credentials((result.stdout + "\n" + result.stderr).strip())[-4000:]
            raise CapabilityCliError(f"{step} failed inside {self.image}:\n{output}")
        return result

    def _reclaim_ownership(self) -> None:
        """Chown read-write mount outputs back to the invoking host user.

        The container runs as root, so files its build steps write into
        read-write bind mounts (the wheelhouse, work dir) are root-owned on the
        host. On Linux that leaves the non-root host user unable to write into
        those container-created directories — e.g. the host cannot drop
        ``manifest.json`` into the wheelhouse dir after the build. Reclaim
        ownership before the container is removed. macOS Docker Desktop already
        remaps ownership, so this is a Linux/CI fix and a no-op (or unsupported,
        hence best-effort) elsewhere. Read-only mounts are left untouched.
        """
        getuid = getattr(os, "getuid", None)
        getgid = getattr(os, "getgid", None)
        if getuid is None or getgid is None:  # e.g. Windows — no POSIX ownership
            return
        owner = f"{getuid()}:{getgid()}"
        for mount in self.mounts:
            if mount.read_only:
                continue
            subprocess.run(  # noqa: S603
                self.exec_argv(["chown", "-R", owner, mount.container]),
                capture_output=True,
                text=True,
                check=False,
            )

    def __exit__(self, *exc_info: object) -> None:
        if self._name is not None:
            self._reclaim_ownership()
            subprocess.run(  # noqa: S603
                [self.tool, "rm", "-f", self._name], capture_output=True, text=True, check=False
            )
            self._name = None
