"""cosign.py: binary preflight and sign argv assembly (mocked subprocess)."""

from __future__ import annotations

from pathlib import Path
from unittest import mock

import pytest

from flokoa.capability_cli import cosign
from flokoa.capability_cli.errors import CapabilityCliError

PINNED = "ghcr.io/org/echo:0.1.0@sha256:" + "a" * 64


class TestFindCosign:
    def test_env_override_wins(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("FLOKOA_COSIGN", "/opt/tools/cosign")
        with mock.patch.object(cosign.shutil, "which", return_value="/opt/tools/cosign"):
            assert cosign.find_cosign() == "/opt/tools/cosign"

    def test_env_override_missing_binary(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("FLOKOA_COSIGN", "/opt/tools/cosign")
        with (
            mock.patch.object(cosign.shutil, "which", return_value=None),
            pytest.raises(CapabilityCliError, match="FLOKOA_COSIGN=/opt/tools/cosign is not an executable"),
        ):
            cosign.find_cosign()

    def test_missing_binary_names_install_one_liner(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("FLOKOA_COSIGN", raising=False)
        with (
            mock.patch.object(cosign.shutil, "which", return_value=None),
            pytest.raises(CapabilityCliError, match=r"--sign needs cosign on PATH.*sigstore/cosign"),
        ):
            cosign.find_cosign()


class TestSignDigest:
    def test_keyless_argv(self) -> None:
        ok = mock.Mock(returncode=0, stdout="", stderr="")
        with mock.patch.object(cosign.subprocess, "run", return_value=ok) as run:
            cosign.sign_digest(PINNED, cosign="/usr/bin/cosign")
        assert run.call_args.args[0] == ["/usr/bin/cosign", "sign", "--yes", PINNED]

    def test_key_based_argv(self, tmp_path: Path) -> None:
        key = tmp_path / "cosign.key"
        key.write_text("key")
        ok = mock.Mock(returncode=0, stdout="", stderr="")
        with mock.patch.object(cosign.subprocess, "run", return_value=ok) as run:
            cosign.sign_digest(PINNED, cosign="/usr/bin/cosign", key=key)
        assert run.call_args.args[0] == ["/usr/bin/cosign", "sign", "--yes", "--key", str(key), PINNED]

    def test_failure_raises_with_mode_and_output(self) -> None:
        boom = mock.Mock(returncode=1, stdout="", stderr="no ambient credentials")
        with (
            mock.patch.object(cosign.subprocess, "run", return_value=boom),
            pytest.raises(CapabilityCliError, match=r"(?s)keyless \(ambient OIDC\) signing.*no ambient credentials"),
        ):
            cosign.sign_digest(PINNED, cosign="/usr/bin/cosign")
