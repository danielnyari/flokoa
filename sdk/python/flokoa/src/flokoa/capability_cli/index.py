"""The capability index: v1 format, fetch/parse, search, append/replace.

Deliberately boring (§3.5): one JSON file in a git repo served raw, grepped
client-side. ``push --index`` edits a local checkout; a human commits — no
git automation. A hosted index is explicitly out of scope.
"""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.parse
import urllib.request
from datetime import UTC, datetime
from pathlib import Path
from typing import Annotated, Literal

from pydantic import BaseModel, ConfigDict, Field, ValidationError

from flokoa.capability_cli.errors import CapabilityCliError

#: Raw GitHub URL of this repo's published index file. The file ships with
#: registry seeding (roadmap 10) — until then fetches 404 and the CLI says so.
DEFAULT_INDEX_URL = "https://raw.githubusercontent.com/danielnyari/flokoa/main/capability-index/index.json"

INDEX_ENV_VAR = "FLOKOA_CAPABILITY_INDEX"

_FETCH_TIMEOUT_SECONDS = 10


class IndexEntry(BaseModel):
    """One published capability version (§3.5)."""

    model_config = ConfigDict(populate_by_name=True)

    name: str
    version: str
    artifact: str
    description: str = ""
    entrypoint: str | None = None
    requires: Annotated[dict[str, str], Field(default_factory=dict)]
    schema_policy: Annotated[Literal["strict", "permissive"], Field(alias="schemaPolicy")] = "strict"
    signed: bool = False
    keywords: Annotated[list[str], Field(default_factory=list)]
    homepage: str | None = None

    @property
    def key(self) -> tuple[str, str]:
        """Entries are appended/replaced keyed on (name, version)."""
        return (self.name, self.version)

    def matches(self, query: str) -> bool:
        """Case-insensitive substring match over name/description/keywords."""
        needle = query.lower()
        haystacks = [self.name, self.description, *self.keywords]
        return any(needle in value.lower() for value in haystacks)


class CapabilityIndex(BaseModel):
    """``index.json`` v1 — schemaVersion, updatedAt, and the entry list."""

    model_config = ConfigDict(populate_by_name=True)

    schema_version: Annotated[Literal[1], Field(alias="schemaVersion")] = 1
    updated_at: Annotated[str, Field(alias="updatedAt")]
    capabilities: Annotated[list[IndexEntry], Field(default_factory=list)]


def utc_now_stamp() -> str:
    """``updatedAt`` timestamps: second-resolution UTC, Z-suffixed."""
    return datetime.now(tz=UTC).strftime("%Y-%m-%dT%H:%M:%SZ")


def empty_index() -> CapabilityIndex:
    return CapabilityIndex(updated_at=utc_now_stamp(), capabilities=[])


def resolve_index_source(option: str | None) -> str:
    """``--index`` > ``FLOKOA_CAPABILITY_INDEX`` env > the published URL."""
    return option or os.environ.get(INDEX_ENV_VAR) or DEFAULT_INDEX_URL


def _parse_index(text: str, *, source: str) -> CapabilityIndex:
    try:
        payload = json.loads(text)
    except json.JSONDecodeError as exc:
        raise CapabilityCliError(f"capability index at {source} is not valid JSON: {exc}") from exc
    try:
        return CapabilityIndex.model_validate(payload)
    except ValidationError as exc:
        raise CapabilityCliError(f"capability index at {source} does not match the v1 format: {exc}") from exc


def _fetch_url(url: str) -> str:
    scheme = urllib.parse.urlparse(url).scheme
    if scheme not in ("http", "https"):  # pragma: no cover — guarded by load_index dispatch
        raise CapabilityCliError(f"unsupported index URL scheme {scheme!r}: {url}")
    try:
        # S310: scheme is allowlisted to http/https just above.
        with urllib.request.urlopen(url, timeout=_FETCH_TIMEOUT_SECONDS) as response:  # noqa: S310
            return response.read().decode("utf-8")
    except urllib.error.HTTPError as exc:
        if exc.code == 404:
            raise CapabilityCliError(
                f"capability index not found at {url} (HTTP 404) — the published index does not "
                f"exist yet; point --index (or {INDEX_ENV_VAR}) at an index URL or local file"
            ) from exc
        raise CapabilityCliError(f"capability index fetch from {url} failed: HTTP {exc.code}") from exc
    except urllib.error.URLError as exc:
        raise CapabilityCliError(f"capability index fetch from {url} failed: {exc.reason}") from exc


def is_url(source: str) -> bool:
    return source.startswith(("http://", "https://"))


def index_file_path(path: Path) -> Path:
    """``--index`` may point at the index file or a checkout containing it."""
    return path / "index.json" if path.is_dir() else path


def load_index(source: str) -> CapabilityIndex:
    """Load the index from a URL or a local path."""
    if is_url(source):
        return _parse_index(_fetch_url(source), source=source)
    path = index_file_path(Path(source))
    if not path.is_file():
        raise CapabilityCliError(f"capability index file {path} does not exist")
    return _parse_index(path.read_text(encoding="utf-8"), source=str(path))


def load_or_init_index(path: Path) -> CapabilityIndex:
    """For ``push --index``: a missing local file starts a fresh v1 index."""
    file = index_file_path(path)
    if not file.exists():
        return empty_index()
    return _parse_index(file.read_text(encoding="utf-8"), source=str(file))


def upsert_entry(index: CapabilityIndex, entry: IndexEntry) -> CapabilityIndex:
    """Replace the (name, version)-keyed entry in place, else append; refresh updatedAt."""
    for position, existing in enumerate(index.capabilities):
        if existing.key == entry.key:
            index.capabilities[position] = entry
            break
    else:
        index.capabilities.append(entry)
    index.updated_at = utc_now_stamp()
    return index


def search_entries(index: CapabilityIndex, query: str | None) -> list[IndexEntry]:
    """Filter entries by the query; no query returns everything."""
    if not query:
        return list(index.capabilities)
    return [entry for entry in index.capabilities if entry.matches(query)]


def write_index(index: CapabilityIndex, path: Path) -> Path:
    """Write the index JSON (2-space indent, trailing newline); returns the file."""
    file = index_file_path(path)
    file.parent.mkdir(parents=True, exist_ok=True)
    payload = index.model_dump(by_alias=True, exclude_none=True)
    file.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
    return file
