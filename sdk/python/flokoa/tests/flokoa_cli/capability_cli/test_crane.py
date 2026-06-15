"""crane.py: binary preflight and push digest capture (mocked subprocess)."""

from __future__ import annotations

from pathlib import Path
from unittest import mock

import pytest

from flokoa.capability_cli import crane
from flokoa.capability_cli.errors import CapabilityCliError

DIGEST = "sha256:" + "a" * 64


class TestFindCrane:
    def test_env_override_wins(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("FLOKOA_CRANE", "/opt/tools/crane")
        with mock.patch.object(crane.shutil, "which", return_value="/opt/tools/crane"):
            assert crane.find_crane() == "/opt/tools/crane"

    def test_env_override_missing_binary(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("FLOKOA_CRANE", "/opt/tools/crane")
        with (
            mock.patch.object(crane.shutil, "which", return_value=None),
            pytest.raises(CapabilityCliError, match="FLOKOA_CRANE=/opt/tools/crane is not an executable"),
        ):
            crane.find_crane()

    def test_path_lookup(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("FLOKOA_CRANE", raising=False)
        with mock.patch.object(crane.shutil, "which", return_value="/usr/local/bin/crane"):
            assert crane.find_crane() == "/usr/local/bin/crane"

    def test_missing_binary_names_install_one_liner(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("FLOKOA_CRANE", raising=False)
        with (
            mock.patch.object(crane.shutil, "which", return_value=None),
            pytest.raises(CapabilityCliError, match=r"crane is not on PATH.*go-containerregistry/cmd/crane@latest"),
        ):
            crane.find_crane()


class TestPushOciArchive:
    def _fake_run(self, *, refs_content: str | None, digest_stdout: str = DIGEST + "\n"):
        """A subprocess.run stand-in: `crane push` writes the --image-refs file."""
        calls: list[list[str]] = []

        def run(argv: list[str], **_kwargs: object) -> mock.Mock:
            calls.append(argv)
            if argv[1] == "push":
                refs_path = Path(argv[argv.index("--image-refs") + 1])
                if refs_content is not None:
                    refs_path.write_text(refs_content)
                return mock.Mock(returncode=0, stdout="", stderr="")
            assert argv[1] == "digest"
            return mock.Mock(returncode=0, stdout=digest_stdout, stderr="")

        return run, calls

    def test_digest_captured_from_image_refs(self, tmp_path: Path) -> None:
        tar = tmp_path / "echo-artifact.oci.tar"
        tar.write_bytes(b"oci")
        run, calls = self._fake_run(refs_content=f"ghcr.io/org/echo:0.1.0@{DIGEST}\n")
        with mock.patch.object(crane.subprocess, "run", side_effect=run):
            digest = crane.push_oci_archive(tar, "ghcr.io/org/echo:0.1.0", crane="/usr/bin/crane")
        assert digest == DIGEST
        # One push, no digest fallback needed.
        assert [argv[1] for argv in calls] == ["push"]
        push_argv = calls[0]
        assert push_argv[:4] == ["/usr/bin/crane", "push", str(tar), "ghcr.io/org/echo:0.1.0"]
        assert "--index" in push_argv
        assert "--image-refs" in push_argv
        assert all(isinstance(argv, list) for argv in calls)

    def test_falls_back_to_crane_digest_when_refs_unparsable(self, tmp_path: Path) -> None:
        tar = tmp_path / "echo-artifact.oci.tar"
        tar.write_bytes(b"oci")
        run, calls = self._fake_run(refs_content="")
        with mock.patch.object(crane.subprocess, "run", side_effect=run):
            digest = crane.push_oci_archive(tar, "ghcr.io/org/echo:0.1.0", crane="/usr/bin/crane")
        assert digest == DIGEST
        assert [argv[1] for argv in calls] == ["push", "digest"]
        assert calls[1] == ["/usr/bin/crane", "digest", "ghcr.io/org/echo:0.1.0"]

    def test_push_failure_raises_with_output(self, tmp_path: Path) -> None:
        tar = tmp_path / "echo-artifact.oci.tar"
        tar.write_bytes(b"oci")
        boom = mock.Mock(returncode=1, stdout="", stderr="UNAUTHORIZED: authentication required")
        with (
            mock.patch.object(crane.subprocess, "run", return_value=boom),
            pytest.raises(CapabilityCliError, match=r"(?s)artifact push failed.*UNAUTHORIZED"),
        ):
            crane.push_oci_archive(tar, "ghcr.io/org/echo:0.1.0", crane="/usr/bin/crane")

    def test_bogus_digest_output_refused(self, tmp_path: Path) -> None:
        tar = tmp_path / "echo-artifact.oci.tar"
        tar.write_bytes(b"oci")
        run, _ = self._fake_run(refs_content="", digest_stdout="not-a-digest\n")
        with (
            mock.patch.object(crane.subprocess, "run", side_effect=run),
            pytest.raises(CapabilityCliError, match="unexpected value"),
        ):
            crane.push_oci_archive(tar, "ghcr.io/org/echo:0.1.0", crane="/usr/bin/crane")


class TestExtractDigest:
    def test_last_digest_wins(self) -> None:
        text = f"ghcr.io/org/echo:0.1.0@sha256:{'b' * 64}\nghcr.io/org/echo:0.1.0@{DIGEST}\n"
        assert crane._extract_digest(text) == DIGEST

    def test_no_digest_returns_none(self) -> None:
        assert crane._extract_digest("nothing here") is None
