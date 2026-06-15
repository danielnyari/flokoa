"""Integration: `flokoa capability build` end-to-end against real docker.

Opt-in only (``pytest -m integration``; see conftest) and docker-gated.
Builds the chunk-1 echo fixture through the CLI and checks:

  * artifact tar + manifest + CR exist,
  * the manifest passes the shipped v1 schema AND the runner's own
    bootstrap-time enforcement (``flokoa_runner.capabilities``),
  * field parity with the fixture's hand-rolled ``artifact.json``
    (name/version/entrypoint/requires/dependencies),
  * an unimportable package is refused by the smoke test.

The runner image defaults to the SDK pin; CI (and laptops without ghcr
access) override via ``FLOKOA_RUNNER_IMAGE`` after building
``flokoa-runner/Dockerfile`` locally.
"""

from __future__ import annotations

import json
import tarfile
from pathlib import Path

import pytest
import yaml
from click.testing import CliRunner

from flokoa.capability_cli.artifact import validate_manifest_dict
from flokoa.capability_cli.build import build

pytestmark = pytest.mark.integration

REPO_ROOT = Path(__file__).parents[6]
ECHO_FIXTURE = REPO_ROOT / "operator" / "test" / "e2e" / "fixtures" / "capabilities" / "echo"


@pytest.fixture(scope="module")
def echo_build(tmp_path_factory: pytest.TempPathFactory) -> Path:
    """Build the echo fixture once for the assertions below."""
    if not ECHO_FIXTURE.is_dir():
        pytest.skip("echo fixture checkout not available")
    output = tmp_path_factory.mktemp("echo-dist")
    result = CliRunner().invoke(
        build,
        [
            str(ECHO_FIXTURE),
            "--tag",
            "flokoa-cap-echo:integration",
            "--output",
            str(output),
        ],
        catch_exceptions=False,
    )
    assert result.exit_code == 0, result.output
    return output


class TestEchoFixtureBuild:
    def test_outputs_exist(self, echo_build: Path) -> None:
        assert (echo_build / "flokoa-cap-echo-artifact.oci.tar").is_file()
        assert (echo_build / "manifest.json").is_file()
        assert (echo_build / "flokoa-cap-echo.capability.yaml").is_file()
        assert (echo_build / "config-schema.json").is_file()

    def test_artifact_tar_is_oci_layout(self, echo_build: Path) -> None:
        with tarfile.open(echo_build / "flokoa-cap-echo-artifact.oci.tar") as tar:
            names = tar.getnames()
        assert any(name.rstrip("/").endswith("oci-layout") for name in names), names

    def test_manifest_passes_published_schema(self, echo_build: Path) -> None:
        manifest = json.loads((echo_build / "manifest.json").read_text())
        validate_manifest_dict(manifest)

    def test_manifest_parity_with_fixture_artifact_json(self, echo_build: Path) -> None:
        """The CLI and the chunk-1 build.sh path must agree on the mirror fields."""
        manifest = json.loads((echo_build / "manifest.json").read_text())
        fixture = json.loads((ECHO_FIXTURE / "artifact.json").read_text())
        for field in ("name", "version", "entrypoint", "requires", "dependencies"):
            assert manifest[field] == fixture[field], f"manifest field {field!r} diverges from the fixture"
        assert manifest["contractVersion"] == fixture["contractVersion"]

    def test_wheelhouse_passes_runner_enforcement(self, echo_build: Path) -> None:
        """The build output must satisfy what the runner enforces at bootstrap
        (manifest shape, requires tuple, wheel integrity) — minus the actual
        pip install."""
        flokoa_runner_capabilities = pytest.importorskip("flokoa_runner.capabilities")
        flokoa_runner_manifest = pytest.importorskip("flokoa_runner.manifest")

        wheelhouse = echo_build / ".build" / "wheelhouse"
        assert wheelhouse.is_dir()
        # The manifest of the actual build image, copied out by freeze_baseline.
        runner_manifest = flokoa_runner_manifest.RunnerManifest.from_dict(
            json.loads((echo_build / ".build" / "runner-manifest.json").read_text())
        )
        manifest = flokoa_runner_capabilities._load_capability_manifest(wheelhouse, runner_manifest)
        flokoa_runner_capabilities._verify_requires(wheelhouse.name, manifest, runner_manifest)
        flokoa_runner_capabilities._verify_wheelhouse(wheelhouse, manifest)

    def test_capability_cr_shape(self, echo_build: Path) -> None:
        doc = yaml.safe_load((echo_build / "flokoa-cap-echo.capability.yaml").read_text())
        assert doc["kind"] == "Capability"
        assert doc["metadata"]["name"] == "flokoa-cap-echo"
        assert doc["spec"]["artifact"] == "flokoa-cap-echo:integration@sha256:DIGEST-PENDING"
        assert doc["spec"]["entrypoint"] == "flokoa_cap_echo:EchoCapability"
        assert doc["spec"]["requires"] == {"python": "3.13", "pydanticAI": ">=1.107,<2", "flokoaRunner": ">=0.2"}
        # Echo's config schema is derived from the dataclass: the prefix field.
        assert "prefix" in doc["spec"]["configSchema"]["properties"]
        assert "schemaPolicy" not in doc["spec"]


class TestSmokeFailurePath:
    def test_unimportable_package_is_refused(self, tmp_path: Path) -> None:
        """A capability that can't import never gets an artifact."""
        project = tmp_path / "broken-cap"
        package = project / "src" / "broken_cap"
        package.mkdir(parents=True)
        (project / "pyproject.toml").write_text(
            """\
[project]
name = "broken-cap"
version = "0.1.0"
requires-python = ">=3.13"
dependencies = []

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[tool.hatch.build.targets.wheel]
packages = ["src/broken_cap"]
"""
        )
        (package / "__init__.py").write_text("import module_that_does_not_exist\n")

        output = tmp_path / "dist"
        result = CliRunner().invoke(
            build,
            [str(project), "--entrypoint", "broken_cap:BrokenCapability", "--permissive", "--output", str(output)],
        )
        assert result.exit_code != 0
        assert "smoke test failed" in result.output
        assert "module_that_does_not_exist" in result.output
        assert not (output / "broken-cap-artifact.oci.tar").exists()
        assert not (output / "broken-cap.capability.yaml").exists()
