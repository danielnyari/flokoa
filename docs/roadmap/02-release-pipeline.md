# 02 — Release Pipeline, Version Alignment, Public READMEs

**Phase:** 0 · **Size:** M · **Depends on:** 01 · **Enables:** 09 (CLI installs), credible public launch

## Goal

One command (a git tag) produces a coherent release: all container images, the Helm chart (OCI), Python wheels, and release notes — all carrying the same version. Public-facing text stops being placeholder.

## Current state

- **Versions are inconsistent across surfaces** (RELEASE_REVIEW §7.4): `operator/Makefile` has `VERSION ?= 0.1.0`; chart `version: 0.1.0` / `appVersion: "0.1.0"`; all Python packages now `0.1.0` (`flokoa`, `flokoa-types`, `flokoa-managed-agent`, `flokoa-managed-task`, `flokoa-common`, workspace root); but root `CLAUDE.md` still documents 0.0.5/0.0.6/0.0.7 and RELEASE_REVIEW references 0.0.6/0.0.23 — several docs are stale relative to code. Reconcile docs to reality first.
- **No release machinery** (§7.5): no git tags, no GitHub Releases, no release workflow, no CHANGELOG. CI workflows: `test.yml` (Go tests + Docker build/push on main), `test-python.yml`, `test-e2e.yml`, `lint.yml`, `docs.yml`.
- **Images**: `make docker-build`/`docker-push` cover `flokoa-operator`, `flokoa-server`, `flokoa-a2a-plugin`, `flokoa-cli` at `ghcr.io/danielnyari/*:$(VERSION)`. **No target builds the managed-task image**, and `ghcr.io/danielnyari/flokoa/managed-task:latest` 403s (§7.3) — the `AgentTask` workflow task type is unusable in real clusters.
- **Default runtime image pinning**: `internal/infra/builder/deployment.go:16` now has `DefaultTemplateRuntimeImage = "ghcr.io/danielnyari/flokoa-cli:0.1.0"` — pinned (good, §7.7 partially addressed) but **hard-coded**, so every release requires a manual code edit. Verify whether a similar hard-coded default exists for the AgentTask runtime image in the workflow compiler.
- **Placeholder text** (§7.6): root `README.md` is 8 bytes; operator README is kubebuilder boilerplate; package descriptions are placeholders.

## Target design

- **Single version source**: top-level `VERSION` file. `operator/Makefile` reads it (`VERSION ?= $(shell cat ../VERSION)`); a `make set-version V=x.y.z` target rewrites Chart.yaml `version`/`appVersion` and all `pyproject.toml` versions (script in `hack/`). CI verifies alignment on every PR.
- **Build-time image defaults**: change `DefaultTemplateRuntimeImage` (and the AgentTask equivalent) from `const` to `var` overridden via `-ldflags "-X github.com/danielnyari/flokoa/operator/internal/infra/builder.DefaultTemplateRuntimeImage=ghcr.io/danielnyari/flokoa-cli:$(VERSION)"` in the Makefile `build` and Dockerfile build args. The committed default stays a valid pinned tag.
- **`release.yml` workflow**, triggered by `v*` tags:
  1. verify `VERSION` file == tag,
  2. run Go + Python test jobs (reuse existing workflows via `workflow_call`),
  3. buildx + push all **five** images (add `docker-build-managed-task` / `docker-push-managed-task` Makefile targets; decide the canonical image name — recommend `ghcr.io/danielnyari/flokoa-managed-task` to match sibling naming — and update the compiler's default reference),
  4. `helm package` + `helm push oci://ghcr.io/danielnyari/charts`,
  5. `uv build` wheels for `flokoa`, `flokoa-types` (+ publish to PyPI behind a `PYPI_TOKEN` secret guard; skip job cleanly when unset),
  6. create GitHub Release with generated notes (use GitHub's auto-generated notes; a curated `CHANGELOG.md` seeded from RELEASE_REVIEW history is maintained manually per release — avoid adopting release-please until release cadence justifies it).
- **READMEs**: root README with the harness positioning (one paragraph), feature bullets, quickstart link, architecture diagram link (reuse `docs/roadmap/00-target-architecture.md` mermaid), badges (CI, chart, PyPI). Real `description` fields in every `pyproject.toml`. Operator README: build/deploy basics + link to docs.

## Implementation plan

1. Create `VERSION` file; rewire `operator/Makefile`; write `hack/set-version.sh`; add a CI job (in `lint.yml`) asserting Makefile/chart/pyproject versions all equal `VERSION`.
2. Add managed-task image: Dockerfile (mirror the existing Python image pattern — Alpine, uv), Makefile targets, compiler default update + ldflags override; grep for the `403`-ing reference `ghcr.io/danielnyari/flokoa/managed-task` and fix the path format.
3. Convert image-default consts to ldflags-overridable vars; thread through `Dockerfile` (`ARG VERSION`) and Makefile.
4. Write `.github/workflows/release.yml` per the design; refactor `test.yml`/`test-python.yml` jobs to be `workflow_call`-able rather than duplicating steps.
5. Write READMEs and pyproject descriptions; update root `CLAUDE.md` version table to point at the `VERSION` file instead of hard-coded numbers.
6. Tag `v0.1.0`, run the release end-to-end, then `docker pull` + `helm pull oci://` as smoke verification.

## Testing

- CI version-alignment check fails when any surface drifts (test by intentionally bumping one in a draft PR).
- Release dry-run: `act` or a `workflow_dispatch` path that does everything except push (use `--dry-run` flags / skip-push input).
- After tagging: `helm install` from the OCI chart on Kind; create an Agent with `runtime.type: template` and an `AgentWorkflow` with an `agentTask` — both pods must pull their images successfully (this is the §7.3 regression test).

## Acceptance criteria

- `git tag v0.1.0 && git push --tags` produces: 5 images, OCI chart, GitHub Release, (optionally) PyPI wheels — all versioned `0.1.0`.
- No hard-coded image tags require editing during a release.
- Root README renders a real project page; RELEASE_REVIEW §7.3–§7.7 closed with evidence.

## Out of scope

- Signed images/SLSA provenance (note as follow-up: `cosign` step is a one-liner once the pipeline exists). Docs-site versioning. The `flokoa-cli` → managed-agent image naming confusion (P1 #8) — record a decision but don't rename registries in this unit.
