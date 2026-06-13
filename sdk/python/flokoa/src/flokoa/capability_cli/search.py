"""``flokoa capability search`` / ``list`` — discover capabilities.

Merges two sources into one table: the published v1 index (fetched and
grepped client-side) and in-cluster Capability CRs (kubectl shell-out,
skipped gracefully when kubectl or the cluster is absent). Permissive
entries are flagged visibly (§4.7): unvalidated per-agent config is a
property the operator picking a capability must see.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import click

from flokoa.capability_cli import index as index_mod
from flokoa.capability_cli import kubectl as kubectl_mod
from flokoa.capability_cli.errors import CapabilityCliError

_PERMISSIVE_MARK = "permissive (!)"

_PERMISSIVE_FOOTNOTE = (
    "(!) permissive: per-agent config is NOT validated at admission — typos and "
    "attacker-shaped config surface only inside the runner pod."
)

_COLUMNS = ("NAME", "VERSION", "RUNNER", "POLICY", "SIGNED", "SOURCE")


@dataclass
class Row:
    """One table row, whichever source it came from."""

    name: str
    version: str
    runner: str
    policy: str
    signed: bool
    source: str

    def cells(self) -> tuple[str, ...]:
        policy = _PERMISSIVE_MARK if self.policy == "permissive" else self.policy
        return (
            self.name,
            self.version,
            self.runner or "-",
            policy,
            "yes" if self.signed else "no",
            self.source,
        )


def _index_rows(entries: list[index_mod.IndexEntry]) -> list[Row]:
    return [
        Row(
            name=entry.name,
            version=entry.version,
            runner=entry.requires.get("flokoa-runner", ""),
            policy=entry.schema_policy,
            signed=entry.signed,
            source="index",
        )
        for entry in entries
    ]


def _cluster_rows(items: list[dict[str, Any]], query: str | None) -> list[Row]:
    rows: list[Row] = []
    for item in items:
        name = item.get("metadata", {}).get("name", "")
        if query and query.lower() not in name.lower():
            continue
        spec = item.get("spec", {})
        conditions = item.get("status", {}).get("conditions", []) or []
        signed = any(
            condition.get("type") == "Verified" and condition.get("status") == "True" for condition in conditions
        )
        rows.append(
            Row(
                name=name,
                version=spec.get("version", "-"),
                runner=spec.get("requires", {}).get("flokoaRunner", ""),
                policy=spec.get("schemaPolicy", "strict"),
                signed=signed,
                source="cluster",
            )
        )
    return rows


def _render_table(rows: list[Row]) -> str:
    table = [_COLUMNS, *(row.cells() for row in rows)]
    widths = [max(len(line[column]) for line in table) for column in range(len(_COLUMNS))]
    return "\n".join(
        "  ".join(cell.ljust(width) for cell, width in zip(line, widths, strict=True)).rstrip() for line in table
    )


def _run_search(query: str | None, index_source: str | None, cluster: bool) -> None:
    """The shared search/list body."""
    source = index_mod.resolve_index_source(index_source)
    rows: list[Row] = []
    try:
        published = index_mod.load_index(source)
    except CapabilityCliError as exc:
        click.secho(f"note: {exc.message}", fg="yellow", err=True)
    else:
        rows += _index_rows(index_mod.search_entries(published, query))

    if cluster:
        in_cluster = kubectl_mod.list_capabilities()
        if in_cluster.skipped_reason is not None:
            click.secho(f"note: in-cluster capabilities skipped ({in_cluster.skipped_reason})", fg="yellow", err=True)
        else:
            rows += _cluster_rows(in_cluster.items, query)

    if not rows:
        click.echo("no capabilities found" + (f" matching {query!r}" if query else ""))
        return

    rows.sort(key=lambda row: (row.name, row.version, row.source))
    click.echo(_render_table(rows))
    if any(row.policy == "permissive" for row in rows):
        click.secho(_PERMISSIVE_FOOTNOTE, fg="yellow")


_INDEX_OPTION_HELP = f"Index URL or local path (default: {index_mod.DEFAULT_INDEX_URL}, env {index_mod.INDEX_ENV_VAR})."


@click.command()
@click.argument("query", required=False)
@click.option("--index", "index_source", default=None, help=_INDEX_OPTION_HELP)
@click.option(
    "--cluster/--no-cluster",
    default=True,
    show_default=True,
    help="Also list in-cluster Capability CRs (skipped gracefully without kubectl/cluster).",
)
def search(query: str | None, index_source: str | None, cluster: bool) -> None:
    """Search the capability index (and the cluster) for QUERY.

    QUERY is a case-insensitive substring matched against name, description,
    and keywords; omit it to list everything.
    """
    _run_search(query, index_source, cluster)


@click.command("list")
@click.option("--index", "index_source", default=None, help=_INDEX_OPTION_HELP)
@click.option(
    "--cluster/--no-cluster",
    default=True,
    show_default=True,
    help="Also list in-cluster Capability CRs (skipped gracefully without kubectl/cluster).",
)
def list_command(index_source: str | None, cluster: bool) -> None:
    """List every known capability (same as `search` with no QUERY)."""
    _run_search(None, index_source, cluster)
