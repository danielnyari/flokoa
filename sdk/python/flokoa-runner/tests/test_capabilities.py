import json
import textwrap

import pytest
from flokoa_runner.capabilities import install_capabilities
from flokoa_runner.errors import BootstrapError
from flokoa_runner.manifest import RunnerManifest


@pytest.fixture
def runner_manifest() -> RunnerManifest:
    return RunnerManifest(
        contract_version=1,
        runner_version="0.2.0",
        python="3.13",
        pydantic_ai="1.107.0",
    )


def write_capability(root, name, manifest):
    cap_dir = root / name
    cap_dir.mkdir(parents=True)
    (cap_dir / "manifest.json").write_text(json.dumps(manifest))
    return cap_dir


def test_no_capability_dir_is_fine(tmp_path, runner_manifest):
    assert install_capabilities(runner_manifest, root=tmp_path / "none") == []


def test_requires_mismatch_fails_before_install(tmp_path, runner_manifest):
    write_capability(
        tmp_path,
        "shields",
        {
            "name": "shields",
            "entrypoint": "shields:Shields",
            "requires": {"pydantic-ai": ">=2.0"},
        },
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
        {"name": "shields", "entrypoint": "shields:Shields", "requires": {"python": "3.12"}},
    )
    with pytest.raises(BootstrapError):
        install_capabilities(runner_manifest, root=tmp_path)


def test_missing_manifest_fails(tmp_path, runner_manifest):
    (tmp_path / "broken").mkdir()
    with pytest.raises(BootstrapError) as excinfo:
        install_capabilities(runner_manifest, root=tmp_path)
    assert "manifest missing" in excinfo.value.error


def test_entrypoint_loading_after_install(tmp_path, runner_manifest, monkeypatch):
    """A compatible wheelhouse installs and its entrypoint class is returned.

    pip is monkeypatched out: the entrypoint module is placed on sys.path
    directly, which is exactly the state pip install would produce.
    """
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
    monkeypatch.setattr("flokoa_runner.capabilities._pip_install", lambda *a, **k: None)

    caps_root = tmp_path / "capabilities"
    write_capability(
        caps_root,
        "fake",
        {
            "name": "fake-capability",
            "version": "0.1.0",
            "entrypoint": "fake_capability:FakeCapability",
            "requires": {"python": "3.13", "pydantic-ai": ">=1.100,<2", "flokoa-runner": ">=0.2"},
        },
    )

    classes = install_capabilities(runner_manifest, root=caps_root)
    assert len(classes) == 1
    assert classes[0].__name__ == "FakeCapability"


def test_bad_entrypoint_format(tmp_path, runner_manifest, monkeypatch):
    monkeypatch.setattr("flokoa_runner.capabilities._pip_install", lambda *a, **k: None)
    write_capability(tmp_path, "bad", {"name": "bad", "entrypoint": "no-colon"})
    with pytest.raises(BootstrapError) as excinfo:
        install_capabilities(runner_manifest, root=tmp_path)
    assert "module:attr" in excinfo.value.error
