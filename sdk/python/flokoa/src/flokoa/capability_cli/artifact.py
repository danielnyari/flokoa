"""Artifact assembly: manifest model, wheelhouse helpers, OCI image build.

The ``ArtifactManifest`` pydantic model mirrors the artifact-manifest v1 JSON
Schema (runtime contract §4; shipped alongside as
``schemas/artifact-manifest-v1.schema.json``) — manifests are validated
against **both** before anything is written, so the model can never drift
from the published schema silently.
"""

from __future__ import annotations

import hashlib
import json
import platform
import subprocess
from functools import cache
from importlib import resources
from pathlib import Path
from typing import Annotated, Any, Literal

import jsonschema
from pydantic import BaseModel, ConfigDict, Field, ValidationError

from flokoa.capability_cli.errors import CapabilityCliError

#: Mount path of the wheelhouse inside the artifact image (runtime contract §4).
WHEELHOUSE_PATH = "/wheelhouse"

_SCHEMA_RESOURCE = "artifact-manifest-v1.schema.json"

# Mirrors `_NON_WHEEL_SUFFIXES` in flokoa_runner.capabilities: non-wheel
# installables are banned from wheelhouses (sdists execute setup code).
_NON_WHEEL_SUFFIXES = (".tar.gz", ".zip")


class WheelEntry(BaseModel):
    """One wheel in the wheelhouse with its integrity hash."""

    model_config = ConfigDict(populate_by_name=True)

    file: Annotated[str, Field(pattern=r"\.whl$")]
    sha256: Annotated[str, Field(pattern=r"^[a-f0-9]{64}$")]


class ManifestRequires(BaseModel):
    """The compatibility tuple checked against the runner manifest."""

    model_config = ConfigDict(populate_by_name=True)

    python: Annotated[str | None, Field(pattern=r"^\d+\.\d+$")] = None
    pydantic_ai: Annotated[str | None, Field(alias="pydantic-ai")] = None
    flokoa_runner: Annotated[str | None, Field(alias="flokoa-runner")] = None


class ArtifactManifest(BaseModel):
    """``manifest.json`` v1 — the artifact's self-description (§3.1)."""

    model_config = ConfigDict(populate_by_name=True)

    name: str
    version: str
    contract_version: Annotated[Literal[1], Field(alias="contractVersion")] = 1
    entrypoint: Annotated[str, Field(pattern=r"^[\w.]+:[A-Za-z_]\w*$")]
    serialization_name: Annotated[str | None, Field(alias="serializationName", pattern=r"^[A-Za-z_]\w*$")] = None
    requires: ManifestRequires
    dependencies: Annotated[
        list[Annotated[str, Field(pattern=r"^[A-Za-z0-9][A-Za-z0-9._-]*==[A-Za-z0-9._+!-]+$")]],
        Field(default_factory=list),
    ]
    wheels: Annotated[list[WheelEntry], Field(min_length=1)]
    schema_digest: Annotated[str | None, Field(alias="schemaDigest", pattern=r"^sha256:[a-f0-9]{64}$")] = None
    config_schema: Annotated[dict[str, Any] | None, Field(alias="configSchema")] = None

    def to_json_dict(self) -> dict[str, Any]:
        return self.model_dump(by_alias=True, exclude_none=True)


@cache
def load_manifest_schema() -> dict[str, Any]:
    """Load the artifact-manifest v1 JSON Schema shipped as package data."""
    schema_file = resources.files("flokoa.capability_cli") / "schemas" / _SCHEMA_RESOURCE
    return json.loads(schema_file.read_text(encoding="utf-8"))


def validate_manifest_dict(manifest: dict[str, Any]) -> None:
    """Validate a manifest dict against the published v1 schema file."""
    validator = jsonschema.Draft202012Validator(load_manifest_schema())
    errors = sorted(validator.iter_errors(manifest), key=lambda e: list(e.absolute_path))
    if errors:
        details = "; ".join(f"{'/'.join(str(p) for p in e.absolute_path) or '<root>'}: {e.message}" for e in errors)
        raise CapabilityCliError(f"manifest.json does not satisfy the artifact-manifest v1 schema: {details}")


def sha256_file(path: Path) -> str:
    with path.open("rb") as fh:
        return hashlib.file_digest(fh, "sha256").hexdigest()


def canonical_schema_digest(schema: dict[str, Any]) -> str:
    """sha256 of the canonical (sorted-keys, no-whitespace) schema JSON.

    Matches the fixture ``build.sh``'s ``jq -cS`` canonicalization so both
    build paths produce identical ``schemaDigest`` values.
    """
    canonical = json.dumps(schema, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return "sha256:" + hashlib.sha256(canonical.encode("utf-8")).hexdigest()


def wheel_entries(wheelhouse: Path) -> list[WheelEntry]:
    """Hash every wheel in the wheelhouse; refuse anything that is not a wheel."""
    entries: list[WheelEntry] = []
    for path in sorted(wheelhouse.iterdir()):
        if path.name == "manifest.json":
            continue
        if path.name.endswith(_NON_WHEEL_SUFFIXES) or path.name == "setup.py" or not path.name.endswith(".whl"):
            raise CapabilityCliError(
                f"non-wheel file in wheelhouse: {path.name} — capability artifacts carry wheels only; "
                "system-level or sdist-only dependencies belong in a custom agent image"
            )
        entries.append(WheelEntry(file=path.name, sha256=sha256_file(path)))
    if not entries:
        raise CapabilityCliError(f"wheelhouse {wheelhouse} contains no wheels")
    return entries


def build_manifest(
    *,
    name: str,
    version: str,
    entrypoint: str,
    requires: ManifestRequires,
    dependencies: list[str],
    wheelhouse: Path,
    serialization_name: str | None = None,
    config_schema: dict[str, Any] | None = None,
) -> ArtifactManifest:
    """Assemble and doubly-validate (model + schema file) the manifest."""
    try:
        manifest = ArtifactManifest(
            name=name,
            version=version,
            entrypoint=entrypoint,
            serialization_name=serialization_name,
            requires=requires,
            dependencies=sorted(dependencies),
            wheels=wheel_entries(wheelhouse),
            schema_digest=canonical_schema_digest(config_schema) if config_schema is not None else None,
            config_schema=config_schema,
        )
    except ValidationError as exc:
        raise CapabilityCliError(f"assembled manifest is invalid: {exc}") from exc
    validate_manifest_dict(manifest.to_json_dict())
    return manifest


def write_manifest(manifest: ArtifactManifest, path: Path) -> None:
    """Write the manifest sorted-keys + 2-space-indented (matches ``jq -S``)."""
    path.write_text(json.dumps(manifest.to_json_dict(), indent=2, sort_keys=True) + "\n", encoding="utf-8")


# --- OCI artifact image -----------------------------------------------------

#: Generated build-context Dockerfile — kept consistent with the fixture
#: Dockerfile at operator/test/e2e/fixtures/capabilities/Dockerfile.
_DOCKERFILE_TEMPLATE = """\
# Capability artifact image (runtime contract §4). Generated by
# `flokoa capability build` — busybox supplies the static shell + cp the
# initContainer copy path needs; nothing else.
FROM busybox:stable-musl

LABEL ai.flokoa.capability-name="{name}" \\
      ai.flokoa.capability-version="{version}" \\
      ai.flokoa.contract-version="{contract_version}"

# World-readable: the initContainer copies as uid 65532 and the runner reads
# as uid 65532 (runtime contract §4).
COPY --chmod=0644 wheelhouse/ /wheelhouse/
"""


def write_dockerfile(context_dir: Path, manifest: ArtifactManifest) -> Path:
    dockerfile = context_dir / "Dockerfile"
    dockerfile.write_text(
        _DOCKERFILE_TEMPLATE.format(
            name=manifest.name,
            version=manifest.version,
            contract_version=manifest.contract_version,
        ),
        encoding="utf-8",
    )
    return dockerfile


def host_platform() -> str:
    """Default ``--platforms`` value: the host architecture."""
    machine = platform.machine().lower()
    arch = {"x86_64": "amd64", "amd64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(machine)
    if arch is None:
        raise CapabilityCliError(
            f"cannot map host architecture {machine!r} to an OCI platform — pass --platforms explicitly"
        )
    return f"linux/{arch}"


def oci_build_argv(
    *,
    tool: str,
    context_dir: Path,
    tag: str,
    platforms: str,
    dest: Path,
) -> list[list[str]]:
    """Argv arrays that build the artifact and emit an OCI-layout tarball.

    docker: one ``buildx build --output type=oci`` invocation (multi-platform
    capable). podman: build + ``podman save --format oci-archive``
    (single-platform only — multi-arch artifact builds need docker buildx).
    """
    if tool == "docker":
        return [
            [
                "docker",
                "buildx",
                "build",
                "--platform",
                platforms,
                "-f",
                str(context_dir / "Dockerfile"),
                "-t",
                tag,
                "--output",
                f"type=oci,dest={dest}",
                str(context_dir),
            ]
        ]
    if "," in platforms:
        raise CapabilityCliError(
            "multi-platform artifact builds require docker buildx — podman builds a single platform per image"
        )
    return [
        [
            tool,
            "build",
            "--platform",
            platforms,
            "-f",
            str(context_dir / "Dockerfile"),
            "-t",
            tag,
            str(context_dir),
        ],
        [tool, "save", "--format", "oci-archive", "-o", str(dest), tag],
    ]


def build_oci_archive(*, tool: str, context_dir: Path, tag: str, platforms: str, dest: Path) -> None:
    """Build the busybox artifact image and write it as an OCI-layout tar."""
    for argv in oci_build_argv(tool=tool, context_dir=context_dir, tag=tag, platforms=platforms, dest=dest):
        result = subprocess.run(argv, capture_output=True, text=True, check=False)  # noqa: S603
        if result.returncode != 0:
            output = (result.stdout + "\n" + result.stderr).strip()[-4000:]
            raise CapabilityCliError(f"artifact image build failed ({' '.join(argv[:3])}):\n{output}")
