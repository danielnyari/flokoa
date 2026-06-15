"""``flokoa capability search`` / ``list``: index+cluster merge and the table."""

from __future__ import annotations

from typing import Any
from unittest import mock

import pytest
from click.testing import CliRunner

from flokoa.capability_cli import index as index_mod
from flokoa.capability_cli import search as search_mod
from flokoa.capability_cli.errors import CapabilityCliError
from flokoa.capability_cli.kubectl import ClusterCapabilities
from flokoa.capability_cli.search import list_command, search

INDEX = index_mod.CapabilityIndex.model_validate({
    "schemaVersion": 1,
    "updatedAt": "2026-06-12T00:00:00Z",
    "capabilities": [
        {
            "name": "flokoa-openapi",
            "description": "Front REST APIs described by OpenAPI as agent tools",
            "version": "0.1.0",
            "artifact": "ghcr.io/danielnyari/capabilities/flokoa-openapi@sha256:" + "a" * 64,
            "requires": {"python": "3.13", "flokoa-runner": ">=0.2"},
            "schemaPolicy": "strict",
            "signed": True,
            "keywords": ["openapi", "rest", "tools"],
        },
        {
            "name": "sketchy-cap",
            "description": "An untyped community capability",
            "version": "2.0.0",
            "artifact": "ghcr.io/example/sketchy@sha256:" + "b" * 64,
            "schemaPolicy": "permissive",
        },
    ],
})

CLUSTER_ITEMS: list[dict[str, Any]] = [
    {
        "metadata": {"name": "flokoa-cap-echo", "namespace": "agents"},
        "spec": {
            "version": "0.3.0",
            "requires": {"python": "3.13", "flokoaRunner": ">=0.2"},
        },
        "status": {"conditions": [{"type": "Verified", "status": "True", "reason": "SignatureVerified"}]},
    },
    {
        "metadata": {"name": "loose-cap"},
        "spec": {"version": "1.0.0", "schemaPolicy": "permissive"},
        "status": {"conditions": [{"type": "Verified", "status": "Unknown", "reason": "VerificationDisabled"}]},
    },
]


@pytest.fixture
def sources(monkeypatch: pytest.MonkeyPatch) -> dict[str, mock.Mock]:
    monkeypatch.delenv(index_mod.INDEX_ENV_VAR, raising=False)
    mocks = {
        "load_index": mock.Mock(return_value=INDEX.model_copy(deep=True)),
        "list_capabilities": mock.Mock(return_value=ClusterCapabilities(items=list(CLUSTER_ITEMS))),
    }
    monkeypatch.setattr(search_mod.index_mod, "load_index", mocks["load_index"])
    monkeypatch.setattr(search_mod.kubectl_mod, "list_capabilities", mocks["list_capabilities"])
    return mocks


class TestSearchMerge:
    def test_table_merges_index_and_cluster(self, sources: dict[str, mock.Mock]) -> None:
        result = CliRunner().invoke(search, [])
        assert result.exit_code == 0, result.output
        lines = result.output.splitlines()
        assert lines[0].split() == ["NAME", "VERSION", "TIER", "RUNNER", "POLICY", "SIGNED", "SOURCE"]
        body = "\n".join(lines[1:])
        assert "flokoa-openapi" in body
        assert "flokoa-cap-echo" in body
        assert "index" in body
        assert "cluster" in body
        # Runner requirement comes through from both sources.
        echo_row = next(line for line in lines if line.startswith("flokoa-cap-echo"))
        assert ">=0.2" in echo_row
        openapi_row = next(line for line in lines if line.startswith("flokoa-openapi"))
        assert ">=0.2" in openapi_row
        assert "yes" in openapi_row  # signed in the index
        assert "yes" in echo_row  # Verified=True in the cluster

    def test_default_index_source_is_published_url(self, sources: dict[str, mock.Mock]) -> None:
        CliRunner().invoke(search, [])
        sources["load_index"].assert_called_once_with(index_mod.DEFAULT_INDEX_URL)

    def test_index_option_forwarded(self, sources: dict[str, mock.Mock]) -> None:
        CliRunner().invoke(search, ["--index", "/checkout/index.json"])
        sources["load_index"].assert_called_once_with("/checkout/index.json")

    def test_query_filters_both_sources(self, sources: dict[str, mock.Mock]) -> None:
        result = CliRunner().invoke(search, ["echo"])
        assert result.exit_code == 0, result.output
        assert "flokoa-cap-echo" in result.output
        assert "flokoa-openapi" not in result.output
        assert "sketchy-cap" not in result.output

    def test_query_matches_index_keywords(self, sources: dict[str, mock.Mock]) -> None:
        result = CliRunner().invoke(search, ["openapi"])
        assert result.exit_code == 0, result.output
        assert "flokoa-openapi" in result.output
        assert "flokoa-cap-echo" not in result.output

    def test_no_results_message(self, sources: dict[str, mock.Mock]) -> None:
        result = CliRunner().invoke(search, ["no-such-capability"])
        assert result.exit_code == 0, result.output
        assert "no capabilities found matching 'no-such-capability'" in result.output


class TestPermissiveFlagging:
    def test_permissive_rows_marked_and_footnoted(self, sources: dict[str, mock.Mock]) -> None:
        result = CliRunner().invoke(search, [])
        assert result.exit_code == 0, result.output
        sketchy_row = next(line for line in result.output.splitlines() if line.startswith("sketchy-cap"))
        assert "permissive (!)" in sketchy_row
        loose_row = next(line for line in result.output.splitlines() if line.startswith("loose-cap"))
        assert "permissive (!)" in loose_row
        assert "NOT validated at admission" in result.output

    def test_no_footnote_without_permissive_rows(self, sources: dict[str, mock.Mock]) -> None:
        result = CliRunner().invoke(search, ["openapi"])
        assert result.exit_code == 0, result.output
        assert "NOT validated at admission" not in result.output


class TestGracefulDegradation:
    def test_cluster_skip_reason_is_a_note_not_a_failure(self, sources: dict[str, mock.Mock]) -> None:
        sources["list_capabilities"].return_value = ClusterCapabilities(skipped_reason="kubectl is not on PATH")
        result = CliRunner().invoke(search, [])
        assert result.exit_code == 0, result.output
        assert "in-cluster capabilities skipped (kubectl is not on PATH)" in result.output
        assert "flokoa-openapi" in result.output

    def test_index_failure_is_a_note_cluster_still_shown(self, sources: dict[str, mock.Mock]) -> None:
        sources["load_index"].side_effect = CapabilityCliError("capability index not found at ... (HTTP 404)")
        result = CliRunner().invoke(search, [])
        assert result.exit_code == 0, result.output
        assert "note: capability index not found" in result.output
        assert "flokoa-cap-echo" in result.output

    def test_both_sources_empty(self, sources: dict[str, mock.Mock]) -> None:
        sources["load_index"].side_effect = CapabilityCliError("capability index not found (HTTP 404)")
        sources["list_capabilities"].return_value = ClusterCapabilities(skipped_reason="kubectl is not on PATH")
        result = CliRunner().invoke(search, [])
        assert result.exit_code == 0, result.output
        assert "no capabilities found" in result.output

    def test_no_cluster_skips_kubectl_entirely(self, sources: dict[str, mock.Mock]) -> None:
        result = CliRunner().invoke(search, ["--no-cluster"])
        assert result.exit_code == 0, result.output
        sources["list_capabilities"].assert_not_called()
        assert "cluster" not in result.output.splitlines()[0]


class TestListAlias:
    def test_list_is_search_without_query(self, sources: dict[str, mock.Mock]) -> None:
        listed = CliRunner().invoke(list_command, [])
        searched = CliRunner().invoke(search, [])
        assert listed.exit_code == 0, listed.output
        assert listed.output == searched.output


# Index entries and cluster items carrying the source tier (§5.5 / §6.5).
TIER_INDEX = index_mod.CapabilityIndex.model_validate({
    "schemaVersion": 1,
    "updatedAt": "2026-06-12T00:00:00Z",
    "capabilities": [
        {
            "name": "git-cap",
            "version": "1.0.0",
            "artifact": "ghcr.io/example/git-cap@sha256:" + "a" * 64,
            "source": "git",
        },
        {
            "name": "danger-cap",
            "version": "1.0.0",
            "artifact": "ghcr.io/example/danger-cap@sha256:" + "b" * 64,
            "source": "pypi",
        },
        {
            # No source field (older index) → TIER renders as "-".
            "name": "legacy-cap",
            "version": "1.0.0",
            "artifact": "ghcr.io/example/legacy-cap@sha256:" + "c" * 64,
        },
    ],
})

TIER_CLUSTER: list[dict[str, Any]] = [
    {
        "metadata": {"name": "builtin-cap"},
        "spec": {"version": "0.2.0", "source": "builtin"},
        "status": {"conditions": []},
    },
    {
        # A pre-source-field CR has no spec.source → defaults to image.
        "metadata": {"name": "old-image-cap"},
        "spec": {"version": "0.1.0"},
        "status": {"conditions": []},
    },
    {
        "metadata": {"name": "cluster-pypi-cap"},
        "spec": {"version": "9.9.9", "source": "pypi"},
        "status": {"conditions": []},
    },
]


@pytest.fixture
def tier_sources(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv(index_mod.INDEX_ENV_VAR, raising=False)
    monkeypatch.setattr(search_mod.index_mod, "load_index", mock.Mock(return_value=TIER_INDEX.model_copy(deep=True)))
    monkeypatch.setattr(
        search_mod.kubectl_mod,
        "list_capabilities",
        mock.Mock(return_value=ClusterCapabilities(items=list(TIER_CLUSTER))),
    )


class TestTierColumn:
    def test_tier_column_header_present(self, tier_sources: None) -> None:
        result = CliRunner().invoke(search, [])
        assert result.exit_code == 0, result.output
        header = result.output.splitlines()[0].split()
        assert header == ["NAME", "VERSION", "TIER", "RUNNER", "POLICY", "SIGNED", "SOURCE"]

    def test_tier_from_index_source(self, tier_sources: None) -> None:
        result = CliRunner().invoke(search, [])
        assert result.exit_code == 0, result.output
        git_row = next(line for line in result.output.splitlines() if line.startswith("git-cap"))
        assert "git" in git_row

    def test_tier_from_cluster_spec_source(self, tier_sources: None) -> None:
        result = CliRunner().invoke(search, [])
        assert result.exit_code == 0, result.output
        builtin_row = next(line for line in result.output.splitlines() if line.startswith("builtin-cap"))
        assert "builtin" in builtin_row

    def test_cluster_cr_without_source_defaults_to_image(self, tier_sources: None) -> None:
        result = CliRunner().invoke(search, [])
        assert result.exit_code == 0, result.output
        old_row = next(line for line in result.output.splitlines() if line.startswith("old-image-cap"))
        assert "image" in old_row

    def test_index_entry_without_source_renders_dash(self, tier_sources: None) -> None:
        result = CliRunner().invoke(search, [])
        assert result.exit_code == 0, result.output
        legacy_row = next(line for line in result.output.splitlines() if line.startswith("legacy-cap"))
        # TIER cell is "-" for an index entry with no source.
        assert legacy_row.split()[2] == "-"

    def test_pypi_rows_flagged_and_footnoted(self, tier_sources: None) -> None:
        result = CliRunner().invoke(search, [])
        assert result.exit_code == 0, result.output
        danger_row = next(line for line in result.output.splitlines() if line.startswith("danger-cap"))
        cluster_pypi_row = next(line for line in result.output.splitlines() if line.startswith("cluster-pypi-cap"))
        assert "pypi (!!)" in danger_row
        assert "pypi (!!)" in cluster_pypi_row
        assert "unvetted PyPI package" in result.output

    def test_no_pypi_footnote_without_pypi_rows(self, sources: dict[str, mock.Mock]) -> None:
        # The default `sources` fixture has no pypi-tier rows.
        result = CliRunner().invoke(search, [])
        assert result.exit_code == 0, result.output
        assert "unvetted PyPI package" not in result.output
