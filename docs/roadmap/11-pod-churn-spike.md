# 11 — Pod-Churn Spike (P1 Gate)

**Phase:** P1, first · **Size:** M (timeboxed: ~1–2 weeks) · **Depends on:** — (can run during P0b) · **Gates:** 12/14 public commitment to the session tier

## Question to answer

Can Kubernetes sustain **pod-per-session** at the scale flokoa intends to claim — and at what cost? Brief §5 makes this an explicit gate: scheduling latency, CNI IP allocation/exhaustion, kubelet/etcd pressure at thousands of short-lived session pods, before the session tier is committed publicly.

## Method

1. **Workload simulator**: a load generator creating/destroying "session" pods (the real runner image with a stub spec, plus warm-pool claim simulation via label patch) with arrival/lifetime distributions modeling chat traffic (Poisson arrivals; lifetimes: 1m / 10m / parked-1h mixes). Parameters: concurrent sessions ∈ {100, 500, 2k, 5k}, arrival rates ∈ {1, 10, 50/s}.
2. **Environments**: Kind (smoke only — numbers don't transfer), then at least one real managed cluster (GKE or EKS, default CNI + one alternative if IPs become the constraint, e.g. EKS prefix delegation).
3. **Measurements**: p50/p95/p99 first-message-ready latency (pod create → A2A ready), cold vs warm-pool claim; scheduler throughput; IP allocation failures; etcd/apiserver metrics under churn (object counts, watch fan-out — note the operator's own watch mappers list across namespaces); node density limits; cost per 1k session-hours at each tier (runc vs gVisor where available — sandbox runtimes change density).
4. **Failure probes**: node drain mid-session, image-pull storms (capability initContainers × N pods), pool refill behavior under burst.

## Deliverables

- `docs/roadmap/11a-spike-report.md`: numbers, limits found, and a **recommendation matrix** — max advertised concurrent sessions per cluster size, required warm-pool sizing math (pool size vs claim-latency SLO vs cost), CNI guidance, and the explicit go/no-go for the session tier's public positioning.
- Reusable load-test harness committed under `test/load/` (the P1 features re-run it in CI-lite form as a regression check).
- Any "this changes the design" findings filed against 12/14 *before* their implementation starts (e.g., if pod-per-session caps out too low: per-session **container**-in-shared-pod or sticky-session-to-replica fallbacks get evaluated — decisions, not assumptions).

## Acceptance criteria

- The report answers: ready-latency distribution warm/cold, sustainable churn rate, density/cost per tier — with reproducible scripts.
- 12/14 specs are amended (or confirmed) against the findings; the session tier's public claims cite the measured envelope.
