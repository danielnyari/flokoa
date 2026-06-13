"""Container execution for the ``flokoa capability`` CLI.

docker/podman detection (honoring the repo-wide ``CONTAINER_TOOL``
convention), runner image resolution, and a single disposable container
session the build pipeline's ``_inrunner/`` scripts execute in.

All subprocess invocations are explicit argv arrays — never shell strings.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import uuid
from dataclasses import dataclass, field
from pathlib import Path

from flokoa.capability_cli.errors import CapabilityCliError

DEFAULT_RUNNER_REPOSITORY = "ghcr.io/danielnyari/flokoa-runner"

# The runner release this SDK pairs with by default. Aligned with the
# operator's spec.DefaultRunnerVersion (operator/internal/spec/spec.go) by the
# release process: release.yml derives both — and the runner image tag — from
# the release tag, so don't hand-bump one without the other.
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


def resolve_runner_image(runner_image: str | None = None, runner_version: str | None = None) -> str:
    """Resolve the pinned runner image the build runs inside.

    Precedence: ``--runner-image`` > ``--runner-version`` (composed with the
    repository) > ``FLOKOA_RUNNER_IMAGE`` env > default repository
    (``FLOKOA_RUNNER_REPOSITORY`` env override) + ``DEFAULT_RUNNER_VERSION``.
    """
    if runner_image:
        return runner_image
    repository = os.environ.get("FLOKOA_RUNNER_REPOSITORY") or DEFAULT_RUNNER_REPOSITORY
    if runner_version:
        return f"{repository}:{runner_version}"
    env_image = os.environ.get("FLOKOA_RUNNER_IMAGE")
    if env_image:
        return env_image
    return f"{repository}:{DEFAULT_RUNNER_VERSION}"


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
    """One disposable runner-image container the whole build executes in.

    The container idles on ``sleep infinity``; each pipeline step is a
    ``<tool> exec``. One session means the venv state carries across steps
    (ensurepip runs once; the smoke install is visible to schema derivation),
    exactly like a runner pod's single venv.

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

    def exec_argv(self, argv: list[str]) -> list[str]:
        return [self.tool, "exec", self.name, *argv]

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

    def exec(self, argv: list[str], *, step: str) -> subprocess.CompletedProcess[str]:
        """Run one pipeline step inside the session; raise with output on failure."""
        result = subprocess.run(  # noqa: S603
            self.exec_argv(argv), capture_output=True, text=True, check=False
        )
        if result.returncode != 0:
            output = (result.stdout + "\n" + result.stderr).strip()[-4000:]
            raise CapabilityCliError(f"{step} failed inside {self.image}:\n{output}")
        return result

    def __exit__(self, *exc_info: object) -> None:
        if self._name is not None:
            subprocess.run(  # noqa: S603
                [self.tool, "rm", "-f", self._name], capture_output=True, text=True, check=False
            )
            self._name = None
