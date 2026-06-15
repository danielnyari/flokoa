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
from flokoa.capability_cli import git_auth as git_auth_mod
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


CLONE_REPORT = {
    "url": "https://github.com/org/repo",
    "ref": "v1.2.0",
    "commit": "a" * 40,
    "subdirectory": None,
}


class FakeSession:
    """Stands in for ContainerSession; materializes the work-dir outputs.

    Records each step's argv AND its secret_env so the git-tier tests can prove
    the token is injected via exec(secret_env=…) and never appears in argv.
    """

    def __init__(
        self,
        work_dir: Path,
        *,
        wheelhouse_report: dict[str, Any] | None = None,
        schema_report: dict[str, Any] | None = None,
        clone_report: dict[str, Any] | None = None,
        clone_fails: bool = False,
    ) -> None:
        self.work_dir = work_dir
        self.wheelhouse_report = wheelhouse_report or WHEELHOUSE_REPORT
        self.schema_report = schema_report or SCHEMA_REPORT
        self.clone_report = clone_report or CLONE_REPORT
        self.clone_fails = clone_fails
        self.steps: list[tuple[str, list[str]]] = []
        self.secret_envs: list[tuple[str, dict[str, str] | None]] = []

    def __enter__(self) -> FakeSession:
        return self

    def __exit__(self, *exc_info: object) -> None:
        return None

    def exec(self, argv: list[str], *, step: str, secret_env: dict[str, str] | None = None) -> None:
        self.steps.append((step, argv))
        self.secret_envs.append((step, secret_env))
        if step == "baseline freeze":
            (self.work_dir / "constraints.txt").write_text("pydantic-ai==1.107.0\n")
            (self.work_dir / "runner-manifest.json").write_text(json.dumps(RUNNER_MANIFEST))
        elif step == "git clone":
            if self.clone_fails:
                from flokoa.capability_cli.errors import CapabilityCliError

                raise CapabilityCliError("git clone failed inside img:\nfatal: could not read Username")
            (self.work_dir / "clone-report.json").write_text(json.dumps(self.clone_report))
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
    """Run the build command with the container layer faked out.

    Build-image resolution is left REAL (container.resolve_build_image), so the
    image the build resolves to is captured from the session factory kwargs —
    base-image precedence can be asserted end-to-end without a container.
    """
    src = tmp_path / "src-project"
    src.mkdir()
    output = tmp_path / "dist"
    sessions: list[FakeSession] = []
    images: list[str] = []

    def _invoke(
        *extra_args: str,
        wheelhouse_report: dict[str, Any] | None = None,
        schema_report: dict[str, Any] | None = None,
        clone_report: dict[str, Any] | None = None,
        clone_fails: bool = False,
        with_path: bool = True,
    ):
        def session_factory(**kwargs: Any) -> FakeSession:
            images.append(kwargs.get("image", ""))
            session = FakeSession(
                output / ".build",
                wheelhouse_report=wheelhouse_report,
                schema_report=schema_report,
                clone_report=clone_report,
                clone_fails=clone_fails,
            )
            sessions.append(session)
            return session

        # Clean any base-image env so the default-resolution assertions hold.
        for var in ("FLOKOA_CAPABILITY_BASE_IMAGE", "FLOKOA_CAPABILITY_BASE_REPOSITORY"):
            monkeypatch.delenv(var, raising=False)
        # Clean git-token env so auth assertions are deterministic (a test that
        # wants the env fallback sets GITHUB_TOKEN/GH_TOKEN explicitly).
        for var in ("GITHUB_TOKEN", "GH_TOKEN", "SSH_AUTH_SOCK"):
            monkeypatch.delenv(var, raising=False)
        monkeypatch.setattr(build_mod.container_mod, "detect_container_tool", lambda: "docker")
        monkeypatch.setattr(build_mod.container_mod, "ContainerSession", session_factory)
        monkeypatch.setattr(build_mod.artifact_mod, "build_oci_archive", mock.Mock())
        args = [str(src)] if with_path else []
        args += ["--output", str(output), *extra_args]
        result = CliRunner().invoke(build, args)
        return result, output, sessions, images

    return _invoke


class TestBuildCommand:
    def test_happy_path_writes_all_outputs(self, invoke) -> None:
        result, output, sessions, _ = invoke()
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

        # A PATH build records source: image (the author-built-their-own case)
        # with no provenance, in both the manifest and the CR.
        assert manifest["source"] == "image"
        assert "provenance" not in manifest

        cr_doc = yaml.safe_load((output / "flokoa-cap-echo.capability.yaml").read_text())
        assert cr_doc["spec"]["artifact"] == "flokoa-cap-echo:0.1.0@sha256:DIGEST-PENDING"
        assert cr_doc["spec"]["source"] == "image"
        assert "provenance" not in cr_doc["spec"]
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
        result, _, _, _ = invoke("--from-pypi", "pkg==1.0", "--allow-pypi")
        assert result.exit_code != 0
        assert "exactly one of PATH, --from-pypi, or --from-git" in result.output

    def test_path_and_from_git_mutually_exclusive(self, invoke) -> None:
        result, _, _, _ = invoke("--from-git", "git+https://github.com/org/repo")
        assert result.exit_code != 0
        assert "exactly one of PATH, --from-pypi, or --from-git" in result.output

    def test_from_pypi_and_from_git_mutually_exclusive(self, invoke) -> None:
        result, _, _, _ = invoke(
            "--from-pypi", "pkg==1.0", "--allow-pypi", "--from-git", "git+https://github.com/org/repo", with_path=False
        )
        assert result.exit_code != 0
        assert "exactly one of PATH, --from-pypi, or --from-git" in result.output

    def test_no_source(self, invoke) -> None:
        result, _, _, _ = invoke(with_path=False)
        assert result.exit_code != 0
        assert "exactly one of PATH, --from-pypi, or --from-git" in result.output

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
        result, _, _, _ = invoke("--from-pypi", value, with_path=False)
        assert result.exit_code != 0
        assert "must be a PyPI package name" in result.output

    @pytest.mark.parametrize("value", ["demo-pkg", "Demo_pkg.plugin2", "demo-pkg==1.0.0", "pkg==2024.1.post1"])
    def test_from_pypi_accepts_name_and_pin(self, invoke, value: str) -> None:
        result, _, sessions, _ = invoke("--from-pypi", value, "--allow-pypi", with_path=False)
        assert result.exit_code == 0, result.output
        wheelhouse_argv = next(argv for step, argv in sessions[0].steps if step == "wheelhouse build")
        assert wheelhouse_argv[wheelhouse_argv.index("--from-pypi") + 1] == value

    def test_schema_and_permissive_mutually_exclusive(self, tmp_path: Path, invoke) -> None:
        schema_file = tmp_path / "schema.json"
        schema_file.write_text("{}")
        result, _, _, _ = invoke("--schema", str(schema_file), "--permissive")
        assert result.exit_code != 0
        assert "mutually exclusive" in result.output

    def test_invalid_entrypoint_format_refused(self, invoke) -> None:
        result, _, _, _ = invoke("--entrypoint", "no-colon-here")
        assert result.exit_code != 0
        assert "module:attr" in result.output

    def test_underivable_refused_without_flags(self, invoke) -> None:
        report = {**SCHEMA_REPORT, "outcome": "underivable", "schema": None, "reason": "constructor takes **kwargs"}
        result, _, _, _ = invoke(schema_report=report)
        assert result.exit_code != 0
        assert "config schema is underivable" in result.output
        assert "--permissive" in result.output

    def test_underivable_with_permissive_warns_loudly(self, invoke) -> None:
        report = {**SCHEMA_REPORT, "outcome": "underivable", "schema": None, "reason": "untyped"}
        result, output, _, _ = invoke("--permissive", schema_report=report)
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
        result, _, _, _ = invoke(schema_report=report)
        assert result.exit_code != 0
        assert "--entrypoint pkg:AlphaCapability" in result.output
        assert "--entrypoint pkg:BetaCapability" in result.output

    def test_explicit_schema_file_used_verbatim(self, tmp_path: Path, invoke) -> None:
        schema_file = tmp_path / "schema.json"
        schema_file.write_text(json.dumps({"type": "object", "properties": {"custom": {"type": "integer"}}}))
        result, output, _, _ = invoke("--schema", str(schema_file))
        assert result.exit_code == 0, result.output
        manifest = json.loads((output / "manifest.json").read_text())
        assert manifest["configSchema"]["properties"] == {"custom": {"type": "integer"}}

    def test_skip_smoke_test_warns_and_skips(self, invoke) -> None:
        result, _, sessions, _ = invoke("--skip-smoke-test")
        assert result.exit_code == 0, result.output
        assert "skips the install/import gate" in result.output
        steps = [step for step, _ in sessions[0].steps]
        assert "smoke test" not in steps
        assert "entrypoint smoke test" not in steps

    def test_explicit_entrypoint_and_schema_skip_derivation(self, tmp_path: Path, invoke) -> None:
        schema_file = tmp_path / "schema.json"
        schema_file.write_text(json.dumps({"type": "object"}))
        result, _, sessions, _ = invoke("--entrypoint", "flokoa_cap_echo:EchoCapability", "--schema", str(schema_file))
        assert result.exit_code == 0, result.output
        steps = [step for step, _ in sessions[0].steps]
        assert "schema derivation" not in steps
        assert steps == ["baseline freeze", "wheelhouse build", "smoke test"]

    def test_name_override_and_tag_default(self, invoke) -> None:
        result, output, _, _ = invoke("--name", "echo-cap")
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
        result, output, sessions, _ = invoke(wheelhouse_report=report)
        assert result.exit_code == 0, result.output
        smoke_argv = next(argv for step, argv in sessions[0].steps if step == "smoke test")
        assert smoke_argv[smoke_argv.index("--dependency") :][:2] == ["--dependency", "inflection==0.5.1"]
        manifest = json.loads((output / "manifest.json").read_text())
        assert manifest["dependencies"] == ["inflection==0.5.1"]
        assert len(manifest["wheels"]) == 2


class TestPypiDangerGate:
    """--from-pypi is the EXTREMELY DANGEROUS tier: gated behind --allow-pypi,
    loud banner, source: pypi stamp + provenance.pypi.requirement."""

    def test_from_pypi_without_allow_pypi_is_usage_error(self, invoke) -> None:
        result, _, _, _ = invoke("--from-pypi", "demo-pkg==1.0.0", with_path=False)
        assert result.exit_code != 0
        assert "EXTREMELY DANGEROUS" in result.output
        assert "--allow-pypi" in result.output

    def test_from_pypi_with_allow_pypi_warns_and_stamps_pypi(self, invoke) -> None:
        result, output, _, _ = invoke("--from-pypi", "demo-pkg==1.0.0", "--allow-pypi", with_path=False)
        assert result.exit_code == 0, result.output
        # Loud banner (printed to stderr; CliRunner merges into .output).
        assert "EXTREMELY DANGEROUS" in result.output
        manifest = json.loads((output / "manifest.json").read_text())
        assert manifest["source"] == "pypi"
        assert manifest["provenance"]["pypi"]["requirement"] == "demo-pkg==1.0.0"
        cr_doc = yaml.safe_load((output / "flokoa-cap-echo.capability.yaml").read_text())
        assert cr_doc["spec"]["source"] == "pypi"
        assert cr_doc["spec"]["provenance"]["pypi"]["requirement"] == "demo-pkg==1.0.0"

    def test_pypi_cr_validates_through_generated_model(self, invoke) -> None:
        from flokoa_types.capability import CapabilitySpec

        result, output, _, _ = invoke("--from-pypi", "demo-pkg==1.0.0", "--allow-pypi", with_path=False)
        assert result.exit_code == 0, result.output
        cr_doc = yaml.safe_load((output / "flokoa-cap-echo.capability.yaml").read_text())
        spec = {**cr_doc["spec"], "artifact": cr_doc["spec"]["artifact"].replace("DIGEST-PENDING", "0" * 64)}
        validated = CapabilitySpec.model_validate(spec)
        assert validated.source is not None
        assert validated.source.value == "pypi"


def _patch_git_auth(monkeypatch: pytest.MonkeyPatch, auth: git_auth_mod.GitAuth) -> None:
    """Force a specific resolved GitAuth (no real git/gh/network/token)."""
    monkeypatch.setattr(build_mod.git_auth_mod, "resolve_auth", lambda parsed: auth)


class TestGitTier:
    """--from-git: grammar, clone wiring, ephemeral token injection (proven out
    of argv/logs/CR), clean URL recorded, source: git + provenance.git.commit."""

    GIT_URL = "git+https://github.com/org/repo@v1.2.0"
    TOKEN = "ghp_secret_token_value"

    @pytest.mark.parametrize(
        "value",
        [
            "https://github.com/org/repo",  # bare https
            "git+ftp://github.com/org/repo",  # bad scheme
            "git+https://github.com/org/repo[extra]",  # extras
            "git+https://github.com/org/repo --extra-index-url https://evil",  # pip option
            "git+https://user:tok@github.com/org/repo",  # embedded creds
        ],
    )
    def test_from_git_grammar_rejects(self, invoke, value: str) -> None:
        result, _, _, _ = invoke("--from-git", value, with_path=False)
        assert result.exit_code != 0
        assert "--from-git must be" in result.output

    def test_token_injected_via_secret_env_never_in_argv(self, invoke, monkeypatch: pytest.MonkeyPatch) -> None:
        _patch_git_auth(monkeypatch, git_auth_mod.GitAuth(token=self.TOKEN, source="$GITHUB_TOKEN"))
        result, _, sessions, _ = invoke("--from-git", self.GIT_URL, with_path=False)
        assert result.exit_code == 0, result.output

        # The clone step carries the token ONLY in secret_env — never in argv.
        clone_secret = next(env for step, env in sessions[0].secret_envs if step == "git clone")
        assert clone_secret == {"GIT_ASKPASS_TOKEN": self.TOKEN}
        clone_argv = next(argv for step, argv in sessions[0].steps if step == "git clone")
        assert all(self.TOKEN not in arg for arg in clone_argv), "token must never appear in argv"
        # And it appears in NO step's argv, anywhere.
        for _step, argv in sessions[0].steps:
            assert all(self.TOKEN not in arg for arg in argv)

    def test_token_never_in_output_or_cr(self, invoke, monkeypatch: pytest.MonkeyPatch) -> None:
        _patch_git_auth(monkeypatch, git_auth_mod.GitAuth(token=self.TOKEN, source="$GITHUB_TOKEN"))
        result, output, _, _ = invoke("--from-git", self.GIT_URL, with_path=False)
        assert result.exit_code == 0, result.output
        # Redaction assertion: the token leaks into nothing the user/CI can read.
        assert self.TOKEN not in result.output
        manifest_text = (output / "manifest.json").read_text()
        cr_text = (output / "flokoa-cap-echo.capability.yaml").read_text()
        assert self.TOKEN not in manifest_text
        assert self.TOKEN not in cr_text

    def test_clean_url_and_provenance_recorded(self, invoke, monkeypatch: pytest.MonkeyPatch) -> None:
        _patch_git_auth(monkeypatch, git_auth_mod.GitAuth(token=self.TOKEN, source="$GITHUB_TOKEN"))
        result, output, sessions, _ = invoke("--from-git", self.GIT_URL, with_path=False)
        assert result.exit_code == 0, result.output

        # The clone step is handed the CLEAN url (no git+ prefix, no creds).
        clone_argv = next(argv for step, argv in sessions[0].steps if step == "git clone")
        url = clone_argv[clone_argv.index("--url") + 1]
        assert url == "https://github.com/org/repo"
        assert clone_argv[clone_argv.index("--ref") + 1] == "v1.2.0"

        manifest = json.loads((output / "manifest.json").read_text())
        assert manifest["source"] == "git"
        assert manifest["provenance"]["git"]["url"] == "https://github.com/org/repo"
        assert manifest["provenance"]["git"]["commit"] == "a" * 40
        assert manifest["provenance"]["git"]["ref"] == "v1.2.0"

    def test_source_git_lands_in_cr_and_validates(self, invoke, monkeypatch: pytest.MonkeyPatch) -> None:
        from flokoa_types.capability import CapabilitySpec

        _patch_git_auth(monkeypatch, git_auth_mod.GitAuth(token=self.TOKEN))
        result, output, _, _ = invoke("--from-git", self.GIT_URL, with_path=False)
        assert result.exit_code == 0, result.output
        cr_doc = yaml.safe_load((output / "flokoa-cap-echo.capability.yaml").read_text())
        assert cr_doc["spec"]["source"] == "git"
        assert cr_doc["spec"]["provenance"]["git"]["commit"] == "a" * 40
        assert cr_doc["spec"]["provenance"]["git"]["url"] == "https://github.com/org/repo"
        spec = {**cr_doc["spec"], "artifact": cr_doc["spec"]["artifact"].replace("DIGEST-PENDING", "0" * 64)}
        validated = CapabilitySpec.model_validate(spec)
        assert validated.source is not None
        assert validated.source.value == "git"
        assert validated.provenance is not None
        assert validated.provenance.git is not None
        assert validated.provenance.git.commit == "a" * 40

    def test_wheelhouse_src_points_at_clone(self, invoke, monkeypatch: pytest.MonkeyPatch) -> None:
        _patch_git_auth(monkeypatch, git_auth_mod.GitAuth(token=self.TOKEN))
        result, _, sessions, _ = invoke("--from-git", self.GIT_URL, with_path=False)
        assert result.exit_code == 0, result.output
        wheelhouse_argv = next(argv for step, argv in sessions[0].steps if step == "wheelhouse build")
        assert wheelhouse_argv[wheelhouse_argv.index("--src") + 1] == "/work/clone"

    def test_subdirectory_threads_into_clone_and_src(self, invoke, monkeypatch: pytest.MonkeyPatch) -> None:
        _patch_git_auth(monkeypatch, git_auth_mod.GitAuth(token=self.TOKEN))
        report = {**CLONE_REPORT, "subdirectory": "pkg/cap"}
        result, output, sessions, _ = invoke(
            "--from-git",
            "git+https://github.com/org/repo@v1#subdirectory=pkg/cap",
            clone_report=report,
            with_path=False,
        )
        assert result.exit_code == 0, result.output
        clone_argv = next(argv for step, argv in sessions[0].steps if step == "git clone")
        assert clone_argv[clone_argv.index("--subdirectory") + 1] == "pkg/cap"
        wheelhouse_argv = next(argv for step, argv in sessions[0].steps if step == "wheelhouse build")
        assert wheelhouse_argv[wheelhouse_argv.index("--src") + 1] == "/work/clone/pkg/cap"
        manifest = json.loads((output / "manifest.json").read_text())
        assert manifest["provenance"]["git"]["subdirectory"] == "pkg/cap"

    def test_ambient_token_preferred_over_env(self, invoke, monkeypatch: pytest.MonkeyPatch) -> None:
        # resolve_auth (mocked here) is what enforces ambient-first; this proves
        # whatever it returns is what reaches secret_env, unaltered.
        _patch_git_auth(monkeypatch, git_auth_mod.GitAuth(token="ambient-pat", source="ambient git credential helper"))
        result, _, sessions, _ = invoke("--from-git", self.GIT_URL, with_path=False)
        assert result.exit_code == 0, result.output
        clone_secret = next(env for step, env in sessions[0].secret_envs if step == "git clone")
        assert clone_secret == {"GIT_ASKPASS_TOKEN": "ambient-pat"}

    def test_public_repo_no_credential_no_secret_env(self, invoke, monkeypatch: pytest.MonkeyPatch) -> None:
        _patch_git_auth(monkeypatch, git_auth_mod.GitAuth(token=None, ssh_auth_sock=None, source="none"))
        result, _, sessions, _ = invoke("--from-git", self.GIT_URL, with_path=False)
        assert result.exit_code == 0, result.output
        clone_secret = next(env for step, env in sessions[0].secret_envs if step == "git clone")
        assert clone_secret is None  # nothing injected for a public clone

    def test_ssh_forwards_agent_socket_no_token(self, invoke, monkeypatch: pytest.MonkeyPatch) -> None:
        _patch_git_auth(monkeypatch, git_auth_mod.GitAuth(ssh_auth_sock="/run/agent.sock", source="host SSH agent"))
        result, _, sessions, _ = invoke("--from-git", "git+ssh://git@github.com/org/repo", with_path=False)
        assert result.exit_code == 0, result.output
        clone_argv = next(argv for step, argv in sessions[0].steps if step == "git clone")
        # The agent socket path is passed inline via env (not a secret), strict ssh.
        assert "SSH_AUTH_SOCK=/flokoa-ssh-agent.sock" in clone_argv
        assert any("StrictHostKeyChecking=yes" in arg for arg in clone_argv)
        clone_secret = next(env for step, env in sessions[0].secret_envs if step == "git clone")
        assert clone_secret is None  # ssh uses the agent socket, no token

    def test_no_credential_clone_failure_names_precedence(self, invoke, monkeypatch: pytest.MonkeyPatch) -> None:
        _patch_git_auth(monkeypatch, git_auth_mod.GitAuth(token=None, ssh_auth_sock=None, source="none"))
        result, _, _, _ = invoke("--from-git", self.GIT_URL, clone_fails=True, with_path=False)
        assert result.exit_code != 0
        # The env-var names the user can set.
        assert "GITHUB_TOKEN" in result.output
        assert "GH_TOKEN" in result.output
        # The ambient methods (git credential fill → gh auth token) must also be
        # named so the user knows they can resolve auth without leaking a token
        # via an env var. These strings come from auth_failure_message() which is
        # appended to the clone-error output when no credential resolved.
        assert "git credential" in result.output
        assert "gh auth token" in result.output


class TestBuildImageResolution:
    """The build command resolves the build image to the base image by default,
    honoring the new --base-image/--base-version flags and the retained
    --runner-image/--runner-version aliases (precedence proven end-to-end)."""

    def test_default_build_image_is_capability_base(self, invoke) -> None:
        result, _, _, images = invoke()
        assert result.exit_code == 0, result.output
        from flokoa.capability_cli import container

        assert images[0] == f"{container.DEFAULT_CAPABILITY_BASE_REPOSITORY}:{container.DEFAULT_RUNNER_VERSION}"

    def test_base_image_override(self, invoke) -> None:
        result, _, _, images = invoke("--base-image", "example.com/base:dev")
        assert result.exit_code == 0, result.output
        assert images[0] == "example.com/base:dev"

    def test_base_version_composes_with_base_repo(self, invoke) -> None:
        from flokoa.capability_cli import container

        result, _, _, images = invoke("--base-version", "0.3.0")
        assert result.exit_code == 0, result.output
        assert images[0] == f"{container.DEFAULT_CAPABILITY_BASE_REPOSITORY}:0.3.0"

    def test_base_image_beats_legacy_runner_image(self, invoke) -> None:
        result, _, _, images = invoke("--base-image", "example.com/base:dev", "--runner-image", "old/runner:legacy")
        assert result.exit_code == 0, result.output
        assert images[0] == "example.com/base:dev"

    def test_legacy_runner_version_alias_composes_with_base_repo(self, invoke) -> None:
        from flokoa.capability_cli import container

        result, _, _, images = invoke("--runner-version", "0.3.0")
        assert result.exit_code == 0, result.output
        assert images[0] == f"{container.DEFAULT_CAPABILITY_BASE_REPOSITORY}:0.3.0"


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
