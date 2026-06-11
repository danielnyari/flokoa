import json
from pathlib import Path

import pytest
from flokoa_runner.errors import BootstrapError
from flokoa_runner.secrets import resolve_secrets, secret_env_name

SPEC_TESTDATA = Path(__file__).resolve().parents[4] / "operator" / "internal" / "spec" / "testdata"


def test_secret_env_name_golden_pairs_shared_with_go():
    """The normalization rule is specified once in the contract; the Go
    compiler and this runner assert the same golden pairs."""
    pairs = json.loads((SPEC_TESTDATA / "secret-env-pairs.json").read_text())
    assert pairs, "golden pairs file must not be empty"
    for name, want in pairs.items():
        assert secret_env_name(name) == want


def test_resolve_replaces_nested_placeholders():
    doc = {
        "capabilities": [
            {"MCP": {"headers": {"Authorization": "Bearer ${secret:kb-token}"}}},
        ],
        "instructions": ["no placeholder here"],
    }
    resolved = resolve_secrets(doc, env={"FLOKOA_SECRET_KB_TOKEN": "s3cret"})
    assert resolved["capabilities"][0]["MCP"]["headers"]["Authorization"] == "Bearer s3cret"
    assert resolved["instructions"] == ["no placeholder here"]


def test_resolve_collects_all_missing_at_once():
    doc = {
        "a": "${secret:first-missing}",
        "b": {"c": "${secret:second.missing}"},
        "d": "${secret:present}",
    }
    with pytest.raises(BootstrapError) as excinfo:
        resolve_secrets(doc, env={"FLOKOA_SECRET_PRESENT": "x"})
    err = excinfo.value
    assert err.stage == "resolve_secrets"
    assert err.details["missing"] == ["first-missing", "second.missing"]
    # The structured error must not leak any resolved values.
    assert "x" not in err.to_json()


def test_resolve_multiple_placeholders_in_one_string():
    doc = {"dsn": "postgres://${secret:user}:${secret:pass}@db/flokoa"}
    resolved = resolve_secrets(doc, env={"FLOKOA_SECRET_USER": "u", "FLOKOA_SECRET_PASS": "p"})
    assert resolved["dsn"] == "postgres://u:p@db/flokoa"
