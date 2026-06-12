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


def test_parse_lock_versions_filters_platform_markers_and_normalizes():
    """The embedded operator baseline must reflect what the linux runner installs.

    Platform-gated packages (win32) are dropped so they cannot seed
    false-positive capability dependency conflicts; names are PEP 503
    normalized to match the Go conflict detector.
    """
    import sys

    sys.path.insert(0, str(PKG_ROOT / "hack"))
    from gen_runner_manifest import normalize_name, parse_lock_versions

    lock = "\n".join(
        [
            "Flokoa_Common==1.0.0",
            "httpx==0.28.1",
            "pywin32==311 ; sys_platform == 'win32'",
            "colorama==0.4.6 ; sys_platform == 'win32'",
            "jeepney==0.9.0 ; sys_platform == 'linux'",
            "hf-xet==1.3.0 ; platform_machine == 'x86_64' or platform_machine == 'aarch64'",
        ]
    )
    versions = parse_lock_versions(lock)

    assert versions["flokoa-common"] == "1.0.0"  # normalized (_ -> -, lowercased)
    assert versions["httpx"] == "0.28.1"
    assert versions["jeepney"] == "0.9.0"  # linux marker holds
    assert versions["hf-xet"] == "1.3.0"  # holds for a supported arch
    assert "pywin32" not in versions  # win32-only, never installed on the runner
    assert "colorama" not in versions

    assert normalize_name("A__b--c..d") == "a-b-c-d"


def test_embedded_baseline_excludes_win32_packages():
    baseline_path = (
        REPO_ROOT / "operator" / "internal" / "spec" / "baselines" / f"runner-baseline-{RUNNER_VERSION}.json"
    )
    packages = json.loads(baseline_path.read_text())["packages"]
    assert "pywin32" not in packages
    assert packages["pydantic-ai"] == MANIFEST["pydantic-ai"]
