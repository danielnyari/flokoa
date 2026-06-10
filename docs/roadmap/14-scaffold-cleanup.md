# 14 — Scaffold Package Cleanup

**Phase:** any (recommend alongside Phase 0) · **Size:** S · **Depends on:** — · **Enables:** honest maturity signaling; unblocks 02's "everything we ship is real" rule

## Goal

Half-built packages in a public repo undercut the harness claim more than their absence would. Decide, then execute: each workspace member is either **supported** (tested, versioned, released) or **explicitly experimental** (marked, excluded from release artifacts). No silent middle state.

## Current state (per-package)

| Package | Reality | Problems |
|---|---|---|
| `flokoa-managed-task` | Functionally complete Marvin task runtime (all 5 task types: run/classify/extract/cast/generate; entrypoint `python -m flokoa_managed_task`; writes `/tmp/result` + `/tmp/artifact` for the Argo container contract) | **Zero tests**; image never published (RELEASE_REVIEW §7.3 — the `AgentTask` workflow type is broken in real clusters); README calls it a scaffold, contradicting the code |
| `flokoa-codemode-mcp` | ~70%: `CodemodeServer` (FastMCP) exposing OpenAPI ops as Python stubs + `execute_code` in a `pydantic-monty` sandbox | No `__main__` entrypoint, zero tests, not integrated with anything |
| `flokoa-common` | Real shared code: auth scheme types (`ExtendedOAuth2`, `OpenIdConnectWithConfig`), OpenAPI parsing (`OpenApiSpecParser`, `OperationParser`), text utils — depended on by `flokoa` | **Zero tests** for load-bearing shared code |
| (context) google-adk integration | Shipped extra | Overmocked tests flagged HIGH in `OVERMOCKING_REVIEW.md` (P1 #6) — tests assert mock wiring, not behavior |

## Decisions (recommendations — confirm with maintainer, then execute)

1. **`flokoa-managed-task`: promote to supported.** The `AgentTask` workflow type is a real differentiator (Marvin-powered ephemeral tasks in DAGs) and the code is done. Actions:
   - Test suite: unit tests per task type against Marvin with a stubbed model (mirror the managed-agent test approach: `tests/test_agent_executor.py`-style fixtures; cover the `/tmp/result` + `/tmp/artifact` contract explicitly — that file contract is what the Argo compiler depends on).
   - Image: Dockerfile + Makefile targets + release wiring (executed in 02; this unit writes the tests that make shipping it defensible).
   - README rewrite: it is not a scaffold.
2. **`flokoa-codemode-mcp`: park explicitly.** It's a promising 13.4-option-2 seed (sandboxed code execution) but not on the Phase 1 path. Actions: move under `sdk/python/experimental/` (or add `Development Status :: 3 - Alpha` + a loud README banner if moving breaks uv workspace ergonomics — prefer the banner, cheaper), exclude from any wheel publishing in 02, open a tracking issue linking it to the 13.4 RFC so the intent is recorded. **Do not delete** — the monty-sandbox work is the hardest part of a future code-interpreter tool.
3. **`flokoa-common`: backfill tests.** Target the two riskiest surfaces: `OpenApiSpecParser`/`OperationParser` (drives every openapi tool — parse fixtures from `docs/examples` specs + a hostile/edge-case spec set) and auth scheme models (serialization round-trips). Aim ~80% on parser modules, matching the SDK's 79% norm.
4. **google-adk tests: de-mock.** Replace mock-wiring assertions with behavior tests through the executor's public `execute(RequestContext, EventQueue)` contract (the managed-agent tests show the pattern). If genuine behavior coverage isn't feasible without heavy ADK scaffolding, shrink the suite to honest smoke tests and say so — fewer true tests beat many false ones. Closes `OVERMOCKING_REVIEW.md`'s HIGH item.

## Implementation plan

1. PR 1: managed-task tests + README + (with 02) image targets.
2. PR 2: codemode-mcp banner/move + publish-exclusion + tracking issue; delete its dead `__main__` stub or add a real one — pick one.
3. PR 3: flokoa-common test backfill.
4. PR 4: google-adk test rewrite per `OVERMOCKING_REVIEW.md`'s specific findings.
5. Workspace hygiene sweep: `uv sync --all-packages --all-extras && pytest` green from `sdk/python/` root (fixes RELEASE_REVIEW P1 #4 "workspace-root pytest broken" — diagnose while in here: likely testpaths/rootdir config in the workspace `pyproject.toml`).

## Acceptance criteria

- Zero workspace packages with zero tests (or an explicit experimental banner exempting them).
- `RELEASE_REVIEW.md` P1 items #4, #5, #6, #7 closed with evidence.
- A reader of any package README knows its support status in the first paragraph.

## Out of scope

- New features in any of these packages. The `flokoa-cli`-image naming question (02 records the decision). Operator-side Go test backfill (tracked separately; P1 #5's Go half).
