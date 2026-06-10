# 09 — CLI Harness Verbs: `init` / `deploy` / `invoke` / `logs`

**Phase:** 1 · **Size:** M · **Depends on:** 02 (published images), benefits from 05/06 · **Enables:** 10 (quickstart is built on these verbs)

## Goal

Close the DX loop developers now expect from a harness CLI (`agentcore configure/launch/invoke/logs` is the reference): scaffold a project, deploy it as CRDs, invoke it, and tail it — all from `flokoa`.

## Current state

- The CLI is Click-based with exactly one command: `flokoa run -m module:agent [--host] [--port] [--framework]` in `flokoa/src/flokoa/__main__.py`, which imports the user's agent object, picks an executor via `get_executor_cls(IntegrationType)`, builds the A2A FastAPI app (`_get_app`), and runs uvicorn. Everything lives in this single file.
- No `init`, no deploy/apply, no invoke, no logs. No kubernetes client dependency anywhere in the SDK.
- Assets the new verbs can lean on:
  - **Playground endpoint** on the control-plane server: `POST /api/v1alpha1/namespaces/{ns}/agents/{name}/playground` streams AG-UI SSE events (`internal/server/playground.go`) — a ready-made invoke path that works from outside the cluster, through authn/authz (05/06), without port-forwards.
  - Sample CRs in `docs/examples/` and `operator/config/samples/` as scaffold templates.
  - Agent Deployments are operator-owned with deterministic labels (set in `internal/infra/builder/deployment.go` — confirm exact label keys when implementing `logs`).

## Target design

### Package restructure (prerequisite, pure refactor)

`flokoa/src/flokoa/cli/` package: `__init__.py` (Click group, re-exported so `python -m flokoa` and the console-script keep working), `run.py` (move existing), `init.py`, `deploy.py`, `invoke.py`, `logs.py`, `_kube.py` (shared k8s client helpers), `_config.py` (CLI context: server URL, token, namespace — env `FLOKOA_SERVER_URL`, `FLOKOA_TOKEN`, `FLOKOA_NAMESPACE`, overridable per-command flags).

New optional extra: `k8s = ["kubernetes>=29", "httpx-sse>=0.4"]` — `deploy`/`logs` need the k8s client; `invoke` needs SSE. Each command `_try_load`-guards its imports with an actionable "pip install flokoa[k8s]" error (house pattern from `integrations/__init__.py`).

### `flokoa init [DIRECTORY] [--name NAME] [--template minimal|tools]`

Scaffolds:

```
my-agent/
├── pyproject.toml        # deps: flokoa[pydantic-ai]
├── src/my_agent/agent.py # pydantic_ai.Agent instance, ready for `flokoa run`
├── flokoa.yaml           # multi-doc K8s manifests: Agent (runtime.type: template),
│                         #   Model + ModelProvider (apiKeySecretRef), optional AgentTool
└── README.md             # run locally → deploy → invoke, in 3 commands
```

Templates are package data (`flokoa/cli/templates/…`, rendered with `string.Template` — no jinja dependency for v1). The generated `Agent` uses `runtime.type: template` so `deploy` works without the user building images; the README shows the `standard` runtime as the BYO-image path.

### `flokoa deploy [-f flokoa.yaml] [-n NAMESPACE] [--dry-run]`

Applies the manifest docs via **server-side apply** (k8s dynamic client, `field_manager="flokoa-cli"`, `force=False`) — SSA gives idempotent create-or-update without local three-way merge logic. Then waits (`--wait/--timeout`, default 120s) for `Agent.status.phase == Running` (watch via the dynamic client), printing status conditions on failure. Explicitly **not** a package manager: it applies CRDs from a file; Helm owns the platform install.

### `flokoa invoke AGENT [MESSAGE] [-n NS] [--context-id ID] [--url URL]`

Two transports, auto-selected:
- **Default (control plane)**: POST to `{FLOKOA_SERVER_URL}/api/v1alpha1/namespaces/{ns}/agents/{name}/playground` with bearer `FLOKOA_TOKEN`, streaming AG-UI SSE deltas to stdout as they arrive. Works from a laptop with no cluster network tricks; rides 05/06 auth.
- **`--url` (direct A2A)**: send an A2A message (a2a-sdk client classes, already a dependency) straight to an agent endpoint — the local-dev path against `flokoa run` (default `http://localhost:10001`). `--context-id` sets the A2A contextId so multi-turn sessions (03) are exercisable from the CLI: repeated invokes with the same id continue the conversation.
- `--json` flag emits raw events for scripting.

Confirm the playground request/response schema from `internal/server/playground.go` + the gateway annotations in `operator/server/proto/` before implementing; the CLI must speak exactly that contract, not a guess.

### `flokoa logs AGENT [-n NS] [-f] [--tail N]`

Resolves the agent's Deployment via owner labels (exact selector taken from `BuildDeployment`), then streams pod logs via the k8s client (`read_namespaced_pod_log(follow=…)`), multiplexing replicas with pod-name prefixes (kubectl-style). `--previous` passthrough.

## Implementation plan

1. CLI package refactor (no behavior change) + tests keep passing (`tests/flokoa_cli/` mirrors the new layout).
2. `_config.py` + `_kube.py` (kubeconfig loading: default chain — in-cluster, `KUBECONFIG`, `~/.kube/config`).
3. `init` with templates + golden-file tests (`flokoa init` then assert tree + `flokoa run` import-check on the generated agent via TestModel).
4. `deploy` with SSA + wait loop (fake dynamic client unit tests; envtest-backed integration optional).
5. `invoke` (httpx + httpx-sse for playground; a2a client for `--url`), `logs`.
6. `--help` polish: the group docstring becomes the 4-verb harness story; README/docs updated (10 owns the full walkthrough).

## Testing

- Unit: Click `CliRunner` for arg parsing/error paths of every verb; `init` golden tree; `invoke --url` against an in-process `flokoa run` app (FastAPI TestClient with the A2A app from `_get_app` — fixture pattern already exists in `flokoa-managed-agent/tests`); `deploy` against a mocked dynamic client asserting SSA payloads.
- E2E (Kind, extends `sdk/python/tests/e2e/`): `init → deploy → invoke → logs` full loop against a real cluster with the chart installed; invoke twice with `--context-id` to assert session continuity once 03/04 are merged.

## Acceptance criteria

- A developer with a kubeconfig and an OpenAI key goes from nothing to a deployed, invocable agent with: `uvx flokoa init my-agent && cd my-agent && flokoa deploy && flokoa invoke my-agent "hello"`.
- `flokoa run` behavior is byte-identical post-refactor.
- Every verb degrades with actionable errors when its optional extra is missing.

## Out of scope

- `flokoa traces` (needs a query backend; revisit after 08/10 — log the idea in the CLI help as planned). Project build/push for standard runtimes (`flokoa build` — follow-up). Workflow verbs (`submit`/`runs` — natural follow-up mapping to `AgentWorkflowService.SubmitRun`). Windows shell niceties.
