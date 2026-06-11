"""Stage: load_manifest — the runner image's identity (runtime contract §1)."""

from __future__ import annotations

import json
import os
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from flokoa_runner.errors import BootstrapError

DEFAULT_MANIFEST_PATH = "/etc/flokoa/runner-manifest.json"

EXPECTED_RUNNER_VERSION_ENV = "FLOKOA_EXPECTED_RUNNER_VERSION"
EXPECTED_SCHEMA_DIGEST_ENV = "FLOKOA_EXPECTED_SCHEMA_DIGEST"


@dataclass(frozen=True)
class RunnerManifest:
    contract_version: int
    runner_version: str
    python: str
    pydantic_ai: str
    baseline: dict[str, str] = field(default_factory=dict)
    platform_capabilities: dict[str, str] = field(default_factory=dict)
    agent_spec_schema_digest: str = ""

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> RunnerManifest:
        return cls(
            contract_version=data["contractVersion"],
            runner_version=data["runnerVersion"],
            python=data["python"],
            pydantic_ai=data["pydantic-ai"],
            baseline=data.get("baseline", {}),
            platform_capabilities=data.get("platformCapabilities", {}),
            agent_spec_schema_digest=data.get("agentSpecSchemaDigest", ""),
        )


def load_manifest(path: str | Path | None = None) -> RunnerManifest:
    manifest_path = Path(path or os.environ.get("FLOKOA_RUNNER_MANIFEST_PATH", DEFAULT_MANIFEST_PATH))
    try:
        data = json.loads(manifest_path.read_text(encoding="utf-8"))
        manifest = RunnerManifest.from_dict(data)
    except FileNotFoundError:
        raise BootstrapError("load_manifest", "runner manifest not found", path=str(manifest_path)) from None
    except (json.JSONDecodeError, KeyError) as exc:
        raise BootstrapError("load_manifest", f"invalid runner manifest: {exc}", path=str(manifest_path)) from exc

    verify_expectations(manifest)
    return manifest


def verify_expectations(manifest: RunnerManifest) -> None:
    """Cross-check the operator's delivered expectations against the image.

    Version skew between operator and runner image is a loud bootstrap
    failure, never a runtime surprise (runtime contract §1).
    """
    expected_version = os.environ.get(EXPECTED_RUNNER_VERSION_ENV)
    if expected_version and expected_version != manifest.runner_version:
        raise BootstrapError(
            "load_manifest",
            "runner version skew: the operator compiled for a different runner release",
            expected=expected_version,
            actual=manifest.runner_version,
        )

    expected_digest = os.environ.get(EXPECTED_SCHEMA_DIGEST_ENV)
    if expected_digest and expected_digest != manifest.agent_spec_schema_digest:
        raise BootstrapError(
            "load_manifest",
            "AgentSpec schema digest skew: operator and image disagree on the spec schema",
            expected=expected_digest,
            actual=manifest.agent_spec_schema_digest,
        )
