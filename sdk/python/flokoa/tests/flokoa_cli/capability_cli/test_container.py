"""container.py: tool detection, runner image resolution, session argv assembly."""

from __future__ import annotations

from pathlib import Path
from unittest import mock

import pytest

from flokoa.capability_cli import container
from flokoa.capability_cli.errors import CapabilityCliError


class TestDetectContainerTool:
    def test_container_tool_env_wins(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("CONTAINER_TOOL", "podman")
        with mock.patch.object(container.shutil, "which", return_value="/usr/bin/podman"):
            assert container.detect_container_tool() == "podman"

    def test_container_tool_env_missing_binary(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("CONTAINER_TOOL", "podman")
        with (
            mock.patch.object(container.shutil, "which", return_value=None),
            pytest.raises(CapabilityCliError, match="CONTAINER_TOOL=podman is not on PATH"),
        ):
            container.detect_container_tool()

    def test_docker_preferred_over_podman(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("CONTAINER_TOOL", raising=False)
        with mock.patch.object(container.shutil, "which", side_effect=lambda tool: f"/usr/bin/{tool}"):
            assert container.detect_container_tool() == "docker"

    def test_podman_fallback(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("CONTAINER_TOOL", raising=False)
        with mock.patch.object(
            container.shutil, "which", side_effect=lambda tool: "/usr/bin/podman" if tool == "podman" else None
        ):
            assert container.detect_container_tool() == "podman"

    def test_no_tool_found(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("CONTAINER_TOOL", raising=False)
        with (
            mock.patch.object(container.shutil, "which", return_value=None),
            pytest.raises(CapabilityCliError, match="no container tool found"),
        ):
            container.detect_container_tool()


class TestResolveRunnerImage:
    @pytest.fixture(autouse=True)
    def _clean_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("FLOKOA_RUNNER_IMAGE", raising=False)
        monkeypatch.delenv("FLOKOA_RUNNER_REPOSITORY", raising=False)

    def test_default(self) -> None:
        assert container.resolve_runner_image() == (
            f"{container.DEFAULT_RUNNER_REPOSITORY}:{container.DEFAULT_RUNNER_VERSION}"
        )

    def test_runner_image_flag_wins(self) -> None:
        assert container.resolve_runner_image("example.com/runner:dev", "9.9.9") == "example.com/runner:dev"

    def test_runner_version_composes_with_repository(self) -> None:
        assert container.resolve_runner_image(None, "0.3.0") == f"{container.DEFAULT_RUNNER_REPOSITORY}:0.3.0"

    def test_env_image_used_when_no_flags(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("FLOKOA_RUNNER_IMAGE", "local/runner:ci")
        assert container.resolve_runner_image() == "local/runner:ci"

    def test_runner_version_flag_beats_env_image(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("FLOKOA_RUNNER_IMAGE", "local/runner:ci")
        assert container.resolve_runner_image(None, "0.3.0") == f"{container.DEFAULT_RUNNER_REPOSITORY}:0.3.0"

    def test_env_repository_override(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("FLOKOA_RUNNER_REPOSITORY", "registry.local/flokoa-runner")
        assert container.resolve_runner_image() == f"registry.local/flokoa-runner:{container.DEFAULT_RUNNER_VERSION}"

    def test_default_version_matches_operator_pin(self) -> None:
        """DEFAULT_RUNNER_VERSION must track operator spec.DefaultRunnerVersion.

        Both move together via release.yml; this guards local drift when the
        operator checkout is present (repo CI), and skips in isolated installs.
        """
        spec_go = Path(__file__).parents[6] / "operator" / "internal" / "spec" / "spec.go"
        if not spec_go.is_file():
            pytest.skip("operator checkout not available")
        import re

        match = re.search(r'var DefaultRunnerVersion = "([^"]+)"', spec_go.read_text(encoding="utf-8"))
        assert match is not None, "could not read DefaultRunnerVersion from spec.go"
        assert match.group(1) == container.DEFAULT_RUNNER_VERSION


class TestContainerSession:
    def _session(self) -> container.ContainerSession:
        return container.ContainerSession(
            tool="docker",
            image="ghcr.io/danielnyari/flokoa-runner:0.2.0",
            mounts=[
                container.Mount(Path("/host/inrunner"), "/flokoa-inrunner"),
                container.Mount(Path("/host/work"), "/work", read_only=False),
            ],
        )

    def test_start_argv_shape(self) -> None:
        argv = self._session().start_argv("flokoa-capability-build-abc")
        assert argv[:5] == ["docker", "run", "--detach", "--name", "flokoa-capability-build-abc"]
        assert argv[5:7] == ["--user", "0"]
        assert "-v" in argv
        assert "/host/inrunner:/flokoa-inrunner:ro" in argv
        assert "/host/work:/work" in argv
        # sleep-forever session: every step is an exec.
        assert argv[-3:] == ["sleep", "ghcr.io/danielnyari/flokoa-runner:0.2.0", "infinity"]
        assert "--entrypoint" in argv

    def test_exec_runs_argv_array(self) -> None:
        session = self._session()
        completed = mock.Mock(returncode=0, stdout="", stderr="")
        with mock.patch.object(container.subprocess, "run", return_value=completed) as run:
            with session:
                session.exec(["python", "/flokoa-inrunner/freeze_baseline.py", "--work", "/work"], step="freeze")
        start_argv = run.call_args_list[0].args[0]
        exec_argv = run.call_args_list[1].args[0]
        # rm is the last call; an ownership-reclaim chown may run just before it.
        rm_argv = run.call_args_list[-1].args[0]
        assert start_argv[:2] == ["docker", "run"]
        assert exec_argv[:2] == ["docker", "exec"]
        assert exec_argv[3:] == ["python", "/flokoa-inrunner/freeze_baseline.py", "--work", "/work"]
        assert rm_argv[:3] == ["docker", "rm", "-f"]
        # No shell strings anywhere — every call is an argv list.
        assert all(isinstance(call.args[0], list) for call in run.call_args_list)

    def test_exit_reclaims_rw_mount_ownership(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """On exit the session chowns its read-write mounts back to the host user.

        The build container runs as root, so outputs it writes into the work
        mount are root-owned; without reclaiming them the non-root host (Linux
        CI) cannot write the manifest into the container-created wheelhouse dir.
        Read-only mounts, which the container never writes to, are left alone.
        """
        monkeypatch.setattr(container.os, "getuid", lambda: 4242)
        monkeypatch.setattr(container.os, "getgid", lambda: 4243)
        session = self._session()
        completed = mock.Mock(returncode=0, stdout="", stderr="")
        with mock.patch.object(container.subprocess, "run", return_value=completed) as run:
            with session:
                pass
        chown_calls = [c.args[0] for c in run.call_args_list if "chown" in c.args[0]]
        assert len(chown_calls) == 1, "only the single read-write mount is reclaimed"
        assert chown_calls[0][3:] == ["chown", "-R", "4242:4243", "/work"]
        assert all("/flokoa-inrunner" not in argv for argv in chown_calls), "read-only mount is never chowned"

    def test_exit_skips_reclaim_without_posix_ownership(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Where the OS has no uid/gid (e.g. Windows), reclaim is skipped, not crashed."""
        monkeypatch.delattr(container.os, "getuid", raising=False)
        session = self._session()
        completed = mock.Mock(returncode=0, stdout="", stderr="")
        with mock.patch.object(container.subprocess, "run", return_value=completed) as run:
            with session:
                pass
        assert not [c.args[0] for c in run.call_args_list if "chown" in c.args[0]]

    def test_exec_failure_raises_with_output(self) -> None:
        session = self._session()
        ok = mock.Mock(returncode=0, stdout="", stderr="")
        boom = mock.Mock(returncode=1, stdout="partial", stderr="pip exploded")
        # start, failing exec, then __exit__'s reclaim-chown + rm.
        with mock.patch.object(container.subprocess, "run", side_effect=[ok, boom, ok, ok]):
            with session, pytest.raises(CapabilityCliError, match=r"(?s)wheelhouse build failed.*pip exploded"):
                session.exec(["python", "x.py"], step="wheelhouse build")

    def test_container_removed_even_on_failure(self) -> None:
        session = self._session()
        ok = mock.Mock(returncode=0, stdout="", stderr="")
        boom = mock.Mock(returncode=1, stdout="", stderr="nope")
        # start, failing exec, then __exit__'s reclaim-chown + rm; rm is last.
        with mock.patch.object(container.subprocess, "run", side_effect=[ok, boom, ok, ok]) as run:
            with pytest.raises(CapabilityCliError):
                with session:
                    session.exec(["python", "x.py"], step="step")
        assert run.call_args_list[-1].args[0][:3] == ["docker", "rm", "-f"]

    def test_start_failure(self) -> None:
        session = self._session()
        boom = mock.Mock(returncode=125, stdout="", stderr="pull access denied")
        with (
            mock.patch.object(container.subprocess, "run", return_value=boom),
            pytest.raises(CapabilityCliError, match=r"(?s)could not start the build container.*pull access denied"),
        ):
            session.__enter__()
