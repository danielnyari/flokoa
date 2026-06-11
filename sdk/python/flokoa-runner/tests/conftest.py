import json
from pathlib import Path

import pytest
from flokoa_runner import CONTRACT_VERSION, RUNNER_VERSION

REPO_ROOT = Path(__file__).resolve().parents[4]
SPEC_TESTDATA = REPO_ROOT / "operator" / "internal" / "spec" / "testdata"


@pytest.fixture
def runner_manifest_file(tmp_path: Path) -> Path:
    manifest = {
        "contractVersion": CONTRACT_VERSION,
        "runnerVersion": RUNNER_VERSION,
        "python": "3.13",
        "pydantic-ai": "1.107.0",
        "baseline": {},
        "platformCapabilities": {},
        "agentSpecSchemaDigest": "sha256:abc",
    }
    path = tmp_path / "runner-manifest.json"
    path.write_text(json.dumps(manifest))
    return path


@pytest.fixture
def etc_flokoa(tmp_path: Path, runner_manifest_file: Path, monkeypatch: pytest.MonkeyPatch) -> Path:
    """A fake /etc/flokoa layout wired up via the contract's env overrides."""
    monkeypatch.setenv("FLOKOA_RUNNER_MANIFEST_PATH", str(runner_manifest_file))
    spec_path = tmp_path / "agent-spec.yaml"
    card_path = tmp_path / "agent-card.json"
    monkeypatch.setenv("FLOKOA_AGENT_SPEC_PATH", str(spec_path))
    monkeypatch.setenv("FLOKOA_AGENT_CARD_PATH", str(card_path))
    monkeypatch.delenv("FLOKOA_EXPECTED_RUNNER_VERSION", raising=False)
    monkeypatch.delenv("FLOKOA_EXPECTED_SCHEMA_DIGEST", raising=False)
    return tmp_path
