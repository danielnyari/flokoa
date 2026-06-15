"""resolve_builtin_capabilities: import built-in classes from the runner env.

Built-in capabilities (source: builtin) are baked into the runner image. The
runner resolves their classes from its own environment by the manifest's
``builtinCapabilities`` metadata — no wheelhouse, no pip, no integrity check
(the image digest is the trust boundary, architecture §2.6). A broken built-in
entrypoint is a loud BootstrapError naming the built-in.
"""

import sys
import types

import pytest
from flokoa_runner.capabilities import install_capabilities, resolve_builtin_capabilities
from flokoa_runner.errors import BootstrapError
from flokoa_runner.manifest import RunnerManifest


def _manifest(builtin: dict) -> RunnerManifest:
    return RunnerManifest(
        contract_version=1,
        runner_version="0.2.0",
        python="3.13",
        pydantic_ai="1.107.0",
        builtin_capabilities=builtin,
    )


@pytest.fixture
def fake_builtin_module(monkeypatch):
    """Register an in-memory module exposing a fake capability class."""
    module = types.ModuleType("fake_builtin_cap")

    class FakeBuiltin:
        @classmethod
        def get_serialization_name(cls) -> str:
            return "flokoa.FakeBuiltin"

    module.FakeBuiltin = FakeBuiltin
    monkeypatch.setitem(sys.modules, "fake_builtin_cap", module)
    return FakeBuiltin


def test_resolve_builtin_imports_from_env(fake_builtin_module):
    manifest = _manifest(
        {"fake-builtin": {"entrypoint": "fake_builtin_cap:FakeBuiltin", "serializationName": "flokoa.FakeBuiltin"}}
    )
    classes = resolve_builtin_capabilities(manifest)
    assert classes == [fake_builtin_module]


def test_install_capabilities_includes_builtins_without_wheelhouse(tmp_path, fake_builtin_module):
    # No /opt/flokoa/capabilities/<name>/ dir exists, yet the built-in class is
    # resolved from the env and returned by install_capabilities.
    manifest = _manifest({"fake-builtin": {"entrypoint": "fake_builtin_cap:FakeBuiltin"}})
    classes = install_capabilities(manifest, root=tmp_path / "no-such-dir")
    assert classes == [fake_builtin_module]


def test_no_builtins_resolves_to_empty():
    assert resolve_builtin_capabilities(_manifest({})) == []


def test_broken_builtin_entrypoint_is_bootstrap_error():
    manifest = _manifest({"missing-builtin": {"entrypoint": "no_such_module:Class"}})
    with pytest.raises(BootstrapError) as exc:
        resolve_builtin_capabilities(manifest)
    # The built-in name lands in structured details (and the JSON error line).
    assert exc.value.details["capability"] == "missing-builtin"
    assert "failed to import" in str(exc.value)
    assert "missing-builtin" in exc.value.to_json()


def test_builtin_missing_entrypoint_is_bootstrap_error():
    manifest = _manifest({"bad-builtin": {"serializationName": "flokoa.X"}})  # no entrypoint
    with pytest.raises(BootstrapError) as exc:
        resolve_builtin_capabilities(manifest)
    assert exc.value.details["capability"] == "bad-builtin"


def test_real_flokoa_openapi_builtin_resolves_from_env():
    # The flokoa-openapi built-in is baked into the runner env; resolve it the
    # same way the runner does at bootstrap.
    manifest = _manifest(
        {
            "flokoa-openapi": {
                "entrypoint": "flokoa_openapi.capability:OpenAPI",
                "serializationName": "flokoa.OpenAPI",
            }
        }
    )
    classes = resolve_builtin_capabilities(manifest)
    assert len(classes) == 1
    assert classes[0].get_serialization_name() == "flokoa.OpenAPI"
