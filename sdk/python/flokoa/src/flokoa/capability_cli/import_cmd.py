"""``flokoa capability import`` — PyPI package to attachable Capability CR.

Composes ``build --from-pypi`` -> interactive schema review -> ``push``, all
in-process (``ctx.invoke`` + ``run_push`` — never a subprocess of itself).
The review gate exists because the schema was *derived*, not authored: a
human confirms the config contract before it is published; ``--yes`` skips
the prompt for CI.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import click

from flokoa.capability_cli import build as build_mod
from flokoa.capability_cli import push as push_mod
from flokoa.capability_cli.errors import CapabilityCliError

_ABORT_GUIDANCE = (
    "import aborted at schema review — refine the contract with --schema file.json, "
    "pick another class with --entrypoint, or (last resort) opt into --permissive; "
    "use --yes to skip this prompt in CI"
)


def _load_built_manifest(output_dir: Path) -> dict[str, Any]:
    manifest_path = output_dir / "manifest.json"
    try:
        return json.loads(manifest_path.read_text(encoding="utf-8"))
    except (FileNotFoundError, json.JSONDecodeError) as exc:  # pragma: no cover — build just wrote it
        raise CapabilityCliError(f"build did not produce a readable {manifest_path}: {exc}") from exc


def _review_schema(manifest: dict[str, Any], package: str) -> None:
    """Show the derived config contract and require a human yes."""
    config_schema = manifest.get("configSchema")
    if config_schema is not None:
        click.echo(f"Derived config schema for {package} (entrypoint {manifest.get('entrypoint')}):")
        click.echo(json.dumps(config_schema, indent=2, sort_keys=True))
        confirmed = click.confirm("Publish this schema as the capability's strict config contract?", default=True)
    else:
        confirmed = click.confirm(
            f"{package} has NO config schema (schemaPolicy: permissive) — "
            "per-agent config will not be validated at admission. Publish anyway?",
            default=False,
        )
    if not confirmed:
        raise CapabilityCliError(_ABORT_GUIDANCE)


@click.command("import")
@click.argument("package")
@click.option("--tag", required=True, help="Artifact image ref to push, e.g. ghcr.io/org/capabilities/name:1.2.0.")
@click.option("--entrypoint", default=None, help="Capability class as module:attr (default: heuristic).")
@click.option(
    "--schema",
    "schema_file",
    type=click.Path(exists=True, dir_okay=False, path_type=Path),
    default=None,
    help="Use this config JSON Schema instead of deriving one.",
)
@click.option("--permissive", is_flag=True, help="Accept an underivable schema (loud warning; permissive CR).")
@click.option("--name", "cr_name_opt", default=None, help="Capability CR name (default: normalized dist name).")
@click.option("--runner-version", default=None, help="Runner release to build against (default: SDK-pinned).")
@click.option("--runner-image", default=None, help="Full runner image override.")
@click.option("--platforms", default=None, help="OCI platforms, e.g. linux/amd64,linux/arm64 (default: host arch).")
@click.option(
    "--output",
    "output_dir",
    type=click.Path(file_okay=False, path_type=Path),
    default=Path("dist"),
    show_default=True,
    help="Build output directory (what push reads).",
)
@click.option("--sign/--no-sign", default=False, show_default=True, help="cosign sign the pushed digest.")
@click.option(
    "--cosign-key",
    type=click.Path(exists=True, dir_okay=False, path_type=Path),
    default=None,
    help="Key-based signing; omitted with --sign means keyless (ambient OIDC).",
)
@click.option("--apply", "apply_", is_flag=True, help="kubectl apply the digest-pinned CR.")
@click.option("--namespace", default=None, help="Namespace for --apply.")
@click.option(
    "--index",
    "index_path",
    type=click.Path(path_type=Path),
    default=None,
    help="Local checkout of the index file/repo to append or update (you commit it).",
)
@click.option("--yes", is_flag=True, help="Non-interactive: accept the derived schema without prompting (CI).")
@click.pass_context
def import_command(
    ctx: click.Context,
    package: str,
    tag: str,
    entrypoint: str | None,
    schema_file: Path | None,
    permissive: bool,
    cr_name_opt: str | None,
    runner_version: str | None,
    runner_image: str | None,
    platforms: str | None,
    output_dir: Path,
    sign: bool,
    cosign_key: Path | None,
    apply_: bool,
    namespace: str | None,
    index_path: Path | None,
    yes: bool,
) -> None:
    """Import PACKAGE (PKG or PKG==VERSION) from PyPI as a capability.

    \b
    Equivalent to:
      flokoa capability build --from-pypi PACKAGE --tag REF ...
      (interactive review of the derived config schema)
      flokoa capability push REF ...
    """
    if cosign_key is not None and not sign:
        raise click.UsageError("--cosign-key needs --sign")
    if namespace is not None and not apply_:
        raise click.UsageError("--namespace needs --apply")

    ctx.invoke(
        build_mod.build,
        path=None,
        from_pypi=package,
        tag=tag,
        entrypoint=entrypoint,
        schema_file=schema_file,
        permissive=permissive,
        cr_name_opt=cr_name_opt,
        runner_version=runner_version,
        runner_image=runner_image,
        platforms=platforms,
        output_dir=output_dir,
        skip_smoke_test=False,
    )

    if not yes:
        _review_schema(_load_built_manifest(output_dir), package)

    push_mod.run_push(
        tag,
        from_dir=output_dir,
        sign=sign,
        cosign_key=cosign_key,
        apply=apply_,
        namespace=namespace,
        index_path=index_path,
    )
    click.echo(f"Imported {package} as a Capability ({tag})")
