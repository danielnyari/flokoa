"""The runner's committed contract artifacts must agree with the installed pin.

These tests are the in-process half of `make verify-runner-contract`: they run
against the resolved environment, so a pin bump without regenerated artifacts
fails here even before the git-diff CI gate.
"""

import hashlib
import json
from importlib.metadata import version
from pathlib import Path

from flokoa_runner import CONTRACT_VERSION, RUNNER_VERSION

PKG_ROOT = Path(__file__).resolve().parents[1]
REPO_ROOT = PKG_ROOT.parents[2]
SCHEMA_PATH = REPO_ROOT / "operator" / "internal" / "spec" / "schemas" / f"agentspec-{RUNNER_VERSION}.json"
MANIFEST = json.loads((PKG_ROOT / "runner-manifest.json").read_text())


def test_manifest_matches_installed_pin():
    assert MANIFEST["contractVersion"] == CONTRACT_VERSION
    assert MANIFEST["runnerVersion"] == RUNNER_VERSION
    assert MANIFEST["pydantic-ai"] == version("pydantic-ai")
    for name, pinned in MANIFEST["baseline"].items():
        assert pinned == version(name), f"baseline package {name} drifted from manifest"


def test_manifest_schema_digest_matches_embedded_schema():
    digest = "sha256:" + hashlib.sha256(SCHEMA_PATH.read_bytes()).hexdigest()
    assert MANIFEST["agentSpecSchemaDigest"] == digest


def test_embedded_schema_matches_generated_schema():
    import importlib.util

    spec = importlib.util.spec_from_file_location("gen_agentspec_schema", PKG_ROOT / "hack" / "gen_agentspec_schema.py")
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)

    generated = json.dumps(module.generate_schema(), indent=2, sort_keys=True) + "\n"
    assert SCHEMA_PATH.read_text() == generated, (
        "embedded AgentSpec schema drifted from the pinned pydantic-ai — run 'make runner-contract'"
    )


def test_no_harness_in_baseline():
    lock = (PKG_ROOT / "runner.lock").read_text().lower()
    assert "pydantic-ai-harness" not in lock


def test_reserved_platform_capability_names():
    caps = MANIFEST["platformCapabilities"]
    for name in (
        "flokoa.platform/telemetry",
        "flokoa.platform/session-persistence",
        "flokoa.platform/budget-guardrail",
    ):
        assert name in caps
