"""``flokoa capability push``: digest capture, CR rewrite, option matrix.

crane/cosign/kubectl are mocked at the module boundary — no binaries, no
network. The dist fixture is produced by the same artifact/cr code `build`
uses, so the placeholder contract between build and push is the real one.
"""

from __future__ import annotations

import json
from pathlib import Path
from unittest import mock

import pytest
import yaml
from click.testing import CliRunner
from flokoa_types.capability import CapabilitySpec

from flokoa.capability_cli import artifact as artifact_mod
from flokoa.capability_cli import cr as cr_mod
from flokoa.capability_cli import push as push_mod
from flokoa.capability_cli.push import push, run_push

DIGEST = "sha256:" + "a" * 64
REF = "ghcr.io/danielnyari/capabilities/flokoa-cap-echo:0.1.0"


def echo_manifest(**overrides: object) -> artifact_mod.ArtifactManifest:
    fields: dict = {
        "name": "flokoa-cap-echo",
        "version": "0.1.0",
        "entrypoint": "flokoa_cap_echo:EchoCapability",
        "requires": artifact_mod.ManifestRequires.model_validate({
            "python": "3.13",
            "pydantic-ai": ">=1.107,<2",
            "flokoa-runner": ">=0.2",
        }),
        "dependencies": [],
        "wheels": [{"file": "flokoa_cap_echo-0.1.0-py3-none-any.whl", "sha256": "f" * 64}],
        "config_schema": {"type": "object", "properties": {"prefix": {"type": "string"}}},
    }
    fields.update(overrides)
    return artifact_mod.ArtifactManifest(**fields)


def make_dist(tmp_path: Path, *, permissive: bool = False, with_manifest: bool = True) -> Path:
    """A dist/ directory exactly as `flokoa capability build` leaves it."""
    dist = tmp_path / "dist"
    dist.mkdir(exist_ok=True)
    manifest = echo_manifest(config_schema=None) if permissive else echo_manifest()
    (dist / "flokoa-cap-echo-artifact.oci.tar").write_bytes(b"oci-layout")
    (dist / "flokoa-cap-echo.capability.yaml").write_text(
        cr_mod.render_capability_cr("flokoa-cap-echo", "flokoa-cap-echo:0.1.0", manifest, permissive=permissive)
    )
    if with_manifest:
        artifact_mod.write_manifest(manifest, dist / "manifest.json")
    return dist


@pytest.fixture
def tools(monkeypatch: pytest.MonkeyPatch) -> dict[str, mock.Mock]:
    """Mock the crane/cosign/kubectl boundary; record every call."""
    mocks = {
        "find_crane": mock.Mock(return_value="/usr/bin/crane"),
        "push_oci_archive": mock.Mock(return_value=DIGEST),
        "find_cosign": mock.Mock(return_value="/usr/bin/cosign"),
        "sign_digest": mock.Mock(),
        "require_kubectl": mock.Mock(return_value="/usr/bin/kubectl"),
        "apply_manifest": mock.Mock(return_value="capability.agent.flokoa.ai/flokoa-cap-echo created"),
    }
    monkeypatch.setattr(push_mod.crane_mod, "find_crane", mocks["find_crane"])
    monkeypatch.setattr(push_mod.crane_mod, "push_oci_archive", mocks["push_oci_archive"])
    monkeypatch.setattr(push_mod.cosign_mod, "find_cosign", mocks["find_cosign"])
    monkeypatch.setattr(push_mod.cosign_mod, "sign_digest", mocks["sign_digest"])
    monkeypatch.setattr(push_mod.kubectl_mod, "require_kubectl", mocks["require_kubectl"])
    monkeypatch.setattr(push_mod.kubectl_mod, "apply_manifest", mocks["apply_manifest"])
    return mocks


class TestPushHappyPath:
    def test_digest_captured_and_cr_rewritten(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path)
        result = CliRunner().invoke(push, [REF, "--from", str(dist)])
        assert result.exit_code == 0, result.output

        push_args = tools["push_oci_archive"].call_args
        assert push_args.args == (dist / "flokoa-cap-echo-artifact.oci.tar", REF)
        assert push_args.kwargs == {"crane": "/usr/bin/crane"}

        cr_text = (dist / "flokoa-cap-echo.capability.yaml").read_text()
        doc = yaml.safe_load(cr_text)
        assert doc["spec"]["artifact"] == f"{REF}@{DIGEST}"
        # The pinned spec is a valid CRD mirror and the placeholder header is gone.
        CapabilitySpec.model_validate(doc["spec"])
        assert "DIGEST-PENDING" not in cr_text
        assert "digest recorded by" in cr_text

        assert f"Pushed {REF}@{DIGEST}" in result.output
        assert "Next: kubectl apply -f" in result.output
        # Nothing optional ran.
        tools["sign_digest"].assert_not_called()
        tools["apply_manifest"].assert_not_called()

    def test_run_push_result(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path)
        outcome = run_push(REF, from_dir=dist)
        assert outcome.pinned_ref == f"{REF}@{DIGEST}"
        assert outcome.cr_name == "flokoa-cap-echo"
        assert outcome.signed is False
        assert outcome.applied is None
        assert outcome.index_file is None


class TestPushErrors:
    def test_missing_dist_dir(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        result = CliRunner().invoke(push, [REF, "--from", str(tmp_path / "nope")])
        assert result.exit_code != 0
        assert "run `flokoa capability build` first" in result.output

    def test_missing_artifact_tar(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path)
        (dist / "flokoa-cap-echo-artifact.oci.tar").unlink()
        result = CliRunner().invoke(push, [REF, "--from", str(dist)])
        assert result.exit_code != 0
        assert "no *-artifact.oci.tar" in result.output

    def test_missing_cr(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path)
        (dist / "flokoa-cap-echo.capability.yaml").unlink()
        result = CliRunner().invoke(push, [REF, "--from", str(dist)])
        assert result.exit_code != 0
        assert "the Capability CR is missing" in result.output

    def test_multiple_builds_in_dir_refused(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path)
        (dist / "other-cap-artifact.oci.tar").write_bytes(b"oci")
        result = CliRunner().invoke(push, [REF, "--from", str(dist)])
        assert result.exit_code != 0
        assert "more than one artifact tar" in result.output

    def test_placeholder_missing_means_already_pushed(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path)
        first = CliRunner().invoke(push, [REF, "--from", str(dist)])
        assert first.exit_code == 0, first.output
        second = CliRunner().invoke(push, [REF, "--from", str(dist)])
        assert second.exit_code != 0
        assert "does not carry the @sha256:DIGEST-PENDING placeholder" in second.output
        assert "looks already pushed" in second.output
        assert tools["push_oci_archive"].call_count == 1

    def test_digest_ref_refused(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path)
        result = CliRunner().invoke(push, [f"{REF}@{DIGEST}", "--from", str(dist)])
        assert result.exit_code != 0
        assert "REF must be a tag reference" in result.output
        tools["push_oci_archive"].assert_not_called()


class TestSignOption:
    def test_sign_keyless(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path)
        result = CliRunner().invoke(push, [REF, "--from", str(dist), "--sign"])
        assert result.exit_code == 0, result.output
        sign_args = tools["sign_digest"].call_args
        assert sign_args.args == (f"{REF}@{DIGEST}",)
        assert sign_args.kwargs == {"cosign": "/usr/bin/cosign", "key": None}
        assert "keyless" in result.output

    def test_sign_with_key(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path)
        key = tmp_path / "cosign.key"
        key.write_text("key")
        result = CliRunner().invoke(push, [REF, "--from", str(dist), "--sign", "--cosign-key", str(key)])
        assert result.exit_code == 0, result.output
        assert tools["sign_digest"].call_args.kwargs["key"] == key
        assert "key-based" in result.output

    def test_cosign_key_without_sign_is_a_usage_error(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path)
        key = tmp_path / "cosign.key"
        key.write_text("key")
        result = CliRunner().invoke(push, [REF, "--from", str(dist), "--cosign-key", str(key)])
        assert result.exit_code != 0
        assert "--cosign-key needs --sign" in result.output
        tools["push_oci_archive"].assert_not_called()

    def test_cosign_preflight_runs_before_push(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path)
        tools["find_cosign"].side_effect = push_mod.CapabilityCliError("--sign needs cosign on PATH")
        result = CliRunner().invoke(push, [REF, "--from", str(dist), "--sign"])
        assert result.exit_code != 0
        tools["push_oci_archive"].assert_not_called()


class TestApplyOption:
    def test_apply_default_namespace(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path)
        result = CliRunner().invoke(push, [REF, "--from", str(dist), "--apply"])
        assert result.exit_code == 0, result.output
        apply_args = tools["apply_manifest"].call_args
        assert apply_args.args == (dist / "flokoa-cap-echo.capability.yaml",)
        assert apply_args.kwargs == {"kubectl": "/usr/bin/kubectl", "namespace": None}
        assert "created" in result.output
        assert "Next: kubectl apply" not in result.output

    def test_apply_with_namespace(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path)
        result = CliRunner().invoke(push, [REF, "--from", str(dist), "--apply", "--namespace", "agents"])
        assert result.exit_code == 0, result.output
        assert tools["apply_manifest"].call_args.kwargs["namespace"] == "agents"

    def test_namespace_without_apply_is_a_usage_error(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path)
        result = CliRunner().invoke(push, [REF, "--from", str(dist), "--namespace", "agents"])
        assert result.exit_code != 0
        assert "--namespace needs --apply" in result.output

    def test_kubectl_preflight_runs_before_push(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path)
        tools["require_kubectl"].side_effect = push_mod.CapabilityCliError("--apply needs kubectl on PATH")
        result = CliRunner().invoke(push, [REF, "--from", str(dist), "--apply"])
        assert result.exit_code != 0
        tools["push_oci_archive"].assert_not_called()


class TestIndexOption:
    def test_appends_entry_to_fresh_index(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path)
        index_file = tmp_path / "checkout" / "index.json"
        result = CliRunner().invoke(push, [REF, "--from", str(dist), "--sign", "--index", str(index_file)])
        assert result.exit_code == 0, result.output
        payload = json.loads(index_file.read_text())
        assert payload["schemaVersion"] == 1
        (entry,) = payload["capabilities"]
        assert entry["name"] == "flokoa-cap-echo"
        assert entry["version"] == "0.1.0"
        assert entry["artifact"] == f"{REF}@{DIGEST}"
        assert entry["entrypoint"] == "flokoa_cap_echo:EchoCapability"
        assert entry["requires"]["flokoa-runner"] == ">=0.2"
        assert entry["schemaPolicy"] == "strict"
        assert entry["signed"] is True
        assert "commit and push it to publish" in result.output

    def test_replaces_same_name_and_version(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        index_file = tmp_path / "index.json"
        for attempt in range(2):
            dist = make_dist(tmp_path)  # re-create: push rewrites the CR placeholder
            result = CliRunner().invoke(push, [REF, "--from", str(dist), "--index", str(index_file)])
            assert result.exit_code == 0, f"attempt {attempt}: {result.output}"
        payload = json.loads(index_file.read_text())
        assert len(payload["capabilities"]) == 1
        assert payload["capabilities"][0]["signed"] is False

    def test_permissive_cr_flagged_in_index(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path, permissive=True)
        index_file = tmp_path / "index.json"
        result = CliRunner().invoke(push, [REF, "--from", str(dist), "--index", str(index_file)])
        assert result.exit_code == 0, result.output
        payload = json.loads(index_file.read_text())
        assert payload["capabilities"][0]["schemaPolicy"] == "permissive"

    def test_index_requires_manifest_json(self, tmp_path: Path, tools: dict[str, mock.Mock]) -> None:
        dist = make_dist(tmp_path, with_manifest=False)
        result = CliRunner().invoke(push, [REF, "--from", str(dist), "--index", str(tmp_path / "index.json")])
        assert result.exit_code != 0
        assert "manifest.json" in result.output
        tools["push_oci_archive"].assert_not_called()
