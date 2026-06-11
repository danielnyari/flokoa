import pytest
from flokoa_runner import RUNNER_VERSION
from flokoa_runner.errors import BootstrapError
from flokoa_runner.manifest import load_manifest


def test_load_manifest(runner_manifest_file, monkeypatch):
    monkeypatch.delenv("FLOKOA_EXPECTED_RUNNER_VERSION", raising=False)
    monkeypatch.delenv("FLOKOA_EXPECTED_SCHEMA_DIGEST", raising=False)
    manifest = load_manifest(runner_manifest_file)
    assert manifest.runner_version == RUNNER_VERSION
    assert manifest.pydantic_ai == "1.107.0"


def test_missing_manifest_fails_with_stage(tmp_path):
    with pytest.raises(BootstrapError) as excinfo:
        load_manifest(tmp_path / "nope.json")
    assert excinfo.value.stage == "load_manifest"


def test_runner_version_skew_is_loud(runner_manifest_file, monkeypatch):
    monkeypatch.setenv("FLOKOA_EXPECTED_RUNNER_VERSION", "9.9.9")
    with pytest.raises(BootstrapError) as excinfo:
        load_manifest(runner_manifest_file)
    err = excinfo.value
    assert err.details["expected"] == "9.9.9"
    assert err.details["actual"] == RUNNER_VERSION


def test_schema_digest_skew_is_loud(runner_manifest_file, monkeypatch):
    monkeypatch.delenv("FLOKOA_EXPECTED_RUNNER_VERSION", raising=False)
    monkeypatch.setenv("FLOKOA_EXPECTED_SCHEMA_DIGEST", "sha256:other")
    with pytest.raises(BootstrapError) as excinfo:
        load_manifest(runner_manifest_file)
    assert "digest" in excinfo.value.error
