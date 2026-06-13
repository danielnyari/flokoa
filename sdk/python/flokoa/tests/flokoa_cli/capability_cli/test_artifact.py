"""artifact.py: manifest model + schema-file agreement, hashing, OCI argv."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any

import pytest
from pydantic import ValidationError

from flokoa.capability_cli import artifact
from flokoa.capability_cli.errors import CapabilityCliError


def manifest_kwargs(**overrides: Any) -> dict[str, Any]:
    base: dict[str, Any] = {
        "name": "flokoa-cap-echo",
        "version": "0.1.0",
        "entrypoint": "flokoa_cap_echo:EchoCapability",
        "requires": artifact.ManifestRequires.model_validate({
            "python": "3.13",
            "pydantic-ai": ">=1.107,<2",
            "flokoa-runner": ">=0.2",
        }),
        "wheels": [artifact.WheelEntry(file="flokoa_cap_echo-0.1.0-py3-none-any.whl", sha256="a" * 64)],
    }
    base.update(overrides)
    return base


class TestArtifactManifestModel:
    def test_minimal_manifest_round_trips_aliases(self) -> None:
        manifest = artifact.ArtifactManifest(**manifest_kwargs())
        dumped = manifest.to_json_dict()
        assert dumped["contractVersion"] == 1
        assert dumped["requires"] == {"python": "3.13", "pydantic-ai": ">=1.107,<2", "flokoa-runner": ">=0.2"}
        assert "serializationName" not in dumped
        assert "schemaDigest" not in dumped

    def test_invalid_entrypoint_refused(self) -> None:
        with pytest.raises(ValidationError):
            artifact.ArtifactManifest(**manifest_kwargs(entrypoint="not-an-entrypoint"))

    def test_invalid_dependency_pin_refused(self) -> None:
        with pytest.raises(ValidationError):
            artifact.ArtifactManifest(**manifest_kwargs(dependencies=["inflection>=0.5"]))

    def test_empty_wheels_refused(self) -> None:
        with pytest.raises(ValidationError):
            artifact.ArtifactManifest(**manifest_kwargs(wheels=[]))

    def test_contract_version_pinned_to_one(self) -> None:
        with pytest.raises(ValidationError):
            artifact.ArtifactManifest.model_validate({
                **artifact.ArtifactManifest(**manifest_kwargs()).to_json_dict(),
                "contractVersion": 2,
            })


class TestManifestSchemaFile:
    """The shipped JSON Schema file and the pydantic model must agree."""

    def test_schema_file_loads_and_is_draft_2020(self) -> None:
        schema = artifact.load_manifest_schema()
        assert schema["$schema"] == "https://json-schema.org/draft/2020-12/schema"
        assert schema["required"] == ["name", "version", "contractVersion", "entrypoint", "requires", "wheels"]

    def test_model_output_satisfies_schema_file(self) -> None:
        manifest = artifact.ArtifactManifest(
            **manifest_kwargs(
                serialization_name="EchoCapability",
                dependencies=["inflection==0.5.1"],
                config_schema={"type": "object", "properties": {"prefix": {"type": "string"}}},
                schema_digest="sha256:" + "b" * 64,
            )
        )
        artifact.validate_manifest_dict(manifest.to_json_dict())  # must not raise

    def test_fixture_artifact_json_plus_wheels_satisfies_schema(self) -> None:
        """The chunk-1 echo fixture's artifact.json is schema-compatible."""
        fixture = (
            Path(__file__).parents[6]
            / "operator"
            / "test"
            / "e2e"
            / "fixtures"
            / "capabilities"
            / "echo"
            / "artifact.json"
        )
        if not fixture.is_file():
            pytest.skip("operator fixture checkout not available")
        doc = json.loads(fixture.read_text(encoding="utf-8"))
        doc["wheels"] = [{"file": "flokoa_cap_echo-0.1.0-py3-none-any.whl", "sha256": "c" * 64}]
        artifact.validate_manifest_dict(doc)  # must not raise

    @pytest.mark.parametrize(
        ("mutation", "fragment"),
        [
            ({"contractVersion": 2}, "contractVersion"),
            ({"entrypoint": "no-colon"}, "entrypoint"),
            ({"wheels": []}, "wheels"),
            ({"dependencies": ["inflection>=0.5"]}, "dependencies"),
            ({"schemaDigest": "sha256:short"}, "schemaDigest"),
        ],
    )
    def test_schema_file_rejects_bad_manifests(self, mutation: dict[str, Any], fragment: str) -> None:
        doc = artifact.ArtifactManifest(**manifest_kwargs()).to_json_dict()
        doc.update(mutation)
        with pytest.raises(CapabilityCliError, match=fragment):
            artifact.validate_manifest_dict(doc)


class TestHashing:
    def test_sha256_file(self, tmp_path: Path) -> None:
        wheel = tmp_path / "x.whl"
        wheel.write_bytes(b"wheel-bytes")
        assert artifact.sha256_file(wheel) == hashlib.sha256(b"wheel-bytes").hexdigest()

    def test_canonical_schema_digest_is_key_order_independent(self) -> None:
        a = artifact.canonical_schema_digest({"type": "object", "properties": {"x": {"type": "string"}}})
        b = artifact.canonical_schema_digest({"properties": {"x": {"type": "string"}}, "type": "object"})
        assert a == b
        assert a.startswith("sha256:")

    def test_canonical_schema_digest_matches_jq_cs(self) -> None:
        """Same canonicalization as the fixture build.sh (jq -cS)."""
        schema = {"type": "object", "properties": {"prefix": {"default": "echo", "type": "string"}}}
        canonical = json.dumps(schema, sort_keys=True, separators=(",", ":"))
        assert artifact.canonical_schema_digest(schema) == "sha256:" + hashlib.sha256(canonical.encode()).hexdigest()


class TestWheelEntries:
    def test_hashes_all_wheels_sorted(self, tmp_path: Path) -> None:
        (tmp_path / "b-1.0-py3-none-any.whl").write_bytes(b"b")
        (tmp_path / "a-1.0-py3-none-any.whl").write_bytes(b"a")
        entries = artifact.wheel_entries(tmp_path)
        assert [e.file for e in entries] == ["a-1.0-py3-none-any.whl", "b-1.0-py3-none-any.whl"]

    def test_manifest_json_is_ignored(self, tmp_path: Path) -> None:
        (tmp_path / "a-1.0-py3-none-any.whl").write_bytes(b"a")
        (tmp_path / "manifest.json").write_text("{}")
        assert len(artifact.wheel_entries(tmp_path)) == 1

    @pytest.mark.parametrize("bad", ["pkg-1.0.tar.gz", "pkg.zip", "setup.py", "README.md"])
    def test_non_wheel_refused(self, tmp_path: Path, bad: str) -> None:
        (tmp_path / "a-1.0-py3-none-any.whl").write_bytes(b"a")
        (tmp_path / bad).write_bytes(b"x")
        with pytest.raises(CapabilityCliError, match="non-wheel file in wheelhouse"):
            artifact.wheel_entries(tmp_path)

    def test_empty_wheelhouse_refused(self, tmp_path: Path) -> None:
        with pytest.raises(CapabilityCliError, match="contains no wheels"):
            artifact.wheel_entries(tmp_path)


class TestDockerfile:
    def test_dockerfile_matches_fixture_convention(self, tmp_path: Path) -> None:
        manifest = artifact.ArtifactManifest(**manifest_kwargs())
        dockerfile = artifact.write_dockerfile(tmp_path, manifest)
        content = dockerfile.read_text()
        assert "FROM busybox:stable-musl" in content
        assert 'ai.flokoa.capability-name="flokoa-cap-echo"' in content
        assert 'ai.flokoa.capability-version="0.1.0"' in content
        assert 'ai.flokoa.contract-version="1"' in content
        assert "COPY --chmod=0644 wheelhouse/ /wheelhouse/" in content


class TestOciBuildArgv:
    def test_docker_buildx_oci_output(self, tmp_path: Path) -> None:
        commands = artifact.oci_build_argv(
            tool="docker",
            context_dir=tmp_path,
            tag="cap:0.1.0",
            platforms="linux/amd64,linux/arm64",
            dest=tmp_path / "cap-artifact.oci.tar",
        )
        assert len(commands) == 1
        argv = commands[0]
        assert argv[:3] == ["docker", "buildx", "build"]
        assert argv[3:5] == ["--platform", "linux/amd64,linux/arm64"]
        assert f"type=oci,dest={tmp_path / 'cap-artifact.oci.tar'}" in argv

    def test_podman_build_plus_save(self, tmp_path: Path) -> None:
        commands = artifact.oci_build_argv(
            tool="podman",
            context_dir=tmp_path,
            tag="cap:0.1.0",
            platforms="linux/arm64",
            dest=tmp_path / "out.tar",
        )
        assert [c[0] for c in commands] == ["podman", "podman"]
        assert commands[0][1] == "build"
        assert commands[1][1:4] == ["save", "--format", "oci-archive"]

    def test_podman_multi_platform_refused(self, tmp_path: Path) -> None:
        with pytest.raises(CapabilityCliError, match="multi-platform"):
            artifact.oci_build_argv(
                tool="podman",
                context_dir=tmp_path,
                tag="cap:0.1.0",
                platforms="linux/amd64,linux/arm64",
                dest=tmp_path / "out.tar",
            )


class TestHostPlatform:
    def test_maps_known_arches(self) -> None:
        assert artifact.host_platform() in ("linux/amd64", "linux/arm64")
