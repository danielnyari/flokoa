"""``flokoa capability build`` — wheelhouse + artifact + Capability CR.

The pipeline runs inside one disposable container of the **capability base
image** by default — the pinned runner baseline plus the build front-end, so
the compatibility matrix is satisfied by construction and pip is already
seeded (no per-build ensurepip on the happy path):

  1. ``freeze_baseline``  — the installed runner venv is the baseline.
  2. ``build_wheelhouse`` — baseline-constrained closure, wheels only.
  3. ``smoke_test``       — runner-identical install + entrypoint import.
  4. ``derive_schema``    — entrypoint resolution + config JSON Schema.

The host then hashes the wheels, writes the doubly-validated
``manifest.json``, builds the busybox artifact image as an OCI-layout tar,
and generates the Capability CR with a digest placeholder for
``flokoa capability push`` to rewrite.
"""

from __future__ import annotations

import json
import re
import shutil
from pathlib import Path
from typing import Any

import click

from flokoa.capability_cli import artifact as artifact_mod
from flokoa.capability_cli import container as container_mod
from flokoa.capability_cli import cr as cr_mod
from flokoa.capability_cli import git_auth as git_auth_mod
from flokoa.capability_cli.errors import CapabilityCliError

_ENTRYPOINT_PATTERN = re.compile(r"^[\w.]+:[A-Za-z_]\w*$")

# --from-pypi accepts exactly a PEP-508 package name with an optional
# ==version pin. Anything else — VCS/URL requirements (git+https://…),
# extras, environment markers, or smuggled pip options
# ("pkg --extra-index-url …") — is rejected before the value reaches the
# in-runner pip invocation.
_FROM_PYPI_PATTERN = re.compile(r"^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?(==[A-Za-z0-9._+!-]+)?$")

_INRUNNER_MOUNT = "/flokoa-inrunner"
_WORK_MOUNT = "/work"
_SRC_MOUNT = "/src"
# The git clone lands inside the (read-write) work mount, so the wheelhouse
# build reads it from there — no extra bind mount needed for the git tier.
_CLONE_DIR = "clone"

#: The ephemeral env var name carrying the git token into the clone step. The
#: host sets it on the `exec` subprocess env only (never argv); clone_repo.py
#: consumes it via a GIT_ASKPASS helper. See git_auth + ContainerSession.exec.
_GIT_TOKEN_ENV = "GIT_ASKPASS_TOKEN"

_PYPI_DANGER_BANNER = """\
EXTREMELY DANGEROUS: building from an unvetted PyPI package.
  --from-pypi pulls code published by arbitrary maintainers with no
  first-party vetting — a supply-chain risk. The artifact is digest-pinned
  once built (integrity), but that says NOTHING about whether the source
  package is malicious. Prefer --from-git or building your own (image tier),
  and restrict the cluster with capabilities.policy.allowedSources.\
"""

_PYPI_NEEDS_ACK = (
    "--from-pypi builds from an unvetted PyPI package (arbitrary maintainers, no "
    "first-party vetting) — the EXTREMELY DANGEROUS tier. Re-run with --allow-pypi to "
    "acknowledge, and prefer --from-git or building your own (image tier)."
)

_PERMISSIVE_WARNING = """\
WARNING: building with schemaPolicy: permissive.
  Per-agent config for this capability is NOT validated at admission — typos
  and attacker-shaped config surface only inside the runner pod. Permissive
  capabilities are flagged in `kubectl get capabilities` and in search output.
  Prefer a typed schema: annotate the capability's config or pass --schema.\
"""

_SKIP_SMOKE_WARNING = (
    "WARNING: --skip-smoke-test skips the install/import gate — a capability "
    "that cannot import will only fail inside agent pods at bootstrap."
)


def _derive_requires(runner_manifest: dict[str, Any]) -> artifact_mod.ManifestRequires:
    """Derive the ``requires`` tuple from the build image's runner manifest.

    The capability is built against this exact environment, so the tuple is
    anchored at the pinned versions: same Python minor, pydantic-ai
    ``>=<built minor>,<<next major>`` (additive-within-major contract), and
    runner ``>=<built minor>``.
    """
    fields: dict[str, str] = {}
    python = runner_manifest.get("python")
    if python:
        fields["python"] = python
    pydantic_ai = runner_manifest.get("pydantic-ai")
    if pydantic_ai:
        major, minor = pydantic_ai.split(".")[:2]
        fields["pydantic-ai"] = f">={major}.{minor},<{int(major) + 1}"
    runner_version = runner_manifest.get("runnerVersion")
    if runner_version:
        major, minor = runner_version.split(".")[:2]
        fields["flokoa-runner"] = f">={major}.{minor}"
    return artifact_mod.ManifestRequires.model_validate(fields)


def _read_work_json(work_dir: Path, name: str) -> dict[str, Any]:
    path = work_dir / name
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (FileNotFoundError, json.JSONDecodeError) as exc:
        raise CapabilityCliError(f"build step did not produce a readable {name}: {exc}") from exc


def _inrunner_script(name: str) -> str:
    return f"{_INRUNNER_MOUNT}/{name}"


def _resolve_schema_outcome(
    schema_report: dict[str, Any],
    *,
    explicit_schema: dict[str, Any] | None,
    permissive: bool,
) -> dict[str, Any] | None:
    """Apply the derivation-outcome policy; returns the configSchema or None."""
    outcome = schema_report.get("outcome")
    if outcome in ("ambiguous", "no-candidates"):
        candidates = schema_report.get("candidates") or []
        listing = "".join(f"\n  --entrypoint {candidate}" for candidate in candidates)
        raise CapabilityCliError(f"{schema_report.get('reason')}{listing}")

    if explicit_schema is not None:
        return explicit_schema
    if outcome == "derived":
        return schema_report["schema"]
    # underivable
    if permissive:
        return None
    raise CapabilityCliError(
        f"config schema is underivable: {schema_report.get('reason')}\n"
        "Type the capability's config (pydantic model / dataclass / typed __init__), "
        "pass --schema file.json, or opt into --permissive (refused by default)."
    )


def _clone_git_repo(
    session: container_mod.ContainerSession,
    git_source: git_auth_mod.ParsedGitSource,
    git_auth: git_auth_mod.GitAuth | None,
    *,
    ssh_sock_mount: str,
) -> str:
    """Clone the repo inside the build container; return the wheelhouse --src path.

    The resolved token (https) is passed to the clone step ONLY as the
    ephemeral ``GIT_ASKPASS_TOKEN`` env on the ``exec`` (``secret_env``) — it
    never touches the argv, a mount file, or the image. For ssh, the forwarded
    agent socket is pointed at via ``SSH_AUTH_SOCK`` / ``GIT_SSH_COMMAND`` set
    as ordinary (non-secret) env. The returned path is the checkout (plus
    subdirectory), which the wheelhouse build treats exactly like a PATH build.
    """
    clone_argv = [
        "python",
        _inrunner_script("clone_repo.py"),
        "--work",
        _WORK_MOUNT,
        "--url",
        git_source.url,
        "--scheme",
        git_source.scheme,
    ]
    if git_source.ref:
        clone_argv += ["--ref", git_source.ref]
    if git_source.subdirectory:
        clone_argv += ["--subdirectory", git_source.subdirectory]

    secret_env: dict[str, str] = {}
    if git_auth is not None and git_auth.token:
        secret_env[_GIT_TOKEN_ENV] = git_auth.token
    if git_auth is not None and git_auth.ssh_auth_sock is not None:
        # The agent-socket path + strict-ssh command are NOT secrets, so they go
        # inline via `env` (the value carries no credential — the socket is a
        # mount, the key material never leaves the host agent).
        clone_argv = [
            "env",
            f"SSH_AUTH_SOCK={ssh_sock_mount}",
            "GIT_SSH_COMMAND=ssh -o StrictHostKeyChecking=yes -o BatchMode=yes",
            *clone_argv,
        ]

    no_credential = git_auth is None or (git_auth.token is None and git_auth.ssh_auth_sock is None)
    try:
        session.exec(clone_argv, step="git clone", secret_env=secret_env or None)
    except CapabilityCliError as exc:
        # A public-repo clone needs no credential; but if none resolved AND the
        # clone failed, name the precedence the user can fix (the clone error
        # itself is already credential-redacted by ContainerSession.exec).
        if no_credential:
            raise CapabilityCliError(f"{exc.message}\n\n{git_auth_mod.auth_failure_message(git_source)}") from exc
        raise

    src = f"{_WORK_MOUNT}/{_CLONE_DIR}"
    if git_source.subdirectory:
        src = f"{src}/{git_source.subdirectory}"
    return src


@click.command()
@click.argument("path", required=False, type=click.Path(exists=True, file_okay=False, path_type=Path))
@click.option(
    "--from-pypi",
    "from_pypi",
    default=None,
    help="Build from PyPI: PKG or PKG==VERSION (requires --allow-pypi; excludes PATH/--from-git).",
)
@click.option(
    "--allow-pypi",
    "allow_pypi",
    is_flag=True,
    help="Acknowledge the EXTREMELY DANGEROUS PyPI tier (required with --from-pypi).",
)
@click.option(
    "--from-git",
    "from_git",
    default=None,
    help="Build from git: git+https://… or git+ssh://… [@ref] [#subdirectory=…] (excludes PATH/--from-pypi).",
)
@click.option("--tag", default=None, help="Artifact image ref (default <name>:<version>); required for push.")
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
@click.option("--base-image", default=None, help="Full build image override (default: flokoa-capability-base:<ver>).")
@click.option("--base-version", default=None, help="Build base image version (composed with the base repository).")
@click.option(
    "--runner-version",
    default=None,
    help="(retained) build environment version; the default now resolves to the base image.",
)
@click.option(
    "--runner-image",
    default=None,
    help="(retained) full build image override (back-compat alias for --base-image).",
)
@click.option("--platforms", default=None, help="OCI platforms, e.g. linux/amd64,linux/arm64 (default: host arch).")
@click.option(
    "--output",
    "output_dir",
    type=click.Path(file_okay=False, path_type=Path),
    default=Path("dist"),
    show_default=True,
    help="Output directory for the artifact tar, manifest, and CR.",
)
@click.option("--skip-smoke-test", is_flag=True, help="Skip the install/import smoke test (discouraged).")
def build(
    path: Path | None,
    from_pypi: str | None,
    allow_pypi: bool,
    from_git: str | None,
    tag: str | None,
    entrypoint: str | None,
    schema_file: Path | None,
    permissive: bool,
    cr_name_opt: str | None,
    base_image: str | None,
    base_version: str | None,
    runner_version: str | None,
    runner_image: str | None,
    platforms: str | None,
    output_dir: Path,
    skip_smoke_test: bool,
) -> None:
    """Build a capability artifact from PATH, --from-pypi, or --from-git.

    \b
    Exactly one source is required:
      PATH          a local Python project (source: image)
      --from-pypi   a PyPI package (EXTREMELY DANGEROUS; needs --allow-pypi)
      --from-git    a git repo, uv-style git+https://… / git+ssh://… (source: git)

    \b
    Produces in --output:
      <name>-artifact.oci.tar   OCI-layout artifact image (busybox + wheelhouse)
      manifest.json             artifact self-description (v1)
      <name>.capability.yaml    Capability CR (digest placeholder; push rewrites)
      config-schema.json        the config schema (strict builds)
    """
    # Exactly one of PATH | --from-pypi | --from-git.
    sources = [path is not None, from_pypi is not None, from_git is not None]
    if sum(sources) != 1:
        raise click.UsageError("exactly one of PATH, --from-pypi, or --from-git is required")
    if from_pypi is not None and not _FROM_PYPI_PATTERN.match(from_pypi):
        raise click.UsageError(
            f"--from-pypi must be a PyPI package name or NAME==VERSION, got {from_pypi!r} "
            "(VCS/URL requirements and pip options are not accepted)"
        )
    if from_pypi is not None and not allow_pypi:
        raise click.UsageError(_PYPI_NEEDS_ACK)
    git_source: git_auth_mod.ParsedGitSource | None = None
    if from_git is not None:
        git_source = git_auth_mod.parse_from_git(from_git)
    if schema_file is not None and permissive:
        raise click.UsageError("--schema and --permissive are mutually exclusive (--schema makes the CR strict)")
    if entrypoint is not None and not _ENTRYPOINT_PATTERN.match(entrypoint):
        raise click.UsageError(f"--entrypoint must be module:attr, got {entrypoint!r}")

    explicit_schema: dict[str, Any] | None = None
    if schema_file is not None:
        try:
            explicit_schema = json.loads(schema_file.read_text(encoding="utf-8"))
        except json.JSONDecodeError as exc:
            raise CapabilityCliError(f"--schema {schema_file} is not valid JSON: {exc}") from exc
        if not isinstance(explicit_schema, dict):
            raise CapabilityCliError(f"--schema {schema_file} must contain a JSON Schema object")

    if skip_smoke_test:
        click.secho(_SKIP_SMOKE_WARNING, fg="yellow", err=True)
    if from_pypi is not None:
        # --allow-pypi was supplied (the gate above passed); warn loudly.
        click.secho(_PYPI_DANGER_BANNER, fg="red", bold=True, err=True)

    tool = container_mod.detect_container_tool()
    image = container_mod.resolve_build_image(
        base_image=base_image,
        base_version=base_version,
        runner_image=runner_image,
        runner_version=runner_version,
    )
    platforms = platforms or artifact_mod.host_platform()

    output_dir = output_dir.resolve()
    work_dir = output_dir / ".build"
    if work_dir.exists():
        shutil.rmtree(work_dir)
    work_dir.mkdir(parents=True)

    # Resolve git auth host-side (before touching the container) so a private
    # repo with no resolvable credential fails up front naming the precedence,
    # not deep in a clone. SSH forwards the agent socket; https resolves a token.
    git_auth: git_auth_mod.GitAuth | None = None
    if git_source is not None:
        git_auth = git_auth_mod.resolve_auth(git_source)

    inrunner_dir = Path(__file__).resolve().parent / "_inrunner"
    mounts = [
        container_mod.Mount(inrunner_dir, _INRUNNER_MOUNT, read_only=True),
        container_mod.Mount(work_dir, _WORK_MOUNT, read_only=False),
    ]
    if path is not None:
        mounts.append(container_mod.Mount(path.resolve(), _SRC_MOUNT, read_only=True))
    # SSH agent forwarding: mount the host agent socket (not keys) at a fixed
    # path and point GIT_SSH_COMMAND at it. The token path (https) needs no
    # mount — it rides the ephemeral exec env (below), never a file.
    ssh_sock_mount = "/flokoa-ssh-agent.sock"
    if git_auth is not None and git_auth.ssh_auth_sock is not None:
        mounts.append(container_mod.Mount(Path(git_auth.ssh_auth_sock), ssh_sock_mount, read_only=False))

    click.echo(f"Building inside {image} ({tool})...")
    session = container_mod.ContainerSession(tool=tool, image=image, mounts=mounts)
    with session:
        session.exec(
            ["python", _inrunner_script("freeze_baseline.py"), "--work", _WORK_MOUNT],
            step="baseline freeze",
        )

        wheelhouse_src = _SRC_MOUNT
        if git_source is not None:
            wheelhouse_src = _clone_git_repo(session, git_source, git_auth, ssh_sock_mount=ssh_sock_mount)

        wheelhouse_argv = ["python", _inrunner_script("build_wheelhouse.py"), "--work", _WORK_MOUNT]
        if path is not None or git_source is not None:
            wheelhouse_argv += ["--src", wheelhouse_src]
        else:
            wheelhouse_argv += ["--from-pypi", from_pypi or ""]
        session.exec(wheelhouse_argv, step="wheelhouse build")
        wheelhouse_report = _read_work_json(work_dir, "wheelhouse-report.json")
        dist_name: str = wheelhouse_report["name"]
        version: str = wheelhouse_report["version"]
        dependencies: list[str] = wheelhouse_report["dependencies"]
        dependency_argv = [arg for pin in dependencies for arg in ("--dependency", pin)]
        pin_argv = ["--name", dist_name, "--version", version, *dependency_argv]

        if not skip_smoke_test:
            smoke_argv = ["python", _inrunner_script("smoke_test.py"), "--work", _WORK_MOUNT, *pin_argv]
            if entrypoint is not None:
                smoke_argv += ["--entrypoint", entrypoint]
            session.exec(smoke_argv, step="smoke test")

        resolved_entrypoint = entrypoint
        serialization_name: str | None = None
        config_schema = explicit_schema
        needs_derivation = not (entrypoint is not None and (explicit_schema is not None or permissive))
        if needs_derivation:
            derive_argv = ["python", _inrunner_script("derive_schema.py"), "--work", _WORK_MOUNT, *pin_argv]
            if entrypoint is not None:
                derive_argv += ["--entrypoint", entrypoint]
            session.exec(derive_argv, step="schema derivation")
            schema_report = _read_work_json(work_dir, "schema-report.json")
            config_schema = _resolve_schema_outcome(
                schema_report, explicit_schema=explicit_schema, permissive=permissive
            )
            resolved_entrypoint = schema_report["entrypoint"]
            serialization_name = schema_report.get("serializationName")

            if entrypoint is None and not skip_smoke_test:
                # The heuristic just picked the entrypoint — close the smoke
                # gate on it (the install itself is idempotent).
                session.exec(
                    [
                        "python",
                        _inrunner_script("smoke_test.py"),
                        "--work",
                        _WORK_MOUNT,
                        *pin_argv,
                        "--entrypoint",
                        resolved_entrypoint,
                    ],
                    step="entrypoint smoke test",
                )

    if not skip_smoke_test:
        smoke_report = _read_work_json(work_dir, "smoke-report.json")
        if smoke_report.get("warning"):
            click.secho(f"WARNING: {smoke_report['warning']}", fg="yellow", err=True)
    if permissive:
        click.secho(_PERMISSIVE_WARNING, fg="yellow", bold=True, err=True)

    if resolved_entrypoint is None:  # pragma: no cover — guarded by needs_derivation logic
        raise CapabilityCliError("entrypoint could not be resolved")

    runner_manifest = _read_work_json(work_dir, "runner-manifest.json")
    requires = _derive_requires(runner_manifest)

    cr_name = cr_mod.validate_cr_name(cr_name_opt or cr_mod.normalize_dist_name(dist_name))
    tag = tag or f"{cr_name}:{version}"

    # Record where the code came from. PATH builds are the author-built-their-own
    # case (source: image, no provenance). git/pypi record source + provenance —
    # the clean URL + resolved commit for git, the requirement for pypi. The git
    # provenance commit is the durable record even if the branch/tag moves.
    source: artifact_mod.CapabilitySource = "image"
    provenance: artifact_mod.Provenance | None = None
    if git_source is not None:
        source = "git"
        clone_report = _read_work_json(work_dir, "clone-report.json")
        provenance = artifact_mod.Provenance(
            git=artifact_mod.GitProvenance(
                url=clone_report["url"],
                commit=clone_report["commit"],
                ref=clone_report.get("ref"),
                subdirectory=clone_report.get("subdirectory"),
            )
        )
    elif from_pypi is not None:
        source = "pypi"
        provenance = artifact_mod.Provenance(pypi=artifact_mod.PypiProvenance(requirement=from_pypi))

    wheelhouse_dir = work_dir / "wheelhouse"
    manifest = artifact_mod.build_manifest(
        name=dist_name,
        version=version,
        entrypoint=resolved_entrypoint,
        requires=requires,
        dependencies=dependencies,
        wheelhouse=wheelhouse_dir,
        serialization_name=serialization_name,
        config_schema=None if permissive else config_schema,
        source=source,
        provenance=provenance,
    )
    artifact_mod.write_manifest(manifest, wheelhouse_dir / "manifest.json")
    artifact_mod.write_dockerfile(work_dir, manifest)

    artifact_tar = output_dir / f"{cr_name}-artifact.oci.tar"
    click.echo(f"Building artifact image {tag} ({platforms})...")
    artifact_mod.build_oci_archive(tool=tool, context_dir=work_dir, tag=tag, platforms=platforms, dest=artifact_tar)

    artifact_mod.write_manifest(manifest, output_dir / "manifest.json")
    if manifest.config_schema is not None:
        (output_dir / "config-schema.json").write_text(
            json.dumps(manifest.config_schema, indent=2, sort_keys=True) + "\n", encoding="utf-8"
        )
    cr_path = output_dir / f"{cr_name}.capability.yaml"
    cr_path.write_text(cr_mod.render_capability_cr(cr_name, tag, manifest, permissive=permissive), encoding="utf-8")

    click.echo(f"Built {cr_name}=={version}: {len(manifest.wheels)} wheel(s) in the wheelhouse")
    click.echo(f"  {artifact_tar}")
    click.echo(f"  {output_dir / 'manifest.json'}")
    click.echo(f"  {cr_path}")
    click.echo(f"Next: flokoa capability push <REF> --from {output_dir}")
