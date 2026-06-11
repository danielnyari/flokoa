# Flokoa — First Release (v0.1.0) Readiness Review

**Date:** 2026-06-10 · **Commit reviewed:** `3a1b01d` (tip of `main`) · **Scope:** entire repo, current feature set, PRs disregarded.

---

## Status addendum (2026-06-11, Phase 0 of the v2.1 pivot)

This review predates the [Pivot v2.1 roadmap](docs/roadmap/README.md) and the
v0.1.0 release; several items below have since been closed. Closed items —
do not re-do:

- **§7.2 Helm chart missing AgentWorkflow CRD** — CLOSED: all seven CRDs ship in `files/crds/`.
- **§7.3 / WP5 managed-task disposition** — CLOSED by deletion: `flokoa-managed-task` was removed entirely in the pivot (Phase 0); the `agentTask` task type is frozen and rejected by the webhook.
- **§7.4 Version alignment** — CLOSED: versions aligned at 0.1.0; `release.yml` derives versions from the tag.
- **§7.5 Release machinery** — CLOSED: tag-triggered `release.yml` (test gate, image matrix, Helm OCI push, install.yaml, GitHub Release, opt-in PyPI) plus `CHANGELOG.md`.
- **§7.6 root README** — CLOSED: root `README.md` carries the v2.1 positioning + quickstart. (operator README / pyproject metadata polish still open.)
- **§8.2 webhooks unreachable via chart** — CLOSED: the chart ships webhook Service, ValidatingWebhookConfiguration, cert-manager Certificate, and controller wiring (`webhooks.enabled`, default true).
- **§8.6 google-adk overmocked tests** — MOOT: google-adk support was deleted in the pivot.
- **§12 stale CLAUDE.md / agenttrigger docs** — CLOSED: CLAUDE.md files refreshed; `docs/agenttrigger-rfc.md` replaced by `docs/agenttrigger.md` (Argo Events design).

Still open (carry forward): §7.1 CI e2e secret wiring, §7.7 `:latest` template-runtime default, §8.1 Instruction service, §8.3 security posture doc, §8.4 workspace-root pytest, §8.5 zero-test packages, §8.9 docs site gaps, §9 polish items, WP6–WP8.

This document is the working reference for cutting the first release. It was produced by a full repo review: parallel deep-dives into every subsystem, plus local execution of all CI legs and GitHub-side verification (releases, tags, workflow-run history, ghcr.io image registry). Facts marked **[verified]** were executed/checked directly during this review; unmarked findings come from code exploration.

---

## 1. Verdict (TL;DR)

The **code is in better shape than the project's own paperwork suggests** — all five CI legs pass locally at HEAD **[verified]**, the architecture is clean and layered, and the February `AUDIT.md` P0s were mostly fixed (contrary to what a stale reading implies). What's missing for a first release is almost entirely **release engineering and packaging**, plus a handful of real product bugs:

1. **CI has been 100% red on `main` since 2026-02-14** (~150 consecutive failed runs across all 5 workflows) **[verified]** — yet tests, lint, Python tests, and docs all pass locally today **[verified]**, and a fresh push during this review came back **green: Tests ✅ and Lint ✅ on GitHub Actions (2026-06-10)** **[verified]**. The historical red was environmental, not code. Only the e2e failure cause is structural (see §6).
2. **No release machinery exists at all**: zero git tags, zero GitHub releases **[verified]**, no release workflow, no CHANGELOG, no Helm/PyPI publishing.
3. **Versions are chaos**: `0.0.6` / `0.0.7` / `0.1.0` / `0.0.23` / `0.0.5` / `0.0.0` across components (§7).
4. **Three shipped-but-broken edges**: Helm chart is missing the AgentWorkflow CRD **[verified]**, the default AgentTask runtime image was never published (registry 403) **[verified]**, and webhooks can't be enabled via the chart **[verified]**.
5. **Public-facing docs are placeholder-level**: root README is 8 bytes (`# flokoa`), `flokoa` pyproject description is "Add your description here", operator README is kubebuilder boilerplate **[verified]**.

**Recommendation:** Release as **v0.1.0 (alpha)** after completing work packages WP1–WP4 (§13). Approx. 4–6 focused sessions. Cut/flag `flokoa-managed-task` and `flokoa-codemode-mcp` as experimental.

---

## 2. Repo snapshot

- **History:** 178 commits, all dated 2026-02-10 → 2026-02-28 (intense 3-week sprint; CI runs exist from late January, so history was likely rebased/rewritten at some point). Dormant since Feb 28. Authors: Daniel Nyari (135), Claude (43). **[verified]**
- **No tags, no GitHub releases.** **[verified]**
- **The repo has evolved well past `CLAUDE.md`** (see §12): an embedded Nuxt 4 web UI, two extra Python packages (`flokoa-common`, `flokoa-codemode-mcp`), new operator layers (`operator/internal/domain`, `operator/internal/converter`, `operator/internal/errors`, `operator/internal/config`), SSE watch + AG-UI playground endpoints.
- Two prior self-audits exist and remain useful: `operator/AUDIT.md` (2026-02-18, 32 findings — **majority of P0s since fixed**, see §11) and `OVERMOCKING_REVIEW.md` (Python test-quality findings — **unaddressed**).

### Component inventory

| Component | Location | One-liner |
|---|---|---|
| Operator (Go 1.24) | `operator/` | 6 CRDs under `agent.flokoa.ai/v1alpha1`, controllers, webhooks, layered domain/app/infra |
| gRPC/REST server | `operator/cmd/server`, `operator/internal/server` | Separate binary; 5 gRPC services + grpc-gateway REST + SSE watch + AG-UI playground; serves embedded UI |
| Web UI | `operator/ui/src` | Nuxt 4 SPA (agents/models/providers/tools/workflows/runs DAG/playground/OIDC login), embedded via `go:embed` into server binary |
| Argo integration | `operator/internal/controller/agentworkflow_compiler.go`, `operator/plugins/a2a/` | AgentWorkflow→WorkflowTemplate compiler + A2A executor plugin (sidecar, port 4355) |
| Helm chart | `operator/charts/flokoa/` | chart 0.1.0 / appVersion 0.0.7; controller+server+A2A plugin+Dex templates |
| Python SDK workspace | `sdk/python/` | 6 uv-workspace packages (see §4) |
| Docs site | `docs/`, `zensical.toml` | Zensical/MkDocs; 10 pages + 26 example YAMLs; deployed to GitHub Pages on main |

---

## 3. Architecture overview (for future sessions)

### 3.1 Operator core

Layering is consistent and clean:

```
internal/controller/   reconcilers (one per CRD) — thin, delegate to app layer
internal/app/agent/    orchestration (reconcile.go + tool/model/instruction sub-reconcilers, DI via Deps struct)
internal/domain/       pure functions: agent/model/modelprovider/tool validation, hash (drift detection)
internal/infra/        repo pattern (CRUD per resource + fakes) and builder (Deployment/Service construction)
internal/converter/    CRD ↔ protobuf conversion (pure, well tested)
internal/errors/       error classification: permanent (no requeue) / dependency (30s requeue) / transient (backoff)
```

- **Agent** controller owns Deployment + Service + ConfigMaps; watches AgentTool/Instruction/ConfigMap/Secret/Model/ModelProvider for cross-resource reactions. Two runtime modes: `standard` (user image) and `template` (managed runtime image, default `ghcr.io/danielnyari/flokoa-cli:latest` — `operator/internal/infra/builder/deployment.go:16`).
- **AgentTool** → ConfigMap with resolved OpenAPI spec (inline value / ConfigMap ref / HTTP fetch). **Instruction** → ConfigMap with prompt. **Model**/**ModelProvider** → validation + status (provider inference, secret hash change detection).
- **AgentWorkflow** → compiled to Argo WorkflowTemplate (SpecHash drift detection; status tracks compilation only — *run* status is intentionally surfaced via the gRPC/REST API and Argo, not the CRD).
- All controllers use `updateStatusWithRetry` (`operator/internal/controller/status.go`) — conflict-safe status updates **[verified]**.
- All **6 webhooks exist and are registered** (`operator/api/v1alpha1/*_webhook.go` for 5 CRDs + `operator/internal/webhook/v1alpha1/agentworkflow_webhook.go` for DAG/cycle/expression validation), but only behind `--enable-webhooks` (default **false**, needs TLS) — `operator/cmd/main.go:92,288-310` **[verified]**.

### 3.2 gRPC/REST server + UI

- 5 services: Agent (read-only by design), AgentTool/Model/ModelProvider (full CRUD), AgentWorkflow (Get/List + SubmitRun/ListRuns/GetRun). **No Instruction service exists** (no proto file) **[verified]**.
- grpc-gateway REST under `/api/v1alpha1/...`; gRPC `Watch*` RPCs return `Unimplemented` — realtime is delivered instead via **SSE watch endpoints** (`operator/internal/server/watch.go`) for all 5 resources + workflow runs.
- **Playground**: `POST /api/v1alpha1/namespaces/{ns}/agents/{name}/playground` bridges to A2A agents and streams **AG-UI events** over SSE (`operator/internal/server/playground.go`).
- **Auth:** optional OIDC (`AUTH_ENABLED`, default false), bearer-token validation, claims in context. **No authorization layer** — any authenticated user can do everything. gRPC is plaintext (no TLS option); reflection enabled by default.
- **UI:** Nuxt 4 + @nuxt/ui + TanStack Table + Vue Flow (workflow DAG). Embedded into the server binary via `operator/ui/embed.go` (`dist/` only has `.gitkeep`; the server Dockerfile builds the UI in-stage). Settings pages are front-end stubs with no backend.

### 3.3 Argo Workflows integration

- **Compiler** supports: agent / agentTask (Marvin) / container / http / switch tasks, `dependsOn`, `condition` (→ Argo `when`), params with defaults, output passing (`{{tasks.x.output}}` incl. `fromJson()` field extraction), per-task + workflow retries/timeouts, service account config. Not supported: loops/`withItems`, inline tool definitions (explicitly rejected), nested workflows. Artifact-IO exists behind `--artifact-io-enabled` flag.
- **A2A executor plugin** (`plugins/a2a/`): sidecar HTTP server (4355) using `a2a-go v0.3.6`; resolves agents via Agent CR `status.url` or convention DNS fallback; per-task state persisted to a ConfigMap store (survives restarts); tolerates 5 transient poll errors; 5m default timeout; propagates W3C traceparent; auth via Argo token, **fatal at startup if token missing** unless `FLOKOA_DEV_MODE=true` (`plugins/a2a/main.go:46-58`) **[verified]**.
- Argo pinned at v3.7.9 (Makefile + go.mod). Installed via `make deploy-argo-workflows` (quick-start manifest + plugin-enable patch) — not part of the Helm chart.

### 3.4 Python SDK (uv workspace, Python ≥ 3.13)

| Package | Version | State |
|---|---|---|
| `flokoa` | 0.0.5 | The public SDK/CLI (`flokoa run -m module:agent`). pydantic-ai + google-adk integrations, OpenAPI tools (OAuth2/service-account exchangers, SSRF validation), A2A serving. **467 tests pass, 79% cov [verified]**. PyPI metadata is placeholder. |
| `flokoa-types` | 0.0.5 | Generated Pydantic v2 models from CRDs (`make generate-python-models`). In sync. Ready. |
| `flokoa-common` | 0.1.0 | Shared auth + OpenAPI parsing used by everything. **Zero tests.** |
| `flokoa-managed-agent` | 0.1.0 | Operator-deployed pydantic-ai runtime (boots from `/etc/flokoa/agent-config.json`, serves A2A). Well tested (unit + e2e). **Shipped as the `flokoa-cli` image** — `operator/Makefile:515-516` builds it from `flokoa-managed-agent/Dockerfile` (confusing naming) **[verified]**. |
| `flokoa-managed-task` | 0.1.0 | Marvin task runtime. **Code is actually complete** (all 5 task types) but README says "scaffold only — not yet implemented"; **zero tests; image never published** (see §7.3). |
| `flokoa-codemode-mcp` | 0.1.0 | FastMCP server: OpenAPI → Python stubs, sandboxed execution via pydantic-monty. ~70% done: **zero tests, no entrypoint/`__main__`, empty README.** |

---

## 4. Component maturity matrix

| Component | Maturity | Notes |
|---|---|---|
| CRD schemas (6) | ★★★★☆ | Comprehensive, validated, provider-specific model params are thorough |
| Agent/Model/Provider/Tool/Instruction controllers | ★★★★☆ | Layered, status-retry, structured errors; watch mappers list across all namespaces (scale concern) |
| AgentWorkflow compiler | ★★★★☆ | 77 test cases / 4,030-line test file; no loops/inline tools |
| A2A executor plugin | ★★★☆☆ | Solid design (state persistence, tracing); thin unit tests, single happy-path e2e |
| gRPC/REST server | ★★★☆☆ | 5 services + SSE + playground; missing Instruction service, no TLS, no authz |
| Web UI | ★★★☆☆ | Feature-rich modern SPA incl. playground + DAG view; settings stubs; no UI e2e tests |
| Helm chart | ★★☆☆☆ | 29 templates incl. Dex; **missing AgentWorkflow CRD**, no webhook support, no README, not published |
| Python `flokoa` + `flokoa-types` + `managed-agent` | ★★★★☆ | Tested and working; metadata placeholder |
| `flokoa-common` / `managed-task` / `codemode-mcp` | ★★☆☆☆ | Untested and/or unshipped |
| Docs site | ★★☆☆☆ | Good CRD/getting-started pages; placeholder branding, broken links, no UI/SDK/server docs |
| Release engineering | ☆☆☆☆☆ | Nothing exists |

---

## 5. Test & quality status (all executed locally at HEAD) **[verified]**

| Check | Command | Result |
|---|---|---|
| Operator unit tests | `cd operator && make test` (envtest, K8s 1.33) | **PASS** (exit 0) |
| Operator lint | `cd operator && make lint` (golangci-lint v2.1.0, same pin as CI) | **PASS** — 0 issues |
| `go mod tidy` drift | `cd operator && go mod tidy && git status` | **clean** |
| `make manifests generate` drift | (runs inside `cd operator && make test`) | **clean** — generated artifacts in sync |
| Python SDK tests (CI-style) | `cd sdk/python/flokoa && uv run python -m pytest --cov` | **PASS** — 467 passed, **79% coverage** |
| Python workspace-root pytest | `cd sdk/python && uv run pytest` | **FAILS at collection** — `flokoa-managed-agent/tests` ImportPathMismatchError + `tests/e2e` ModuleNotFoundError (config issue, not failing tests) |
| Docs build | `zensical build --clean` | **PASS** — 3 broken-link warnings (`docs/index.md:67` → `e2e-test-plan.md`; `:278` → `../sdk/python/README.md`; `:279` → `../operator/README.md`) |

### Go coverage by package (from `make test` cover.out)

- Strong: `domain/hash` 100%, `domain/modelprovider` 100%, `errors` 94.1%, `webhook/v1alpha1` 89.1%, `domain/agent` 73.1%, `converter` 68.6%, `controller` 63.2%
- Weak/zero (direct): `server` 28.2%, `domain/model` 26.2%, and **0%**: `app/agent` (exercised only indirectly via controller envtests), `infra/builder`, `infra/repo`, `config`, `telemetry`, `domain/tool`
- ~41 Go test files (~167 controller test cases, 100 converter test funcs). Tests run against real envtest API server — no mock-heavy patterns (confirmed by `OVERMOCKING_REVIEW.md`).

### Test counts elsewhere

- E2E (`operator/test/e2e/`): 8 Ginkgo cases over 3 files — full Agent stack (petstore fixture), Argo + A2A plugin + workflow run via REST, manager health/metrics. Covers all 6 CRDs. **Requires a real `OPENAI_API_KEY` and makes real LLM calls.**
- UI: 14 Vitest files (auth middleware, components, feature pages). No browser e2e.
- Python: `flokoa` extensive; `flokoa-managed-agent` good (unit+e2e); `flokoa-common`/`managed-task`/`codemode-mcp` **zero tests**.
- Known test-quality debt (`OVERMOCKING_REVIEW.md`, unaddressed): `tests/flokoa_cli/integrations/google_adk/test_google_adk_agent_executor.py` execute-tests are mock-wiring only (HIGH), FlokoaToolset tests near-tautological (MEDIUM).

---

## 6. CI/CD: state and root causes **[verified via GitHub API]**

373 workflow runs on `main`. **Fresh signal (2026-06-10, this review's branch push): Tests ✅ success, Lint ✅ success** — first green runs since January; the E2E run fails at the `OPENAI_API_KEY` guard as explained below. Historical conclusions:

- **Every run from 2026-02-14 → 2026-02-28 failed** — all 5 workflows (Tests, Lint, E2E, Python SDK Tests, Documentation), every push.
- Last observed E2E success: 2026-01-25 (Lint/Tests/Docs were already failing then).
- **Historical logs are expired (HTTP 410)** — root causes for Tests/Lint/Python/Docs can't be recovered from GitHub. But since all of them **pass locally today at the same pinned versions**, the next push will give clean signal.
- **E2E failure cause is structural and still present:** `test/e2e/helpers_test.go:438-440` hard-fails without `OPENAI_API_KEY`, and `.github/workflows/test-e2e.yml` passes **no env/secrets at all**. E2E also runs on *every push of any branch* and makes real (paid) OpenAI calls.
- `docs.yml` deploys to GitHub Pages (`environment: github-pages`) — the build passes locally, and **the repo is currently private** **[verified]**; GitHub Pages on private repos requires a paid plan, which is the most likely cause of the Documentation failures. Decide: make the repo public at release time (Pages then works) or drop/disable `docs.yml` until then.
- **Image publishing:** only `flokoa-operator` is pushed by CI (test.yml `build-image`, main-only, gated on the failing test job — so it has been stale since CI went red). `flokoa-server`, `flokoa-a2a-plugin`, `flokoa-cli` are pushed manually via `make docker-push*`. All four exist on ghcr.io with `:latest` **[verified — registry returns 200]**.

---

## 7. Release blockers (P0)

### 7.1 Re-establish green CI
Push to a branch to get fresh runs (this review's branch push will do exactly that). Fix what's actually red. Known-required fixes:
- `test-e2e.yml`: wire `OPENAI_API_KEY` from repo secrets **and** make e2e conditional (e.g., only on main/label, or skip-gracefully when the secret is absent). Consider a fake/stub model provider for CI to remove paid-API dependency and flake.
- Decide whether `docs.yml` Pages deployment is configured for the repo.

### 7.2 Helm chart missing the AgentWorkflow CRD **[verified]**
`operator/charts/flokoa/files/crds/` contains 5 of 6 CRDs — `agent.flokoa.ai_agentworkflows.yaml` is absent (the other 5 are byte-identical with `config/crd/bases/`). Any chart install cannot create AgentWorkflows. Add the file + a CI check (or make target) that keeps `files/crds/` in sync with `config/crd/bases/`.

### 7.3 Default AgentTask image was never published **[verified]**
`internal/controller/agentworkflow_compiler.go:37` defaults to `ghcr.io/danielnyari/flokoa/managed-task:latest` → registry returns 403 (nonexistent/private; also note the path style differs from every other image). `sdk/python/flokoa-managed-task/Dockerfile` exists but there is **no Makefile build/push target** for it. Either publish it under a consistent name (`ghcr.io/danielnyari/flokoa-managed-task`) or exclude `agentTask` from v0.1.0 and document.

### 7.4 Version alignment **[verified values]**

| Place | Value | File |
|---|---|---|
| Operator `VERSION` | 0.0.6 | `operator/Makefile:6` |
| Helm chart `version` | 0.1.0 | `operator/charts/flokoa/Chart.yaml:5` |
| Helm chart `appVersion` | 0.0.7 | `operator/charts/flokoa/Chart.yaml:6` |
| Kustomize image tags | 0.0.23 | `operator/config/manager/kustomization.yaml:9`, `operator/config/server/kustomization.yaml:13` |
| `flokoa`, `flokoa-types` | 0.0.5 | pyprojects |
| other 4 Python packages | 0.1.0 | pyprojects |
| workspace root | 0.0.0 | `sdk/python/pyproject.toml:3` |
| UI package.json | (none) | `operator/ui/src/package.json` |

**Proposal:** set everything to **0.1.0** for the first release; keep chart version == appVersion until there's a reason to decouple.

### 7.5 Create release machinery
None exists (no tags/releases/workflow/CHANGELOG). Minimum viable for v0.1.0, as a tag-triggered (`v*`) workflow:
1. Run tests + lint.
2. Build & push **all** images with the semver tag (+ `latest`): operator, server, a2a-plugin, flokoa-cli (consider renaming → `flokoa-managed-agent`), managed-task (if kept).
3. Generate a single-file `install.yaml` (kustomize bundle) and attach to a GitHub Release.
4. Package + publish the Helm chart (easiest: OCI push to `ghcr.io/danielnyari/charts/flokoa`).
5. (Optional for v0.1.0) Publish `flokoa` + `flokoa-types` to PyPI — requires §7.6 metadata first.
6. `CHANGELOG.md` with a v0.1.0 entry.

### 7.6 Public-facing placeholder text **[verified]**
- Root `README.md` = `# flokoa` (8 bytes). Needs: what Flokoa is, architecture diagram, quickstart (helm install + first agent), links.
- `operator/README.md` = kubebuilder boilerplate with `TODO(user)` markers.
- `sdk/python/flokoa/pyproject.toml`: `description = "Add your description here"`; no license/classifiers/urls. Empty READMEs: `sdk/python/README.md`, `sdk/python/flokoa/README.md`, `flokoa-common`, `flokoa-codemode-mcp`.
- `zensical.toml`: site name "Documentation", default-template description, `site_url` commented out.

### 7.7 Unpinned `:latest` defaults in code
`DefaultTemplateRuntimeImage = "ghcr.io/danielnyari/flokoa-cli:latest"` (`operator/internal/infra/builder/deployment.go:16`) and the managed-task default (§7.3). Pin to the release version at build time (ldflags or generated constant) so operator v0.1.0 deploys runtime v0.1.0.

---

## 8. High priority (P1) — should land in or immediately after v0.1.0

1. **Instruction gRPC/REST service missing** — UI/API users can't manage Instructions (`operator/server/proto/` has no instruction service) → also absent from UI.
2. **Webhooks unreachable in practice** **[verified]** — chart has no cert handling, no `--enable-webhooks`, no `ValidatingWebhookConfiguration` templates (grep over chart returns nothing). Either ship opt-in webhook support (cert-manager) in the chart or document that admission validation is off and rely on controller-side validation (which exists and is solid).
3. **Security posture documentation** — auth disabled by default, no authorization layer (any authenticated user = full CRUD), gRPC plaintext, gRPC reflection on by default, server ServiceAccount permissions (AUDIT.md flagged escalation). For an alpha this can be *documented* rather than fixed, but it must be written down.
4. **Workspace-root pytest broken** **[verified]** — `uv run pytest` from `sdk/python/` dies at collection (test module name collisions / missing packages config). Fix `conftest.py`/`rootdir`/`__init__.py` so the documented workspace commands work.
5. **Zero-test packages**: `flokoa-common` (it underpins everything), `flokoa-managed-task`, `flokoa-codemode-mcp`; Go `operator/internal/telemetry`, `operator/internal/config`, `operator/internal/domain/tool`, `operator/internal/infra/builder`, `operator/internal/infra/repo` (direct).
6. **google-adk executor overmocked tests** (`OVERMOCKING_REVIEW.md`, HIGH) — fix or annotate as known debt.
7. **`flokoa-managed-task` decision** — code complete but README claims scaffold, no tests, no image. Recommend: mark experimental, exclude from release notes, or invest a session (tests + image + README).
8. **Image/runtime naming** — `flokoa-cli` image actually contains the managed-agent runtime; rename or document.
9. **Docs site gaps** — no pages for: web UI, gRPC/REST API, playground, Python SDK, codemode-mcp, Helm install. `docs/examples/agent/minimal-agent.yaml` references `ghcr.io/example/simple-agent:latest`.
10. **Watch mappers list across all namespaces** (AUDIT.md 4b) — scale concern, fine for alpha.

---

## 9. Polish / later (P2)

- Prometheus metrics (ports configured, nothing implemented), Grafana dashboard.
- AgentWorkflow run-status: document the "status via API, not CRD" design decision prominently.
- Stuck-workflow requeue timeout (AUDIT 4d), `MaxConcurrentReconciles`, PodDisruptionBudget, NetworkPolicy.
- Multi-arch images (currently amd64), SECURITY.md, CONTRIBUTING.md.
- UI: browser e2e tests, settings backend, dark mode, bulk ops, export/import YAML.
- A2A plugin: circuit breaker/backoff for poll retries; more e2e under failure injection.
- Go coverage upload to Codecov (Python already has it; target 90% configured in `sdk/python/flokoa/codecov.yaml`).

---

## 10. What's actually solid (don't re-litigate in future sessions)

- Layered operator architecture + repo pattern + structured error classification — consistently applied.
- Conflict-safe status updates everywhere (`updateStatusWithRetry`) **[verified]**.
- Generated artifacts (CRDs, deepcopy, Python models) are **in sync** at HEAD **[verified]**.
- AgentWorkflow compiler: most-tested code in the repo.
- A2A plugin auth (fatal without token unless `FLOKOA_DEV_MODE`), state persistence, trace propagation.
- envtest-based controller tests + clean lint at CI-pinned versions.
- E2E suite design (Kind + real Argo + petstore fixture) — just needs CI wiring.

---

## 11. Status of the February audits

`operator/AUDIT.md` (2026-02-18, 32 findings). Re-verified during this review:

**Fixed since the audit [verified]:** discarded status updates (now retry-helper everywhere; the `_ =` pattern survives only inside AUDIT.md's own examples) · `Runtime.Standard` nil validation (`operator/internal/domain/agent/validate.go:27`) · finalizer NotFound-tolerance (`operator/internal/controller/agenttool_controller.go:383-384` ignores IsNotFound; same pattern elsewhere) · A2A auth fatal-on-missing-token · A2A plugin state persistence (ConfigMap store) · structured error types.

**Still open:** webhooks off-by-default + no chart support (§8.2) · cross-resource reference validation at admission · all-namespace watch mappers · stuck-workflow timeout · metrics · server SA least-privilege review · `OVERMOCKING_REVIEW.md` Python findings.

**Action:** a cleanup session should re-triage AUDIT.md line-by-line, mark fixed items, and fold the remainder into GitHub issues — then delete both audit files from the repo root (they'd be confusing in a public v0.1.0).

---

## 12. Stale internal docs to refresh (`CLAUDE.md` et al.)

- Root `CLAUDE.md` omits: `operator/ui/` (entire web UI), `operator/internal/{domain,converter,errors,config}`, `flokoa-common`, `flokoa-codemode-mcp`, SSE/playground endpoints, Dex in the chart; version table will be wrong after alignment; "5 CI workflows" section should note release workflow once added.
- `sdk/python/flokoa-managed-task/README.md` claims "scaffold only" — code is complete.
- `operator/CLAUDE.md` and skills (`.claude/skills/*`) are largely accurate — keep.

---

## 13. Suggested work packages for subsequent Claude Code sessions

Sized to one session each, ordered by dependency:

**WP1 — CI resurrection** *(do first; everything else needs signal)*
Fix `test-e2e.yml` secret wiring + conditional skip; confirm Tests/Lint/Python workflows are green on a fresh push; add Go coverage upload; consider concurrency-cancel + path filters (Tests/Lint currently run on every push of every branch); resolve docs.yml vs private-repo Pages (§6). Files: `.github/workflows/*`, `operator/test/e2e/helpers_test.go`. Note: the repo is private — going public (and when) is a user decision that gates Pages, ghcr visibility expectations, and PyPI links.

**WP2 — Version alignment + release workflow**
Set 0.1.0 everywhere (§7.4 table); write `release.yml` (tag-triggered: images w/ semver tags incl. managed runtimes, install.yaml bundle, Helm OCI push, GitHub Release, optional PyPI); pin `DefaultTemplateRuntimeImage`/managed-task default to the release tag; add `CHANGELOG.md`. Files: `operator/Makefile`, `operator/charts/flokoa/Chart.yaml`, `operator/config/*/kustomization.yaml`, all pyprojects, `operator/internal/infra/builder/deployment.go:16`, `operator/internal/controller/agentworkflow_compiler.go:37`.

**WP3 — Helm chart completeness**
Add `files/crds/agent.flokoa.ai_agentworkflows.yaml` + sync check; chart README; optional webhook support (cert-manager) or documented limitation; decide Argo install story (docs vs subchart); validate `helm template` against a Kind cluster.

**WP4 — Public docs & metadata**
Root README (pitch, architecture, quickstart), operator README, SDK READMEs + pyproject metadata (description/license/classifiers/urls), zensical.toml branding + site_url, fix 3 broken links, new pages: UI tour, REST/gRPC API, Helm install, Python SDK; fix `minimal-agent.yaml` example image; refresh root CLAUDE.md (§12).

**WP5 — managed-task & codemode-mcp disposition**
Either productionize managed-task (tests, image target + publish, README) or gate it; codemode-mcp: add entrypoint, README, basic tests, or mark experimental and exclude from docs.

**WP6 — Server hardening**
Add InstructionService (proto + impl + converter + UI); TLS option for gRPC; reflection default off; document security posture (authn/authz state); review server ServiceAccount RBAC.

**WP7 — Test debt**
`flokoa-common` test suite; fix workspace-root pytest collection; google-adk overmocking fix; unit tests for `operator/internal/infra/builder`, `operator/internal/domain/tool`, `operator/internal/config`; UI Playwright smoke test.

**WP8 — AUDIT close-out**
Re-triage AUDIT.md + OVERMOCKING_REVIEW.md per §11, convert remainder to issues, remove both files from the repo.

---

## 14. Quick reference: verified commands & results (2026-06-10)

```
operator/  make test            → PASS (envtest 1.33; coverage: controller 63.2%, webhook 89.1%, converter 68.6%)
operator/  make lint            → PASS (golangci-lint v2.1.0, 0 issues)
operator/  go mod tidy          → no diff
sdk/python/flokoa  uv run python -m pytest --cov  → 467 passed, 79% cov
repo root  zensical build       → PASS (3 broken-link warnings)
GitHub     releases/tags        → none; CI red on main since 2026-02-14; run logs expired (410)
GitHub     fresh CI (this branch, 2026-06-10) → Tests ✅  Lint ✅  (E2E fails on missing OPENAI_API_KEY wiring)
ghcr.io    flokoa-operator/server/a2a-plugin/cli :latest → exist (200); danielnyari/flokoa/managed-task → 403
```
