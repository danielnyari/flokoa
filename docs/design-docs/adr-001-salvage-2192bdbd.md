# ADR-001: Salvage of the 2192bdbd deletions

**Status:** Accepted — 2026-06-12

## Context

Merge commit `2192bdbd` (PR #137, the v2.1 pivot) deleted the pre-pivot Python
runtime wholesale: the unified config system (`flokoa/config/`), the
pydantic-ai integration layer (`flokoa/integrations/pydantic_ai/`), the
OpenAPI toolset machinery (`flokoa/tools/`), and the entire
`flokoa-managed-agent` package. A June 2026 audit re-read every deleted module
against current pydantic-ai documentation and the working tree to decide what
was genuinely obsolete and what was a differentiator deleted by association.

## Decision

**Confirmed retired** — each superseded by a *named* feature, not by hope:

| Deleted | Superseded by |
|---|---|
| Config system (`AgentConfig`, builders, loaders, `CodeRef`) | pydantic-ai `AgentSpec` / `Agent.from_spec` / `Agent.from_file`; Capability CRD entrypoints (P0b) |
| Model factory + `PROVIDER_MODEL_MAP` | `provider:model` strings + `model_settings` (the spec compiler emits both); provider env vars |
| Managed-agent runtime (`flokoa-managed-agent`) | `flokoa-runner` hydration pipeline + `flokoa.serving`; upstream `agent.to_a2a()` for non-K8s users |
| JSONSchema structured-output bootstrap (hand-rolled `StructuredDict`) | native `AgentSpec.output_schema` |

**Salvaged:**

- The **auth/credential-exchange layer** moved into `flokoa-common`
  (`flokoa_common.auth`): credential models, scheme helpers, and async
  exchangers (OAuth2 refresh, Google service accounts behind a `[google]`
  extra, auto-dispatch). Dynamic credential exchange was expressible nowhere
  else in the platform — the one genuine differentiator in the deletion.
- The **hardened REST engine** recombined as the `flokoa-openapi` capability:
  one typed `ToolDefinition` per operation (`parameters_json_schema` plus
  `return_schema` from the 2xx response schema), with `defer_loading` so
  large specs ride core **ToolSearch** for discovery and stack with harness
  **CodeMode** for execution. `Tool.from_schema` is deliberately bypassed —
  it cannot carry `return_schema` or `defer_loading`, which are the point.

**Demoted:** `flokoa-codemode-mcp` keeps the out-of-process credential
isolation niche (the token never enters the agent pod); its silent auth
bypass was fixed by wiring the salvaged exchangers. In-process CodeMode +
ToolSearch supersede it as the default code-mode path.

**Unchanged:** `type: openapi` stays retired on AgentTool; the admission
webhook keeps rejecting it. The Capability CRD (roadmap 08, P0b) is the CRD
surface for `flokoa.OpenAPI` — AgentTool remains a pure MCP endpoint pointer.

## Consequences

- `flokoa` now depends on `flokoa-common`; publishing `flokoa` to PyPI
  requires publishing `flokoa-common` too (release workflow updated).
- `flokoa-openapi` ships as a workspace package today and as a Capability
  artifact once roadmap 08–10 land; it is deliberately not a runner
  dependency.
- An upstream pydantic-ai OpenAPI integration would compete only with the
  thin spec→ToolDefinition layer; the auth/hardening path is the
  least-upstreamable piece. Re-check upstream docs before extending it.
- **Open question** (decide in the P1 session work): upstream FastA2A
  (`agent.to_a2a()`) now persists conversation history per `context_id` with
  pluggable storage — overlapping roadmap 13 (sessions state backend) and the
  reserved session-persistence capability. One of them must own persistence;
  doing both would be two sources of truth.
