# 17 — Cross-Cutting Threads: Observability & Control-Plane Hardening

**Phase:** ongoing (decision 6: these thread through all pillars rather than disappearing) · Successor to old specs 06 (control-plane hardening) and 08 (observability); old 05/11 live in unit 12.

Not a single implementation unit — a checklist binding the phased units, plus two small standalone work items.

## Observability thread

| Where it lands | What | Unit |
|---|---|---|
| Runner | GenAI semconv traces + token metrics via the injected telemetry capability; per-agent `OTEL_SERVICE_NAME` | 07 |
| Router | RED metrics per agent, claim-latency histograms, access logs (no payloads), traceparent continuity | 12 |
| Pools/registry | pool depth/refill/reap metrics, claim outcomes; session counts per agent | 13/14 |
| SwarmRun | usage in status; run-as-trace | 15 |
| **Standalone item A (S, anytime after 07)** | Helm-optional OTel collector + Grafana dashboards shipped in-chart (requests/latency/tokens per agent; router/pool panels added as P1 lands); quickstart shows one invocation as one trace | — |

Binding rule: every new component ships its metrics + spans in the same PR as the component. No "instrumentation later."

## Control-plane hardening thread (old 06, still valid — the pivot doesn't change it)

| What | Notes | When |
|---|---|---|
| TLS for flokoa-server (gRPC + REST/SSE) | cert-manager Certificate via the chart (02's webhook plumbing reuses the issuer) | with/after 02 |
| AuthZ on the API: SubjectAccessReview against cluster RBAC | the old-06 design survives verbatim: method→(group,resource,verb) table, fail-closed on unmapped methods, TTL cache; SSE watch + playground + **trigger invoke endpoint** (now load-bearing for SwarmRuns) get the same `authorize()` helper | **Standalone item B (M)** — before any multi-team production claim; independent of pillars |
| Webhooks installable | done in 02 | 02 |
| NetworkPolicies | server/operator policies in-chart (02); per-sandbox egress policies | 02, 14 |
| Per-namespace capability allow-listing | which namespaces may reference which Capability CRs (admission policy) — needed before "platform team provisions, AI engineers consume" is safe | post-P0b, pre-P1 GA |
| Threat model document | prerequisite for any "secure" language (brief §10); covers capability supply chain (signing, permissive schemas), sandbox tiers, router, secret flow | before session-tier public positioning |

## Sequencing note

Items A and B are deliberately small and independent — schedule them opportunistically between phase boundaries. Everything else in this doc is enforced via PR review against the binding rules above, not scheduled separately.
