import pytest
from flokoa_runner import RUNNER_VERSION
from flokoa_runner.errors import BootstrapError
from flokoa_runner.manifest import RunnerManifest, load_manifest


def test_load_manifest(runner_manifest_file, monkeypatch):
    monkeypatch.delenv("FLOKOA_EXPECTED_RUNNER_VERSION", raising=False)
    monkeypatch.delenv("FLOKOA_EXPECTED_SCHEMA_DIGEST", raising=False)
    manifest = load_manifest(runner_manifest_file)
    assert manifest.runner_version == RUNNER_VERSION
    assert manifest.pydantic_ai == "1.107.0"


def test_manifest_parses_builtin_capabilities():
    data = {
        "contractVersion": 1,
        "runnerVersion": "0.2.0",
        "python": "3.13",
        "pydantic-ai": "1.107.0",
        "builtinCapabilities": {
            "flokoa-openapi": {
                "entrypoint": "flokoa_openapi.capability:OpenAPI",
                "serializationName": "flokoa.OpenAPI",
            }
        },
    }
    manifest = RunnerManifest.from_dict(data)
    assert "flokoa-openapi" in manifest.builtin_capabilities
    assert manifest.builtin_capabilities["flokoa-openapi"]["entrypoint"] == "flokoa_openapi.capability:OpenAPI"


def test_manifest_without_builtin_capabilities_defaults_empty():
    data = {"contractVersion": 1, "runnerVersion": "0.2.0", "python": "3.13", "pydantic-ai": "1.107.0"}
    manifest = RunnerManifest.from_dict(data)
    assert manifest.builtin_capabilities == {}


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
