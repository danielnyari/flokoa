# 02 — Phase 0: Cleanup & Release Baseline

**Phase:** 0 · **Size:** M · **Depends on:** — · **Enables:** everything (clean foundation, honest public surface)

## Goal

Execute the pivot's deletions, reconcile stale docs, and close the residual release-engineering gaps — so P0a starts from a repo that contains only what the brief says flokoa is.

## Current state (verified against `main@ac123b5`)

- **Already done** (do not redo): `release.yml` exists (tag-triggered, version-from-tag, test gate, 5-image buildx matrix), `CHANGELOG.md` exists, versions aligned at 0.1.0, AgentWorkflow `SubmitRun` already removed (template-only), AgentTrigger already on Argo Events (`agenttrigger_types.go` EventSource/EventBus refs; `trigger_handler.go` invoke endpoint; `trigger_limiter.go`; `push_gateway.go`).
- **To delete** (still present):
  - google-adk: `sdk/python/flokoa/src/flokoa/integrations/google_adk/`, its tests, the `google-adk` extra in `flokoa/pyproject.toml`.
  - Integrations registry: `flokoa/integrations/__init__.py` (`IntegrationType` dispatch, `_EXTRA_NAMES`, `_try_load`, `get_executor_cls`) — N=1 framework makes it ceremony. `IntegrationType` lives in `flokoa-types` and is referenced by config models (`LlmAgentConfig.framework`) — handle the type carefully (keep a deprecated single-value enum or drop the field; decide during implementation, prefer drop + regenerate).
  - `sdk/python/flokoa-managed-task/` entirely: package, its Dockerfile, **its entry in the `release.yml` build matrix**, its image refs in the AgentWorkflow compiler (AgentTask default image), `taskconfig` generated types, samples referencing `agentTask`.
  - `flokoa run --framework` flag (single framework; keep `flokoa run -m module:agent`).
- **AgentTask consequence**: with managed-task deleted, the `AgentTask` task type in `AgentWorkflowSpec` has no runtime. AgentWorkflow is frozen — so **remove the `AgentTask` task type from docs/samples and mark it unsupported in the CRD description** (leave the field for API compatibility, validation-reject new uses via the existing webhook) rather than ripping the type out of a frozen API.
- **Stale docs**: `docs/agenttrigger-rfc.md` describes the Knative-era design; CLAUDE.md files describe google-adk, managed-task, old versions; root `README.md` is 8 bytes; `RELEASE_REVIEW.md` items partially closed without being marked.
- **Helm/webhook gap persists**: chart has all 7 CRDs in `files/crds/` but **no webhook templates** (validators exist in `api/v1alpha1/*_webhook.go` but are unreachable in chart installs) and no published chart (no OCI push in `release.yml`).

## Implementation plan

1. **Deletions** (one PR, mechanical): google-adk + registry + managed-task + release-matrix entry + `--framework` flag; `PydanticAIAgentExecutor` becomes the only executor and `flokoa-managed-agent` imports it directly. Regenerate types (`make generate-python-models`); fix all imports/tests; `uv sync --all-packages` + full test suite green.
2. **AgentWorkflow freeze hygiene**: webhook rejects new `agentTask` usage with a message pointing at the freeze; docs state the frozen status and the SwarmRun direction (brief §7).
3. **Docs reconciliation**: rewrite `docs/agenttrigger-rfc.md` → `docs/agenttrigger.md` describing the shipped Argo Events design (EventSource/EventBus/Sensor → server invoke endpoint → push delivery), with the RFC archived; update root `CLAUDE.md` + module CLAUDE.md files (remove google-adk/managed-task/framework-table rows, point versions at the release process); root `README.md` gets the §1 positioning one-liner + quickstart links.
4. **Release gaps**: add `helm package` + `helm push oci://ghcr.io/danielnyari/charts` to `release.yml`; add webhook templates to the chart (Service, ValidatingWebhookConfiguration generated from `operator/config/webhook/manifests.yaml`, cert-manager Certificate, `webhooks.enabled` default true) — this is the surviving core of old spec 01 and is a **hard prerequisite for P0b**, whose Capability validation lives in admission.
5. **Tag `v0.2.0`** (first post-pivot release) once green.

## Testing

- Full Go + Python suites green after deletions; `helm template` snapshot incl. webhook manifests; Kind e2e: chart install with webhooks → invalid AgentTool rejected; release dry-run produces 4 images (managed-task gone) + OCI chart.

## Acceptance criteria

- `grep -ri "google.adk\|managed.task\|marvin" --include="*.py" --include="*.go" --include="*.toml"` returns only CHANGELOG/archive hits.
- Fresh `helm install` from OCI gives a working operator **with admission validation active**.
- Docs describe only what exists; README states the v2.1 positioning.

## Out of scope

- Any P0a work (no CRD shape changes beyond the AgentTask freeze guard). OpenAPI-tools retirement (happens in 04/05 when capabilities replace them — they still serve the current managed agent until then).
