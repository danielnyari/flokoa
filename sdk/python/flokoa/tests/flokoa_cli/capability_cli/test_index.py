"""index.py: v1 format parsing, fetch fallbacks, search, and upsert."""

from __future__ import annotations

import email.message
import io
import json
import urllib.error
from pathlib import Path
from unittest import mock

import pytest

from flokoa.capability_cli import index
from flokoa.capability_cli.errors import CapabilityCliError

SPEC_EXAMPLE = {
    "schemaVersion": 1,
    "updatedAt": "2026-06-12T00:00:00Z",
    "capabilities": [
        {
            "name": "flokoa-openapi",
            "description": "Front REST APIs described by OpenAPI as agent tools",
            "version": "0.1.0",
            "artifact": "ghcr.io/danielnyari/capabilities/flokoa-openapi@sha256:" + "a" * 64,
            "entrypoint": "flokoa_openapi:OpenAPICapability",
            "requires": {"python": "3.13", "pydantic-ai": ">=1.107,<2", "flokoa-runner": ">=0.2"},
            "schemaPolicy": "strict",
            "signed": True,
            "keywords": ["openapi", "rest", "tools"],
            "homepage": "https://github.com/danielnyari/flokoa",
        },
        {
            "name": "sketchy-cap",
            "description": "An untyped community capability",
            "version": "2.0.0",
            "artifact": "ghcr.io/example/sketchy@sha256:" + "b" * 64,
            "schemaPolicy": "permissive",
        },
    ],
}


def example_index() -> index.CapabilityIndex:
    return index.CapabilityIndex.model_validate(SPEC_EXAMPLE)


class TestModel:
    def test_spec_example_roundtrips(self) -> None:
        parsed = example_index()
        assert parsed.schema_version == 1
        assert parsed.capabilities[0].requires["flokoa-runner"] == ">=0.2"
        assert parsed.capabilities[0].signed is True
        assert parsed.capabilities[1].schema_policy == "permissive"
        assert parsed.capabilities[1].signed is False

    def test_unsupported_schema_version_refused(self) -> None:
        with pytest.raises(CapabilityCliError, match="v1 format"):
            index._parse_index(json.dumps({**SPEC_EXAMPLE, "schemaVersion": 2}), source="test")


class TestResolveSource:
    def test_option_wins(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv(index.INDEX_ENV_VAR, "https://env.example/index.json")
        assert index.resolve_index_source("./local/index.json") == "./local/index.json"

    def test_env_beats_default(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv(index.INDEX_ENV_VAR, "https://env.example/index.json")
        assert index.resolve_index_source(None) == "https://env.example/index.json"

    def test_default_is_published_raw_url(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv(index.INDEX_ENV_VAR, raising=False)
        assert index.resolve_index_source(None) == index.DEFAULT_INDEX_URL
        assert index.DEFAULT_INDEX_URL.startswith("https://raw.githubusercontent.com/danielnyari/flokoa/")


class TestLoadIndex:
    def test_local_file(self, tmp_path: Path) -> None:
        file = tmp_path / "index.json"
        file.write_text(json.dumps(SPEC_EXAMPLE))
        assert index.load_index(str(file)).capabilities[0].name == "flokoa-openapi"

    def test_local_directory_uses_index_json(self, tmp_path: Path) -> None:
        (tmp_path / "index.json").write_text(json.dumps(SPEC_EXAMPLE))
        assert len(index.load_index(str(tmp_path)).capabilities) == 2

    def test_missing_local_file(self, tmp_path: Path) -> None:
        with pytest.raises(CapabilityCliError, match="does not exist"):
            index.load_index(str(tmp_path / "index.json"))

    def test_invalid_json(self, tmp_path: Path) -> None:
        file = tmp_path / "index.json"
        file.write_text("{nope")
        with pytest.raises(CapabilityCliError, match="not valid JSON"):
            index.load_index(str(file))

    def test_url_fetch(self) -> None:
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = json.dumps(SPEC_EXAMPLE).encode()
        with mock.patch.object(index.urllib.request, "urlopen", return_value=response) as urlopen:
            parsed = index.load_index("https://example.com/index.json")
        assert parsed.capabilities[0].name == "flokoa-openapi"
        assert urlopen.call_args.args[0] == "https://example.com/index.json"

    def test_url_404_gets_helpful_message(self) -> None:
        not_found = urllib.error.HTTPError(
            "https://example.com/index.json", 404, "Not Found", email.message.Message(), io.BytesIO()
        )
        with (
            mock.patch.object(index.urllib.request, "urlopen", side_effect=not_found),
            pytest.raises(CapabilityCliError, match=r"does not\s+exist yet.*FLOKOA_CAPABILITY_INDEX"),
        ):
            index.load_index("https://example.com/index.json")

    def test_url_other_http_error(self) -> None:
        denied = urllib.error.HTTPError(
            "https://example.com/index.json", 503, "Unavailable", email.message.Message(), io.BytesIO()
        )
        with (
            mock.patch.object(index.urllib.request, "urlopen", side_effect=denied),
            pytest.raises(CapabilityCliError, match="HTTP 503"),
        ):
            index.load_index("https://example.com/index.json")

    def test_url_network_error(self) -> None:
        unreachable = urllib.error.URLError("name or service not known")
        with (
            mock.patch.object(index.urllib.request, "urlopen", side_effect=unreachable),
            pytest.raises(CapabilityCliError, match="name or service not known"),
        ):
            index.load_index("https://example.com/index.json")


class TestSearchEntries:
    @pytest.mark.parametrize("query", ["openapi", "OpenAPI", "REST APIs", "rest"])
    def test_case_insensitive_over_name_description_keywords(self, query: str) -> None:
        hits = index.search_entries(example_index(), query)
        assert [entry.name for entry in hits] == ["flokoa-openapi"]

    def test_no_query_returns_everything(self) -> None:
        assert len(index.search_entries(example_index(), None)) == 2
        assert len(index.search_entries(example_index(), "")) == 2

    def test_no_match(self) -> None:
        assert index.search_entries(example_index(), "does-not-exist") == []


class TestUpsert:
    def _entry(self, name: str = "new-cap", version: str = "1.0.0", **overrides: object) -> index.IndexEntry:
        fields: dict = {
            "name": name,
            "version": version,
            "artifact": f"ghcr.io/example/{name}@sha256:{'c' * 64}",
        }
        fields.update(overrides)
        return index.IndexEntry.model_validate(fields)

    def test_append_preserves_order(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setattr(index, "utc_now_stamp", lambda: "2026-06-12T12:00:00Z")
        parsed = example_index()
        index.upsert_entry(parsed, self._entry())
        assert [entry.name for entry in parsed.capabilities] == ["flokoa-openapi", "sketchy-cap", "new-cap"]
        assert parsed.updated_at == "2026-06-12T12:00:00Z"

    def test_replace_keyed_on_name_and_version(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setattr(index, "utc_now_stamp", lambda: "2026-06-12T12:00:00Z")
        parsed = example_index()
        replacement = self._entry(name="flokoa-openapi", version="0.1.0", signed=False)
        index.upsert_entry(parsed, replacement)
        # Replaced in place: same position, same count, new content.
        assert [entry.name for entry in parsed.capabilities] == ["flokoa-openapi", "sketchy-cap"]
        assert parsed.capabilities[0].signed is False
        assert parsed.capabilities[0].artifact == replacement.artifact

    def test_new_version_of_existing_name_appends(self) -> None:
        parsed = example_index()
        index.upsert_entry(parsed, self._entry(name="flokoa-openapi", version="0.2.0"))
        assert len(parsed.capabilities) == 3


class TestWriteAndInit:
    def test_write_roundtrip_drops_unset_optionals(self, tmp_path: Path) -> None:
        file = index.write_index(example_index(), tmp_path / "index.json")
        payload = json.loads(file.read_text())
        assert payload["schemaVersion"] == 1
        assert payload["capabilities"][0]["homepage"] == "https://github.com/danielnyari/flokoa"
        assert "homepage" not in payload["capabilities"][1]
        assert index.load_index(str(file)).capabilities[1].name == "sketchy-cap"

    def test_write_into_directory_checkout(self, tmp_path: Path) -> None:
        file = index.write_index(example_index(), tmp_path)
        assert file == tmp_path / "index.json"
        assert file.is_file()

    def test_load_or_init_missing_file_starts_fresh(self, tmp_path: Path) -> None:
        fresh = index.load_or_init_index(tmp_path / "index.json")
        assert fresh.schema_version == 1
        assert fresh.capabilities == []

    def test_load_or_init_existing_file(self, tmp_path: Path) -> None:
        file = tmp_path / "index.json"
        file.write_text(json.dumps(SPEC_EXAMPLE))
        assert len(index.load_or_init_index(file).capabilities) == 2
