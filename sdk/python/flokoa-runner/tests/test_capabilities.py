"""install_capabilities: manifest schema, requires, wheelhouse integrity, install.

The integrity matrix (roadmap 09) runs on tmp-dir wheelhouses with real (tiny,
zipfile-built) wheels; the valid-path test hydrates the actual echo reference
fixture (operator/test/e2e/fixtures/capabilities/echo) through the full
pipeline, so the fixture and the runner contract cannot drift apart.
"""

import hashlib
import json
import sys
import textwrap
import types
import zipfile
from pathlib import Path

import pytest
from flokoa_runner.capabilities import install_capabilities
from flokoa_runner.errors import BootstrapError
from flokoa_runner.manifest import RunnerManifest

REPO_ROOT = Path(__file__).resolve().parents[4]
ECHO_FIXTURE = REPO_ROOT / "operator" / "test" / "e2e" / "fixtures" / "capabilities" / "echo"

DUMMY_SHA = "0" * 64


@pytest.fixture
def runner_manifest() -> RunnerManifest:
    return RunnerManifest(
        contract_version=1,
        runner_version="0.2.0",
        python="3.13",
        pydantic_ai="1.107.0",
    )


def sha256_of(path: Path) -> str:
    with path.open("rb") as fh:
        return hashlib.file_digest(fh, "sha256").hexdigest()


def build_wheel(
    directory: Path,
    name: str = "fake-capability",
    version: str = "0.1.0",
    files: dict[str, str] | None = None,
) -> str:
    """Write a real (minimal) wheel — a zip with a dist-info — and return its filename."""
    dist = name.replace("-", "_")
    filename = f"{dist}-{version}-py3-none-any.whl"
    dist_info = f"{dist}-{version}.dist-info"
    directory.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(directory / filename, "w") as zf:
        for arcname, content in (files or {f"{dist}.py": "class FakeCapability:\n    pass\n"}).items():
            zf.writestr(arcname, content)
        zf.writestr(f"{dist_info}/METADATA", f"Metadata-Version: 2.1\nName: {name}\nVersion: {version}\n")
        zf.writestr(
            f"{dist_info}/WHEEL", "Wheel-Version: 1.0\nGenerator: test\nRoot-Is-Purelib: true\nTag: py3-none-any\n"
        )
        zf.writestr(f"{dist_info}/RECORD", "")
    return filename


def base_manifest(**overrides):
    manifest = {
        "name": "fake-capability",
        "version": "0.1.0",
        "contractVersion": 1,
        "entrypoint": "fake_capability:FakeCapability",
        "requires": {"python": "3.13", "pydantic-ai": ">=1.100,<2", "flokoa-runner": ">=0.2"},
        "dependencies": [],
    }
    manifest.update(overrides)
    return manifest


def write_capability(root, name, manifest):
    cap_dir = root / name
    cap_dir.mkdir(parents=True)
    (cap_dir / "manifest.json").write_text(json.dumps(manifest))
    return cap_dir


def write_valid_capability(root, name="fake", **manifest_overrides):
    """A wheelhouse whose single wheel exists and hashes to its manifest entry."""
    cap_dir = root / name
    filename = build_wheel(cap_dir)
    manifest = base_manifest(
        wheels=[{"file": filename, "sha256": sha256_of(cap_dir / filename)}],
        **manifest_overrides,
    )
    (cap_dir / "manifest.json").write_text(json.dumps(manifest))
    return cap_dir


@pytest.fixture
def no_pip(monkeypatch):
    monkeypatch.setattr("flokoa_runner.capabilities._pip_install", lambda *a, **k: None)


# --- manifest schema enforcement ------------------------------------------


def test_no_capability_dir_is_fine(tmp_path, runner_manifest):
    assert install_capabilities(runner_manifest, root=tmp_path / "none") == []


def test_missing_manifest_fails(tmp_path, runner_manifest):
    (tmp_path / "broken").mkdir()
    with pytest.raises(BootstrapError) as excinfo:
        install_capabilities(runner_manifest, root=tmp_path)
    assert "manifest missing" in excinfo.value.error


@pytest.mark.parametrize("wheels", [None, []], ids=["absent", "empty"])
def test_missing_wheels_list_fails(tmp_path, runner_manifest, wheels):
    manifest = base_manifest()
    manifest.pop("dependencies")
    if wheels is not None:
        manifest["wheels"] = wheels
    write_capability(tmp_path, "shields", manifest)
    with pytest.raises(BootstrapError) as excinfo:
        install_capabilities(runner_manifest, root=tmp_path)
    err = excinfo.value
    assert err.error == "capability manifest missing wheels list"
    assert err.details["capability"] == "shields"


def test_malformed_wheel_entry_fails(tmp_path, runner_manifest):
    write_capability(tmp_path, "shields", base_manifest(wheels=[{"file": "x.whl"}]))
    with pytest.raises(BootstrapError) as excinfo:
        install_capabilities(runner_manifest, root=tmp_path)
    assert "wheels entries must be objects with file and sha256" in excinfo.value.error


def test_wheel_path_traversal_rejected(tmp_path, runner_manifest):
    write_capability(tmp_path, "shields", base_manifest(wheels=[{"file": "../evil.whl", "sha256": DUMMY_SHA}]))
    with pytest.raises(BootstrapError) as excinfo:
        install_capabilities(runner_manifest, root=tmp_path)
    assert "bare filenames" in excinfo.value.error
    assert excinfo.value.details["file"] == "../evil.whl"


def test_unsupported_contract_version_fails(tmp_path, runner_manifest):
    write_capability(tmp_path, "shields", base_manifest(contractVersion=2))
    with pytest.raises(BootstrapError) as excinfo:
        install_capabilities(runner_manifest, root=tmp_path)
    err = excinfo.value
    assert err.error == "capability manifest declares an unsupported contractVersion"
    assert err.details["manifest_contract"] == 2
    assert err.details["runner_contract"] == 1


def test_invalid_dependency_pin_fails(tmp_path, runner_manifest):
    write_capability(
        tmp_path,
        "shields",
        base_manifest(
            wheels=[{"file": "x.whl", "sha256": DUMMY_SHA}],
            dependencies=["inflection>=0.5"],
        ),
    )
    with pytest.raises(BootstrapError) as excinfo:
        install_capabilities(runner_manifest, root=tmp_path)
    err = excinfo.value
    assert err.error == "invalid dependency pin in manifest"
    assert err.details["pin"] == "inflection>=0.5"


def test_dependencies_must_be_a_list(tmp_path, runner_manifest):
    write_capability(
        tmp_path,
        "shields",
        base_manifest(wheels=[{"file": "x.whl", "sha256": DUMMY_SHA}], dependencies="inflection==0.5.1"),
    )
    with pytest.raises(BootstrapError) as excinfo:
        install_capabilities(runner_manifest, root=tmp_path)
    assert "dependencies must be a list" in excinfo.value.error


# --- requires (defense in depth; unchanged semantics) ----------------------


def test_requires_mismatch_fails_before_install(tmp_path, runner_manifest):
    write_capability(
        tmp_path,
        "shields",
        base_manifest(
            name="shields",
            entrypoint="shields:Shields",
            requires={"pydantic-ai": ">=2.0"},
            wheels=[{"file": "shields-0.1.0-py3-none-any.whl", "sha256": DUMMY_SHA}],
        ),
    )
    with pytest.raises(BootstrapError) as excinfo:
        install_capabilities(runner_manifest, root=tmp_path)
    err = excinfo.value
    assert err.stage == "install_capabilities"
    assert err.details["capability"] == "shields"


def test_python_minor_mismatch_fails(tmp_path, runner_manifest):
    write_capability(
        tmp_path,
        "shields",
        base_manifest(
            name="shields",
            entrypoint="shields:Shields",
            requires={"python": "3.12"},
            wheels=[{"file": "shields-0.1.0-py3-none-any.whl", "sha256": DUMMY_SHA}],
        ),
    )
    with pytest.raises(BootstrapError):
        install_capabilities(runner_manifest, root=tmp_path)


# --- wheelhouse integrity matrix -------------------------------------------


def test_valid_wheelhouse_passes_integrity(tmp_path, runner_manifest, no_pip, monkeypatch):
    pkg_dir = tmp_path / "srcpkg"
    pkg_dir.mkdir()
    (pkg_dir / "fake_capability.py").write_text(
        textwrap.dedent(
            """
            class FakeCapability:
                pass
            """
        )
    )
    monkeypatch.syspath_prepend(str(pkg_dir))

    caps_root = tmp_path / "capabilities"
    write_valid_capability(caps_root)

    classes = install_capabilities(runner_manifest, root=caps_root)
    assert len(classes) == 1
    assert classes[0].__name__ == "FakeCapability"


def test_sha_tampered_wheel_fails(tmp_path, runner_manifest, no_pip):
    cap_dir = write_valid_capability(tmp_path)
    wheel = next(cap_dir.glob("*.whl"))
    expected = sha256_of(wheel)
    with wheel.open("ab") as fh:
        fh.write(b"tampered")
    with pytest.raises(BootstrapError) as excinfo:
        install_capabilities(runner_manifest, root=tmp_path)
    err = excinfo.value
    assert err.error == "wheelhouse integrity check failed"
    assert err.details["capability"] == "fake"
    assert err.details["file"] == wheel.name
    assert err.details["expected_sha256"] == expected
    assert err.details["actual_sha256"] == sha256_of(wheel)
    assert err.details["expected_sha256"] != err.details["actual_sha256"]


def test_missing_listed_wheel_fails(tmp_path, runner_manifest, no_pip):
    write_capability(
        tmp_path,
        "fake",
        base_manifest(wheels=[{"file": "fake_capability-0.1.0-py3-none-any.whl", "sha256": DUMMY_SHA}]),
    )
    with pytest.raises(BootstrapError) as excinfo:
        install_capabilities(runner_manifest, root=tmp_path)
    err = excinfo.value
    assert err.error == "wheel listed in manifest missing from wheelhouse"
    assert err.details["file"] == "fake_capability-0.1.0-py3-none-any.whl"


def test_unlisted_wheel_rejected(tmp_path, runner_manifest, no_pip):
    cap_dir = write_valid_capability(tmp_path)
    build_wheel(cap_dir, name="ride-along", version="6.6.6")
    with pytest.raises(BootstrapError) as excinfo:
        install_capabilities(runner_manifest, root=tmp_path)
    err = excinfo.value
    assert err.error == "unlisted wheel in wheelhouse"
    assert err.details["file"] == "ride_along-6.6.6-py3-none-any.whl"


@pytest.mark.parametrize("intruder", ["pkg-1.0.0.tar.gz", "vendored.zip", "setup.py"])
def test_non_wheel_installable_rejected(tmp_path, runner_manifest, no_pip, intruder):
    cap_dir = write_valid_capability(tmp_path)
    (cap_dir / intruder).write_bytes(b"not a wheel")
    with pytest.raises(BootstrapError) as excinfo:
        install_capabilities(runner_manifest, root=tmp_path)
    err = excinfo.value
    assert err.error == "non-wheel file in wheelhouse"
    assert err.details["file"] == intruder


# --- pinned-closure install -------------------------------------------------


def test_pip_installs_the_explicit_pin_set(tmp_path, runner_manifest, monkeypatch):
    cap_dir = write_valid_capability(tmp_path, dependencies=["inflection==0.5.1", "extra-pin==2.0"])

    captured: dict[str, list[str]] = {}

    def fake_run(cmd, **kwargs):
        captured["cmd"] = cmd
        return types.SimpleNamespace(returncode=0, stderr="")

    monkeypatch.setattr("flokoa_runner.capabilities.subprocess.run", fake_run)
    monkeypatch.setattr("flokoa_runner.capabilities._load_entrypoint", lambda *a, **k: object)

    install_capabilities(runner_manifest, root=tmp_path)
    assert captured["cmd"] == [
        sys.executable,
        "-m",
        "pip",
        "install",
        "--no-index",
        "--find-links",
        str(cap_dir),
        "fake-capability==0.1.0",
        "inflection==0.5.1",
        "extra-pin==2.0",
    ]


def test_pip_failure_is_a_structured_error(tmp_path, runner_manifest, monkeypatch):
    write_valid_capability(tmp_path)
    monkeypatch.setattr(
        "flokoa_runner.capabilities.subprocess.run",
        lambda cmd, **kwargs: types.SimpleNamespace(returncode=1, stderr="resolution impossible"),
    )
    with pytest.raises(BootstrapError) as excinfo:
        install_capabilities(runner_manifest, root=tmp_path)
    err = excinfo.value
    assert err.error == "wheelhouse install failed"
    assert err.details["requirement"] == "fake-capability==0.1.0"
    assert "resolution impossible" in err.details["pip_stderr"]


def test_pip_failure_redacts_url_credentials(tmp_path, runner_manifest, monkeypatch):
    write_valid_capability(tmp_path)
    stderr = (
        "Looking in indexes: https://deploy:s3cretT0ken@pypi.internal.example/simple\n"
        "WARNING: Retrying after connection broken by proxy http://svc-user@proxy.internal:3128\n"
        "ERROR: Could not find a version that satisfies the requirement fake-capability==0.1.0"
    )
    monkeypatch.setattr(
        "flokoa_runner.capabilities.subprocess.run",
        lambda cmd, **kwargs: types.SimpleNamespace(returncode=1, stderr=stderr),
    )
    with pytest.raises(BootstrapError) as excinfo:
        install_capabilities(runner_manifest, root=tmp_path)
    pip_stderr = excinfo.value.details["pip_stderr"]
    assert "s3cretT0ken" not in pip_stderr
    assert "deploy:" not in pip_stderr
    assert "svc-user@" not in pip_stderr
    assert "https://<redacted>@pypi.internal.example/simple" in pip_stderr
    assert "http://<redacted>@proxy.internal:3128" in pip_stderr
    # The rest of pip's diagnostics survive the redaction.
    assert "Could not find a version" in pip_stderr


# --- entrypoint loading ------------------------------------------------------


def test_bad_entrypoint_format(tmp_path, runner_manifest, no_pip):
    write_valid_capability(tmp_path, name="bad", entrypoint="no-colon")
    with pytest.raises(BootstrapError) as excinfo:
        install_capabilities(runner_manifest, root=tmp_path)
    assert "module:attr" in excinfo.value.error


def test_echo_fixture_loads_through_the_full_pipeline(tmp_path, runner_manifest, monkeypatch):
    """The e2e reference fixture is a real capability under this contract.

    Builds a wheelhouse from the actual echo fixture source + artifact.json,
    runs the full pipeline (schema → requires → integrity → install →
    entrypoint), with pip install simulated by extracting the verified wheel
    onto sys.path — exactly the state a real install produces.
    """
    artifact = json.loads((ECHO_FIXTURE / "artifact.json").read_text())
    source = (ECHO_FIXTURE / "src" / "flokoa_cap_echo" / "__init__.py").read_text()

    caps_root = tmp_path / "capabilities"
    cap_dir = caps_root / "echo"
    filename = build_wheel(
        cap_dir,
        name=artifact["name"],
        version=artifact["version"],
        files={"flokoa_cap_echo/__init__.py": source},
    )
    manifest = {**artifact, "wheels": [{"file": filename, "sha256": sha256_of(cap_dir / filename)}]}
    (cap_dir / "manifest.json").write_text(json.dumps(manifest))

    site_dir = tmp_path / "site"
    site_dir.mkdir()

    def extract_install(cap_dir, manifest):
        for entry in manifest["wheels"]:
            with zipfile.ZipFile(cap_dir / entry["file"]) as zf:
                zf.extractall(site_dir)

    monkeypatch.setattr("flokoa_runner.capabilities._pip_install", extract_install)
    monkeypatch.syspath_prepend(str(site_dir))
    monkeypatch.delitem(sys.modules, "flokoa_cap_echo", raising=False)

    classes = install_capabilities(runner_manifest, root=caps_root)
    assert len(classes) == 1
    cls = classes[0]
    assert cls.__name__ == "EchoCapability"
    assert cls.get_serialization_name() == "EchoCapability"

    from pydantic_ai.capabilities.abstract import AbstractCapability

    assert issubclass(cls, AbstractCapability)
    # Spec entries hydrate via from_spec(**config) — the config field works.
    instance = cls.from_spec(prefix="custom")
    assert instance.get_toolset() is not None
