"""``flokoa capability build``: orchestration with a mocked container session.

The fake session executes no containers — it writes the work-dir reports the
_inrunner scripts would produce, so the host-side orchestration (outcome
policy, manifest/CR/Dockerfile emission, warnings) is exercised hermetically.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any
from unittest import mock

import pytest
import yaml
from click.testing import CliRunner

from flokoa.capability_cli import artifact as artifact_mod
from flokoa.capability_cli import build as build_mod
from flokoa.capability_cli.build import _derive_requires, build

RUNNER_MANIFEST = {
    "contractVersion": 1,
    "runnerVersion": "0.2.0",
    "python": "3.13",
    "pydantic-ai": "1.107.0",
}

WHEELHOUSE_REPORT = {
    "name": "flokoa-cap-echo",
    "version": "0.1.0",
    "wheels": ["flokoa_cap_echo-0.1.0-py3-none-any.whl"],
    "dependencies": [],
}

SCHEMA_REPORT = {
    "outcome": "derived",
    "entrypoint": "flokoa_cap_echo:EchoCapability",
    "serializationName": None,
    "schema": {"type": "object", "properties": {"prefix": {"type": "string", "default": "echo"}}},
    "reason": None,
    "candidates": ["flokoa_cap_echo:EchoCapability"],
}


class FakeSession:
    """Stands in for ContainerSession; materializes the work-dir outputs."""

    def __init__(
        self,
        work_dir: Path,
        *,
        wheelhouse_report: dict[str, Any] | None = None,
        schema_report: dict[str, Any] | None = None,
    ) -> None:
        self.work_dir = work_dir
        self.wheelhouse_report = wheelhouse_report or WHEELHOUSE_REPORT
        self.schema_report = schema_report or SCHEMA_REPORT
        self.steps: list[tuple[str, list[str]]] = []

    def __enter__(self) -> FakeSession:
        return self

    def __exit__(self, *exc_info: object) -> None:
        return None

    def exec(self, argv: list[str], *, step: str) -> None:
        self.steps.append((step, argv))
        if step == "baseline freeze":
            (self.work_dir / "constraints.txt").write_text("pydantic-ai==1.107.0\n")
            (self.work_dir / "runner-manifest.json").write_text(json.dumps(RUNNER_MANIFEST))
        elif step == "wheelhouse build":
            wheelhouse = self.work_dir / "wheelhouse"
            wheelhouse.mkdir(parents=True, exist_ok=True)
            for wheel in self.wheelhouse_report["wheels"]:
                (wheelhouse / wheel).write_bytes(b"not-a-real-wheel")
            (self.work_dir / "wheelhouse-report.json").write_text(json.dumps(self.wheelhouse_report))
        elif step in ("smoke test", "entrypoint smoke test"):
            (self.work_dir / "smoke-report.json").write_text(
                json.dumps({"installed": True, "imported": None, "instantiated": True, "warning": None})
            )
        elif step == "schema derivation":
            (self.work_dir / "schema-report.json").write_text(json.dumps(self.schema_report))


@pytest.fixture
def invoke(tmp_path: Path, monkeypatch: pytest.MonkeyPatch):
    """Run the build command with the container layer faked out."""
    src = tmp_path / "src-project"
    src.mkdir()
    output = tmp_path / "dist"
    sessions: list[FakeSession] = []

    def _invoke(
        *extra_args: str,
        wheelhouse_report: dict[str, Any] | None = None,
        schema_report: dict[str, Any] | None = None,
        with_path: bool = True,
    ):
        def session_factory(**_kwargs: Any) -> FakeSession:
            session = FakeSession(output / ".build", wheelhouse_report=wheelhouse_report, schema_report=schema_report)
            sessions.append(session)
            return session

        monkeypatch.setattr(build_mod.container_mod, "detect_container_tool", lambda: "docker")
        monkeypatch.setattr(build_mod.container_mod, "ContainerSession", session_factory)
        monkeypatch.setattr(build_mod.artifact_mod, "build_oci_archive", mock.Mock())
        args = [str(src)] if with_path else []
        args += ["--output", str(output), *extra_args]
        result = CliRunner().invoke(build, args)
        return result, output, sessions

    return _invoke


class TestBuildCommand:
    def test_happy_path_writes_all_outputs(self, invoke) -> None:
        result, output, sessions = invoke()
        assert result.exit_code == 0, result.output
        assert (output / "flokoa-cap-echo-artifact.oci.tar").exists() is False  # oci build mocked
        assert (output / "manifest.json").is_file()
        assert (output / "config-schema.json").is_file()
        assert (output / "flokoa-cap-echo.capability.yaml").is_file()
        assert (output / ".build" / "wheelhouse" / "manifest.json").is_file()
        assert (output / ".build" / "Dockerfile").is_file()

        manifest = json.loads((output / "manifest.json").read_text())
        assert manifest["name"] == "flokoa-cap-echo"
        assert manifest["entrypoint"] == "flokoa_cap_echo:EchoCapability"
        assert manifest["requires"] == {"python": "3.13", "pydantic-ai": ">=1.107,<2", "flokoa-runner": ">=0.2"}
        assert manifest["schemaDigest"].startswith("sha256:")
        artifact_mod.validate_manifest_dict(manifest)

        cr_doc = yaml.safe_load((output / "flokoa-cap-echo.capability.yaml").read_text())
        assert cr_doc["spec"]["artifact"] == "flokoa-cap-echo:0.1.0@sha256:DIGEST-PENDING"
        assert cr_doc["spec"]["configSchema"]["properties"]["prefix"]["type"] == "string"

        steps = [step for step, _ in sessions[0].steps]
        assert steps == [
            "baseline freeze",
            "wheelhouse build",
            "smoke test",
            "schema derivation",
            "entrypoint smoke test",
        ]

    def test_path_and_from_pypi_mutually_exclusive(self, invoke) -> None:
        result, _, _ = invoke("--from-pypi", "pkg==1.0")
        assert result.exit_code != 0
        assert "exactly one of PATH or --from-pypi" in result.output

    def test_neither_path_nor_from_pypi(self, invoke) -> None:
        result, _, _ = invoke(with_path=False)
        assert result.exit_code != 0
        assert "exactly one of PATH or --from-pypi" in result.output

    @pytest.mark.parametrize(
        "value",
        [
            "git+https://github.com/evil/pkg.git",
            "https://evil.example/pkg-1.0.tar.gz",
            "pkg --extra-index-url https://evil.example/simple",
            "pkg[extra]==1.0",
            "pkg==1.0; python_version>'3'",
            "-r requirements.txt",
        ],
    )
    def test_from_pypi_rejects_non_name_requirements(self, invoke, value: str) -> None:
        result, _, _ = invoke("--from-pypi", value, with_path=False)
        assert result.exit_code != 0
        assert "must be a PyPI package name" in result.output

    @pytest.mark.parametrize("value", ["demo-pkg", "Demo_pkg.plugin2", "demo-pkg==1.0.0", "pkg==2024.1.post1"])
    def test_from_pypi_accepts_name_and_pin(self, invoke, value: str) -> None:
        result, _, sessions = invoke("--from-pypi", value, with_path=False)
        assert result.exit_code == 0, result.output
        wheelhouse_argv = next(argv for step, argv in sessions[0].steps if step == "wheelhouse build")
        assert wheelhouse_argv[wheelhouse_argv.index("--from-pypi") + 1] == value

    def test_schema_and_permissive_mutually_exclusive(self, tmp_path: Path, invoke) -> None:
        schema_file = tmp_path / "schema.json"
        schema_file.write_text("{}")
        result, _, _ = invoke("--schema", str(schema_file), "--permissive")
        assert result.exit_code != 0
        assert "mutually exclusive" in result.output

    def test_invalid_entrypoint_format_refused(self, invoke) -> None:
        result, _, _ = invoke("--entrypoint", "no-colon-here")
        assert result.exit_code != 0
        assert "module:attr" in result.output

    def test_underivable_refused_without_flags(self, invoke) -> None:
        report = {**SCHEMA_REPORT, "outcome": "underivable", "schema": None, "reason": "constructor takes **kwargs"}
        result, _, _ = invoke(schema_report=report)
        assert result.exit_code != 0
        assert "config schema is underivable" in result.output
        assert "--permissive" in result.output

    def test_underivable_with_permissive_warns_loudly(self, invoke) -> None:
        report = {**SCHEMA_REPORT, "outcome": "underivable", "schema": None, "reason": "untyped"}
        result, output, _ = invoke("--permissive", schema_report=report)
        assert result.exit_code == 0, result.output
        assert "schemaPolicy: permissive" in result.output
        cr_doc = yaml.safe_load((output / "flokoa-cap-echo.capability.yaml").read_text())
        assert cr_doc["spec"]["schemaPolicy"] == "permissive"
        assert "configSchema" not in cr_doc["spec"]
        manifest = json.loads((output / "manifest.json").read_text())
        assert "configSchema" not in manifest
        assert "schemaDigest" not in manifest

    def test_ambiguous_candidates_listed(self, invoke) -> None:
        report = {
            **SCHEMA_REPORT,
            "outcome": "ambiguous",
            "schema": None,
            "reason": "multiple capability classes exported — pick one with --entrypoint",
            "candidates": ["pkg:AlphaCapability", "pkg:BetaCapability"],
        }
        result, _, _ = invoke(schema_report=report)
        assert result.exit_code != 0
        assert "--entrypoint pkg:AlphaCapability" in result.output
        assert "--entrypoint pkg:BetaCapability" in result.output

    def test_explicit_schema_file_used_verbatim(self, tmp_path: Path, invoke) -> None:
        schema_file = tmp_path / "schema.json"
        schema_file.write_text(json.dumps({"type": "object", "properties": {"custom": {"type": "integer"}}}))
        result, output, _ = invoke("--schema", str(schema_file))
        assert result.exit_code == 0, result.output
        manifest = json.loads((output / "manifest.json").read_text())
        assert manifest["configSchema"]["properties"] == {"custom": {"type": "integer"}}

    def test_skip_smoke_test_warns_and_skips(self, invoke) -> None:
        result, _, sessions = invoke("--skip-smoke-test")
        assert result.exit_code == 0, result.output
        assert "skips the install/import gate" in result.output
        steps = [step for step, _ in sessions[0].steps]
        assert "smoke test" not in steps
        assert "entrypoint smoke test" not in steps

    def test_explicit_entrypoint_and_schema_skip_derivation(self, tmp_path: Path, invoke) -> None:
        schema_file = tmp_path / "schema.json"
        schema_file.write_text(json.dumps({"type": "object"}))
        result, _, sessions = invoke("--entrypoint", "flokoa_cap_echo:EchoCapability", "--schema", str(schema_file))
        assert result.exit_code == 0, result.output
        steps = [step for step, _ in sessions[0].steps]
        assert "schema derivation" not in steps
        assert steps == ["baseline freeze", "wheelhouse build", "smoke test"]

    def test_name_override_and_tag_default(self, invoke) -> None:
        result, output, _ = invoke("--name", "echo-cap")
        assert result.exit_code == 0, result.output
        cr_doc = yaml.safe_load((output / "echo-cap.capability.yaml").read_text())
        assert cr_doc["metadata"]["name"] == "echo-cap"
        assert cr_doc["spec"]["artifact"].startswith("echo-cap:0.1.0@")

    def test_dependencies_forwarded_to_inrunner_steps(self, invoke) -> None:
        report = {
            **WHEELHOUSE_REPORT,
            "dependencies": ["inflection==0.5.1"],
            "wheels": [
                "flokoa_cap_echo-0.1.0-py3-none-any.whl",
                "inflection-0.5.1-py2.py3-none-any.whl",
            ],
        }
        result, output, sessions = invoke(wheelhouse_report=report)
        assert result.exit_code == 0, result.output
        smoke_argv = next(argv for step, argv in sessions[0].steps if step == "smoke test")
        assert smoke_argv[smoke_argv.index("--dependency") :][:2] == ["--dependency", "inflection==0.5.1"]
        manifest = json.loads((output / "manifest.json").read_text())
        assert manifest["dependencies"] == ["inflection==0.5.1"]
        assert len(manifest["wheels"]) == 2


class TestDeriveRequires:
    def test_echo_fixture_parity(self) -> None:
        requires = _derive_requires(RUNNER_MANIFEST)
        assert requires.model_dump(by_alias=True, exclude_none=True) == {
            "python": "3.13",
            "pydantic-ai": ">=1.107,<2",
            "flokoa-runner": ">=0.2",
        }

    def test_partial_manifest(self) -> None:
        requires = _derive_requires({"python": "3.14"})
        assert requires.python == "3.14"
        assert requires.pydantic_ai is None
        assert requires.flokoa_runner is None
