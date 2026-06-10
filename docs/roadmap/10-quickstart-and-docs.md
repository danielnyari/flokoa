# 10 — Quickstart, Docs IA, Grafana Dashboard

**Phase:** 1 (closes it) · **Size:** M · **Depends on:** 03–09 (documents what they shipped) · **Enables:** the public harness claim

## Goal

Ship the artifacts that make the harness claim *legible*: a 5-minute quickstart that exercises the whole loop (declare → deploy → invoke → multi-turn → trace), a docs information architecture that covers the SDK and security (today's biggest doc holes), and the Grafana dashboard that shows an invocation end-to-end. Positioning line all of it supports: **"the open-source agent harness for Kubernetes"** — every claim in the docs must be true by construction (it documents merged units only).

## Current state

- `docs/` (Zensical/MkDocs site, `zensical.toml` at repo root; deployed by `docs.yml` on main): 10 pages — CRD references (`agent.md`, `agenttool.md`, `model.md`, `modelprovider.md`), `getting-started.md`, `architecture.md`, `quick-reference.md`, `agenttrigger-rfc.md` + ~26 example YAMLs in `docs/examples/`.
- Known gaps (RELEASE_REVIEW P1 #9 + audit): **no Python SDK docs, no gRPC/REST API docs, no security docs, no sessions/memory docs (new), 3 broken links**, root README is 8 bytes (fixed in 02).
- New roadmap docs land in `docs/roadmap/` (this directory) — decide whether they're in the published nav (recommend: yes, under "Project/Roadmap"; transparency is a feature for an alpha OSS project).

## Target design

### Docs IA

```
docs/
├── index.md                    # positioning + capability matrix (honest: pod-level isolation)
├── quickstart.md               # THE 5-minute path (below)
├── concepts/
│   ├── architecture.md         # refresh existing + absorb 00-target-architecture diagrams
│   ├── sessions-and-memory.md  # contextId semantics, backends, TTL (03/04)
│   └── observability.md        # traces/metrics, collector setup, dashboard (08)
├── guides/
│   ├── security.md             # endpoint auth (05), control-plane TLS+authz (06), threat model
│   ├── tools.md                # openapi + mcp (07), secret headers
│   ├── workflows.md            # AgentWorkflow + A2A plugin (exists in pieces; consolidate)
│   └── local-development.md    # flokoa run / init / invoke --url loop (09)
├── reference/
│   ├── crds/ (existing 4 + instruction + agentworkflow + agenttrigger)
│   ├── cli.md                  # generated from Click (--help capture script in hack/)
│   ├── api.md                  # REST surface from grpc-gateway (link openapi output if buf emits it)
│   └── runtime-contract.md     # /etc/flokoa/* files + FLOKOA_* env table (from 00)
└── examples/                   # existing YAMLs, each linked from a guide (orphan examples = dead docs)
```

Migration is `git mv` + nav edits in `zensical.toml` + redirects if the site supports them; fix the 3 broken links as part of the move (they're enumerated in RELEASE_REVIEW).

### Quickstart (the artifact that sells the harness)

One page, copy-pasteable, on Kind:

1. `helm install flokoa oci://ghcr.io/danielnyari/charts/flokoa` (+cert-manager note) — from 01/02.
2. `kubectl create secret … openai-key` + apply `ModelProvider`/`Model`.
3. `uvx flokoa init my-agent && flokoa deploy` — from 09.
4. `flokoa invoke my-agent "remember my name is Dana" --context-id demo` then `flokoa invoke my-agent "what's my name?" --context-id demo` — **the multi-turn moment** (03/04), the line that separates "deployment platform" from "harness".
5. `flokoa logs my-agent` + screenshot of the trace in Grafana/Tempo and the token panel (08).

Every command in the quickstart runs in CI (see Testing) — a quickstart that can rot is a liability.

### Grafana dashboard

`operator/charts/flokoa/dashboards/agent-harness.json` (provisioned via configmap when `telemetry.collector.enabled`, 08's plumbing):
- Row 1: requests/s + error rate per agent (`flokoa.agent.requests`), p50/p95 duration.
- Row 2: token usage by agent/model/type (`gen_ai.client.token.usage`) + estimated cost variable (per-model $/token as dashboard variables — display-only; enforcement is 12).
- Row 3: trace panel / Tempo link for `flokoa.agent.invoke` spans.

## Implementation plan

1. IA migration + nav + link fixes; `index.md` with the capability matrix (source: `AGENT_HARNESS_REVIEW.md` §6 scorecard, updated to post-Phase-1 truth).
2. Write the four new concept/guide pages from the merged units' specs + code (specs in `docs/roadmap/` stay as design records; user docs are written fresh, task-oriented).
3. `reference/runtime-contract.md` — promote the contract table from `00-target-architecture.md` into user-facing reference, marking stability (stable/experimental) per row.
4. CLI reference generation script (`hack/gen-cli-docs.py` capturing `--help` recursively) wired into `docs.yml`.
5. Dashboard JSON + provisioning template + screenshots for the quickstart.
6. Quickstart CI job: a workflow (or e2e Ginkgo wrapper) that executes the quickstart's commands verbatim on Kind — fail the build when the quickstart lies.

## Testing

- `docs.yml` build green; link checker (lychee or zensical's own) added to the workflow — broken links become CI failures.
- Quickstart CI job passes from a clean Kind cluster.
- Dashboard JSON lint (grafana dashboard schema check) + manual screenshot review.

## Acceptance criteria

- A newcomer completes the quickstart in <10 minutes with no tribal knowledge; the multi-turn demo and the trace screenshot both work as documented.
- Docs cover: SDK usage, security setup, sessions, tools (both types), observability, runtime contract, CLI, REST API.
- The published site's claims match shipped code exactly (review checklist in the PR template for this unit).

## Out of scope

- Marketing site for flokoa.ai (docs site only). Versioned docs. Video/asciinema (nice-to-have follow-up). Translations.
