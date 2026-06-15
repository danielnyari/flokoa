"""``flokoa capability import``: build -> schema review -> push orchestration.

build and push are faked at the function boundary (ctx.invoke target callback
and ``run_push``) — the test exercises the review gate UX: pretty-printed
schema, confirm/abort, the permissive prompt, and ``--yes`` for CI.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import pytest
from click.testing import CliRunner

from flokoa.capability_cli import import_cmd as import_mod
from flokoa.capability_cli.import_cmd import import_command
from flokoa.capability_cli.push import PushResult

DIGEST = "sha256:" + "a" * 64
TAG = "ghcr.io/danielnyari/capabilities/pydantic-ai-foo:1.2.0"

DERIVED_SCHEMA = {"type": "object", "properties": {"workspace": {"type": "string", "default": "/workspace"}}}


@pytest.fixture
def harness(tmp_path: Path, monkeypatch: pytest.MonkeyPatch):
    """Fake build (writes the manifest the review reads) and record run_push."""
    output = tmp_path / "dist"
    build_calls: list[dict[str, Any]] = []
    push_calls: list[dict[str, Any]] = []
    state = {"config_schema": DERIVED_SCHEMA}

    def fake_build(**kwargs: Any) -> None:
        build_calls.append(kwargs)
        output.mkdir(parents=True, exist_ok=True)
        manifest: dict[str, Any] = {
            "name": "pydantic-ai-foo",
            "version": "1.2.0",
            "entrypoint": "pydantic_ai_foo:FooCapability",
        }
        if state["config_schema"] is not None:
            manifest["configSchema"] = state["config_schema"]
        (output / "manifest.json").write_text(json.dumps(manifest))

    def fake_run_push(ref: str, **kwargs: Any) -> PushResult:
        push_calls.append({"ref": ref, **kwargs})
        return PushResult(
            pinned_ref=f"{ref}@{DIGEST}",
            cr_path=output / "pydantic-ai-foo.capability.yaml",
            cr_name="pydantic-ai-foo",
            signed=bool(kwargs.get("sign")),
            applied=None,
            index_file=None,
        )

    monkeypatch.setattr(import_mod.build_mod.build, "callback", fake_build)
    monkeypatch.setattr(import_mod.push_mod, "run_push", fake_run_push)

    def invoke(*extra_args: str, args_input: str | None = None, config_schema: Any = DERIVED_SCHEMA):
        state["config_schema"] = config_schema
        args = ["pydantic-ai-foo==1.2.0", "--tag", TAG, "--output", str(output), *extra_args]
        result = CliRunner().invoke(import_command, args, input=args_input)
        return result, build_calls, push_calls

    return invoke


class TestNonInteractive:
    def test_yes_skips_review_and_pushes(self, harness) -> None:
        result, _build_calls, push_calls = harness("--yes")
        assert result.exit_code == 0, result.output
        assert "Publish this schema" not in result.output
        assert len(push_calls) == 1
        assert push_calls[0]["ref"] == TAG
        assert "Imported pydantic-ai-foo==1.2.0 as a Capability" in result.output

    def test_build_receives_from_pypi_and_flags(self, harness, tmp_path: Path) -> None:
        schema_file = tmp_path / "schema.json"
        schema_file.write_text("{}")
        result, build_calls, _ = harness(
            "--yes", "--entrypoint", "pydantic_ai_foo:FooCapability", "--schema", str(schema_file), "--name", "foo-cap"
        )
        assert result.exit_code == 0, result.output
        kwargs = build_calls[-1]
        assert kwargs["path"] is None
        assert kwargs["from_pypi"] == "pydantic-ai-foo==1.2.0"
        assert kwargs["tag"] == TAG
        assert kwargs["entrypoint"] == "pydantic_ai_foo:FooCapability"
        assert kwargs["schema_file"] == schema_file
        assert kwargs["cr_name_opt"] == "foo-cap"
        assert kwargs["skip_smoke_test"] is False

    def test_push_options_forwarded(self, harness, tmp_path: Path) -> None:
        key = tmp_path / "cosign.key"
        key.write_text("key")
        index_path = tmp_path / "index.json"
        result, _, push_calls = harness(
            "--yes",
            "--sign",
            "--cosign-key",
            str(key),
            "--apply",
            "--namespace",
            "agents",
            "--index",
            str(index_path),
        )
        assert result.exit_code == 0, result.output
        call = push_calls[-1]
        assert call["sign"] is True
        assert call["cosign_key"] == key
        assert call["apply"] is True
        assert call["namespace"] == "agents"
        assert call["index_path"] == index_path


class TestInteractiveReview:
    def test_confirm_shows_schema_then_pushes(self, harness) -> None:
        result, _, push_calls = harness(args_input="y\n")
        assert result.exit_code == 0, result.output
        assert "Derived config schema for pydantic-ai-foo==1.2.0" in result.output
        assert "pydantic_ai_foo:FooCapability" in result.output
        assert '"workspace"' in result.output  # pretty-printed schema body
        assert "Publish this schema as the capability's strict config contract?" in result.output
        assert len(push_calls) == 1

    def test_enter_accepts_the_derived_schema(self, harness) -> None:
        result, _, push_calls = harness(args_input="\n")
        assert result.exit_code == 0, result.output
        assert len(push_calls) == 1

    def test_abort_gives_guidance_and_does_not_push(self, harness) -> None:
        result, _, push_calls = harness(args_input="n\n")
        assert result.exit_code != 0
        assert "import aborted at schema review" in result.output
        assert "--schema" in result.output
        assert "--permissive" in result.output
        assert "--yes" in result.output
        assert push_calls == []

    def test_permissive_prompt_defaults_to_abort(self, harness) -> None:
        result, _, push_calls = harness("--permissive", args_input="\n", config_schema=None)
        assert result.exit_code != 0
        assert "schemaPolicy: permissive" in result.output
        assert "not be validated at admission" in result.output
        assert push_calls == []

    def test_permissive_prompt_explicit_yes_pushes(self, harness) -> None:
        result, _, push_calls = harness("--permissive", args_input="y\n", config_schema=None)
        assert result.exit_code == 0, result.output
        assert len(push_calls) == 1


class TestUsageGuards:
    def test_tag_is_required(self, harness) -> None:
        result = CliRunner().invoke(import_command, ["pydantic-ai-foo"])
        assert result.exit_code != 0
        assert "--tag" in result.output

    def test_cosign_key_needs_sign(self, harness, tmp_path: Path) -> None:
        key = tmp_path / "cosign.key"
        key.write_text("key")
        result, _, push_calls = harness("--yes", "--cosign-key", str(key))
        assert result.exit_code != 0
        assert "--cosign-key needs --sign" in result.output
        assert push_calls == []

    def test_namespace_needs_apply(self, harness) -> None:
        result, _, push_calls = harness("--yes", "--namespace", "agents")
        assert result.exit_code != 0
        assert "--namespace needs --apply" in result.output
        assert push_calls == []
