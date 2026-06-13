"""kubectl.py: apply argv assembly and graceful in-cluster listing."""

from __future__ import annotations

import json
from pathlib import Path
from unittest import mock

import pytest

from flokoa.capability_cli import kubectl
from flokoa.capability_cli.errors import CapabilityCliError


class TestFindRequireKubectl:
    def test_env_override(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("FLOKOA_KUBECTL", "/opt/tools/kubectl")
        with mock.patch.object(kubectl.shutil, "which", return_value="/opt/tools/kubectl"):
            assert kubectl.find_kubectl() == "/opt/tools/kubectl"

    def test_absent_is_none_not_an_error(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("FLOKOA_KUBECTL", raising=False)
        with mock.patch.object(kubectl.shutil, "which", return_value=None):
            assert kubectl.find_kubectl() is None

    def test_require_raises_with_install_one_liner(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("FLOKOA_KUBECTL", raising=False)
        with (
            mock.patch.object(kubectl.shutil, "which", return_value=None),
            pytest.raises(CapabilityCliError, match=r"--apply needs kubectl on PATH.*brew install kubectl"),
        ):
            kubectl.require_kubectl()


class TestApplyManifest:
    def test_argv_without_namespace(self, tmp_path: Path) -> None:
        cr = tmp_path / "echo.capability.yaml"
        cr.write_text("kind: Capability\n")
        ok = mock.Mock(returncode=0, stdout="capability.agent.flokoa.ai/echo created\n", stderr="")
        with mock.patch.object(kubectl.subprocess, "run", return_value=ok) as run:
            output = kubectl.apply_manifest(cr, kubectl="/usr/bin/kubectl")
        assert run.call_args.args[0] == ["/usr/bin/kubectl", "apply", "-f", str(cr)]
        assert output == "capability.agent.flokoa.ai/echo created"

    def test_argv_with_namespace(self, tmp_path: Path) -> None:
        cr = tmp_path / "echo.capability.yaml"
        cr.write_text("kind: Capability\n")
        ok = mock.Mock(returncode=0, stdout="", stderr="")
        with mock.patch.object(kubectl.subprocess, "run", return_value=ok) as run:
            kubectl.apply_manifest(cr, kubectl="/usr/bin/kubectl", namespace="agents")
        assert run.call_args.args[0] == ["/usr/bin/kubectl", "apply", "-f", str(cr), "--namespace", "agents"]

    def test_failure_raises_with_output(self, tmp_path: Path) -> None:
        cr = tmp_path / "echo.capability.yaml"
        cr.write_text("kind: Capability\n")
        boom = mock.Mock(returncode=1, stdout="", stderr="admission webhook denied the request")
        with (
            mock.patch.object(kubectl.subprocess, "run", return_value=boom),
            pytest.raises(CapabilityCliError, match=r"(?s)kubectl apply of echo.capability.yaml failed.*denied"),
        ):
            kubectl.apply_manifest(cr, kubectl="/usr/bin/kubectl")


class TestListCapabilities:
    def test_kubectl_absent_skips_gracefully(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("FLOKOA_KUBECTL", raising=False)
        with mock.patch.object(kubectl.shutil, "which", return_value=None):
            result = kubectl.list_capabilities()
        assert result.items == []
        assert result.skipped_reason == "kubectl is not on PATH"

    def test_cluster_unreachable_skips_with_reason(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("FLOKOA_KUBECTL", raising=False)
        boom = mock.Mock(returncode=1, stdout="", stderr="The connection to the server localhost:8080 was refused")
        with (
            mock.patch.object(kubectl.shutil, "which", return_value="/usr/bin/kubectl"),
            mock.patch.object(kubectl.subprocess, "run", return_value=boom),
        ):
            result = kubectl.list_capabilities()
        assert result.items == []
        assert result.skipped_reason is not None
        assert "connection to the server" in result.skipped_reason

    def test_unparsable_json_skips(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("FLOKOA_KUBECTL", raising=False)
        garbled = mock.Mock(returncode=0, stdout="not json", stderr="")
        with (
            mock.patch.object(kubectl.shutil, "which", return_value="/usr/bin/kubectl"),
            mock.patch.object(kubectl.subprocess, "run", return_value=garbled),
        ):
            result = kubectl.list_capabilities()
        assert result.skipped_reason == "cluster lookup returned unparsable JSON"

    def test_success_parses_items_with_expected_argv(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("FLOKOA_KUBECTL", raising=False)
        payload = {"items": [{"metadata": {"name": "echo"}, "spec": {"version": "0.1.0"}}]}
        ok = mock.Mock(returncode=0, stdout=json.dumps(payload), stderr="")
        with (
            mock.patch.object(kubectl.shutil, "which", return_value="/usr/bin/kubectl"),
            mock.patch.object(kubectl.subprocess, "run", return_value=ok) as run,
        ):
            result = kubectl.list_capabilities()
        assert run.call_args.args[0] == ["/usr/bin/kubectl", "get", "capabilities", "-A", "-o", "json"]
        assert result.skipped_reason is None
        assert result.items[0]["metadata"]["name"] == "echo"
