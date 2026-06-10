# flokoa — Product Brief (Pivot v2.1)

**Status**: Approved direction, incorporating codebase-grounded review feedback
**Audience**: Claude Code, for producing an implementation plan against the existing flokoa codebase
**Supersedes**: Brief v2; framework-agnostic positioning; Knative Serving plans; Vue/CopilotKit plans
**Review feedback**: All factual corrections accepted. The six open decisions are resolved in §8 (adopted defaults — Dani may veto, especially Decision 2).

-----

## 1. One-liner & positioning

**flokoa is the open-source agent harness for Kubernetes — what AgentCore is for AWS.** A Kubernetes-native runtime for single agents and swarms of pydantic-ai agents: declarative definitions, packaged capabilities, isolated sessions, event triggers, and an A2A gateway, on your own cluster.

(Use AgentCore as the *comparison*, never as the category name — phrasing must survive AWS renaming it.)

- **Single framework, deep integration**: pydantic-ai core + pydantic-ai-harness exclusively. flokoa builds on upstream primitives — `AgentSpec`, Capabilities, Hooks, sub-agents/Teams — rather than wrapping them.
- **Relationship to Pydantic**: complement. flokoa is the distribution + runtime channel for the harness capability ecosystem; the self-hosted path alongside their likely hosted one.
- **Boring infrastructure unchanged**: AgentSpec, OCI registries, initContainers, RuntimeClass passthrough, Argo Events, OTel, Postgres. Lean controllers.
- **Target buyer**: teams with existing Kubernetes + pydantic-ai adoption + self-host/compliance needs. Platform engineers provision; AI engineers consume.

## 2. Codebase ground truth (plan against this, not the spec set)

- **No flokoa-owned data path exists today.** A2A traffic goes client → agent Service → pods directly. flokoa-server sees only playground calls; the only contextId machinery is `ExtractSessionKey()` in the trigger path. The session router (Pillar 3) is therefore a **new build** — the single biggest in this plan — not an extension. It subsumes the gateway from old spec 11 and is the natural home for endpoint auth (old spec 05).
- **The Postgres sessions layer is design-only** (roadmap specs; storage choice still open). There is no Knative backend code and no Vue/CopilotKit code to delete.
- **Deletions that actually exist in-repo**: google-adk support, the integrations registry, **Marvin/flokoa-managed-task (deletion confirmed)**, and the Argo Workflows execution path (already removed from execution per prior refactor; AgentWorkflow remains template-only).
- The in-repo agenttrigger-spec.md still describes the older Knative Eventing design; the Argo Events migration is current state. Reconcile docs during planning.

## 3. Pillar 1 — Declarative core (P0)

**Agent CRD as a thin AgentSpec wrapper.**

- Embeds a pydantic-ai `AgentSpec` as a mostly-opaque passthrough (insulates CRD schema from upstream 0.x churn). The CRD adds only what AgentSpec structurally cannot express: secret references, in-cluster Service refs as MCP endpoints, Capability CR references with per-agent config, runtime config (image, resources, isolation tier, pools), status/conditions.
- **Generic runner image**: hydrates the resolved spec at startup (projected ConfigMap), installs referenced capability wheelhouses, constructs the agent via `Agent.from_spec()`, serves A2A. Most users never build a container; custom images remain the escape hatch.
- **Killer DX loop**: edit YAML → behavior changes → no CI pipeline, no image rebuild.

**Composition model (the answer to "why not AgentSpec in a ConfigMap").**

The Agent CRD is a **composition root** and the controller is a **compiler**: an Agent references Model, Instruction, AgentTool, and Capability CRs plus an optional inline spec fragment; the controller compiles them into one resolved AgentSpec, validates it, and delivers it to the runner. flokoa doesn't just ship AgentSpec — it makes AgentSpec **composable, referenceable, and fleet-manageable**: one Instruction CR shared by forty agents; rotate a Model CR and the fleet follows; canary a prompt version across a subset of agents.

Three rules keep composability from re-importing the churn problem the passthrough solved:

1. **Churn risk is scoped to where it actually lives.** The Capabilities API itself is **core pydantic-ai (1.x strict policy)** — what churns is individual capability *implementations* (harness 0.x, third-party). Those flow exclusively through versioned, digest-pinned Capability artifacts with per-version config schemas (§4 taxonomy), so their breaking changes never reach an agent uninvited. The leaf CRDs — Model, Instruction, AgentTool — map to core pydantic-ai concepts (model identifiers, ModelSettings, instructions, MCP config) and get **fully typed schemas**: `kubectl explain`, admission validation, printcolumns. The only opacity that remains: capability config blocks (validated by the capability's own published per-version schema) and an `extra` passthrough on model settings for additive provider-specific knobs (`extra_body`, thinking settings) — typed-common-fields-plus-extras, i.e. ordinary K8s API design.
2. **Explicit merge precedence**: referenced CRs compose in declared order; inline Agent fields win scalar conflicts; list fields (instructions, capabilities) append. Deterministic and documented.
3. **Compiled-spec validation via the runtime contract**: each runner release ships the AgentSpec JSON Schema for its pinned pydantic-ai version; the Go controller validates the compiled spec against it (no Python in the control plane — this is about cheap output validation, not churn insurance). Bad compositions surface as `SpecValid=False` conditions, not CrashLooping pods. The resolved spec hash is surfaced in status for drift detection.

**Virtual endpoint identity (Pillar-3 constraint, designed now — P0a deliverable).**
An Agent's published endpoint (`status.url`) is a **flokoa-owned virtual endpoint** from day one. In v1 it may simply resolve to the agent's Service, but the identity is flokoa's, so inserting the session-routing gateway later is not a breaking change for any caller.

**Platform-injected capabilities.**
The operator auto-injects flokoa-owned capabilities into every hydrated spec: session persistence, budget guardrail, telemetry/OTel wiring. One mechanism — the same one users already understand — unifies:

- **budget enforcement**: in-band via the injected guardrail capability (pod-kill as backstop), upgrading budgets from advisory metadata to enforced limits;
- **session persistence**: conversation state survives sandbox reaping only via this capability (see Pillar 3 truth-in-docs note);
- **observability**: instrumentation without user configuration.

**Secret resolution placement.**
`${secret:...}` placeholders are resolved **in the runner at hydration time** from env/mounted Secrets — never operator-side. Secret values must never be rendered into the spec ConfigMap. Admission validates config *shape* with placeholders intact; values resolve at render time.

## 4. Pillar 2 — Capability packaging & registry (P0, the differentiator)

**Runtime contract (keystone document — write first).**

- Each flokoa release pins **one runner version**: Python minor, pydantic-ai core (which includes the stable Capabilities API and the native capabilities — WebSearch, WebFetch, MCP, Thinking, ToolSearch, …), and baseline libs (httpx, starlette, pydantic, opentelemetry). The published lockfile is the platform.
- **pydantic-ai-harness is deliberately NOT in the baseline.** The capability taxonomy:
  - **Core Capabilities API + native capabilities** → runner baseline, pydantic-ai 1.x strict version policy, directly usable in inline AgentSpec fragments without a Capability CR.
  - **Harness capabilities and third-party capabilities** → ship exclusively as versioned, digest-pinned Capability artifacts. Harness's 0.x policy (minors may rename parameters, change defaults, restructure APIs) is absorbed here: a breaking change is a new artifact version with a new config schema; running agents keep their pinned digest and never break uninvited; upgrades are explicit CR edits validated against the new schema at admission. Different harness versions pinned by two Capabilities on one Agent are rejected by dep-conflict detection — correct behavior, not a limitation.
- **One global runner version per flokoa release**, with `runnerVersion` override per-Agent as the escape hatch. This keeps the compatibility matrix one-dimensional; per-Agent free choice would make it combinatorial.
- Support window: ~2 concurrent runner versions. Core pydantic-ai upgrades are absorbed by cutting a new runner version.

**Capability CRD.**

- Fields: name, version, OCI artifact ref (digest-pinned), Python entrypoint (`module:attr`), JSON Schema for configuration, `requires` tuple, signature/provenance metadata.
- **Machine-checked compatibility**: the capability manifest declares `requires: {python: "3.13", pydantic-ai: ">=1.80,<2", flokoa-runner: ">=0.3"}` and pins its own implementation deps (e.g. `pydantic-ai-harness==0.2.1`) inside the wheelhouse; the runner image publishes its tuple; the controller/webhook **refuses incompatible attachments at admission** with a clear message. The matrix is owned with tooling, not docs.
- **Admission-time dependency-conflict detection**: union pinned deps across an Agent's referenced Capabilities + the runtime baseline; reject conflicts pre-deploy with precise errors.

**Schema provenance (for the validation webhook).**

- Schema is stored in the Capability CR so admission stays offline.
- For `flokoa capability import <pypi-package>`: **derive the JSON Schema by introspection** when the capability's constructor/`from_spec` uses typed/pydantic parameters (most will — it's the pydantic ecosystem). Require an author-provided schema otherwise.
- `schema: permissive` is an explicit, **loud** opt-out (surfaced in CR status, CLI output, and UI): without the escape hatch, import friction kills the marketplace; without it being visible, the validation story is a lie.

**Packaging format: pre-built wheelhouses in OCI artifacts.**

- Artifact: capability wheel + pinned closure of deps not in the baseline + manifest (all pins, `requires` tuple, contract version). Multi-arch (amd64/arm64) via OCI manifest lists.
- **Delivery — initContainer is the default, ImageVolume is the optimization**: default path is an initContainer that copies the wheelhouse from the artifact image into an emptyDir (kubelet layer caching applies; works on every cluster shipped this decade). Where the cluster supports ImageVolume (beta, feature-gated, containerd 2.x — patchy managed-cloud availability in mid-2026), the operator uses the read-only OCI mount instead. Mechanism selected per-cluster via feature detection or Helm value. Same digests, same cosign verification, same install path (`pip install --no-index --find-links <dir>`). Treat ImageVolume exactly like Kata: a tier, not the architecture.
- **Explicit boundary**: system-level deps (binaries, apt packages) are out of scope — those use custom agent images. Documented from day one.

**Registry & CLI ("marketplace").**

- OCI registry + search index. CLI: `flokoa capability build` (**builds wheels inside the pinned runner image** so the matrix is satisfied by construction; smoke-tests import; signs), `push`, `search`, `import <pypi-package>`.
- **Seeding is P0b scope**: package all harness capabilities + endorsed community packages (vstorm-co shields/deepagents/subagents/summarization/todo; DougTrajano skills) at launch.
- Publish friction is the pillar's success metric.

## 5. Pillar 3 — Isolated session runtime (P1, the moat)

**The session router — the single biggest new build.**

An A2A-aware **data-plane gateway**: terminates requests, parses the A2A request body to extract `contextId` (not a header — this cannot be a dumb L7 proxy), looks up or claims a sandbox, proxies including SSE streaming, and is HA-critical and latency-critical. It subsumes old spec 11 (gateway) and hosts endpoint auth (old spec 05). The Pillar-1 virtual-endpoint decision (§3) exists so this inserts without breaking callers.

**Isolation tiers** (`spec.runtime.isolation`):

- `shared` (default): pooled runner pods, many sessions — today's model.
- `session`: one sandbox per A2A context, created on first message, parked/reaped on idle TTL. Required for filesystem/shell/code-execution capabilities and per-user state. **The primary win — one session, one pod, one filesystem, one blast radius — is real even at tier 0 sandboxing.**

**Sandbox tiering (honest version):**

- **Tier 0** (truly everywhere): runc + hardened securityContext + seccomp profiles.
- **Tier 1**: gVisor/Kata via `runtimeClassName` passthrough with per-cloud recommendations — GKE: gVisor first-class (GKE Sandbox); EKS: self-managed nodes with runsc; AKS: Kata-based pod sandboxing. flokoa consumes whatever RuntimeClass the cluster provides and surfaces availability via conditions; it never installs or manages runtimes.
- Marketing language: "defense-in-depth isolation via standard Kubernetes RuntimeClasses" — never "secure" — until a threat model is published.

**Warm pools.**

- Per-agent pool size as an Agent field, **default 0**; a generic-runner pool as fallback (cheap to pool, but pays hydration — capability mount + install + spec load, i.e. seconds — at claim time; per-agent pools claim instantly but multiply cost by agent count).
- **Pool lifecycle lives in a controller reconciling a pool CR, not in flokoa-server.** The server routes; controllers own lifecycle — preserve the layering.

**Truth-in-docs requirement (sandbox reaping).**
When a session sandbox is reaped, conversation state survives **only** if persisted through the platform-injected session-persistence capability (or a user memory/checkpoint capability) to a flokoa-provisioned backend. The sandbox filesystem does not survive. Snapshot/restore is explicitly v2+. State this loudly; users will assume otherwise.

**Engineering gate**: pod-churn spike (scheduling latency, CNI IP allocation, etcd pressure at thousands of short-lived session pods) **before** committing to the session tier publicly.

## 6. Swarms & loop engineering (P2 — payoff, not a separate product)

**Swarm-in-a-box, stated explicitly (resolves the prior ambiguity):**

- The unit of scheduling is the **swarm**: one sandbox running a pydantic-ai program using harness sub-agents (and Teams when it ships). Internal coordination is in-process — typed outputs, shared deps, shared local filesystem. **No A2A between swarm members.** This is precisely why the shared-filesystem question dissolves (no RWX PVC, no NFS).
- **A2A is the boundary protocol**: the surface between deployed Agents, the gateway's external interface, and the transport for cross-swarm/cross-agent chaining. Cross-pod A2A composition of distinct Agents remains fully supported — it's the existing model — but it is not the swarm-internal story.

**Upstream schedule risk, named**: harness **Teams is planned, not shipped** (sub-agents exist). Loop engineering's brain has a dependency flokoa doesn't control — acceptable as a P2 payoff, fatal as a v1 headline. This confirms the sequencing.

**SwarmRun CRD** (P2): high-level requirement (prompt + workspace source, e.g. git repo+branch), swarm template, budgets (enforced via the injected guardrail capability), termination criteria, result delivery via A2A push notifications (existing machinery). flokoa ships a reference orchestrator built on harness sub-agents + verification-loop + planning capabilities; orchestration intelligence stays upstream. AgentTrigger fires SwarmRuns from events.

## 7. CRD surface after the pivot — composable, not collapsed

Decision: the leaf CRDs are **kept and repositioned** as composable AgentSpec building blocks (see §3 composition model), not deprecated into a monolithic spec.

| CRD | Fate |
|-------------|-------------------------------------------------------------------------------|
| Agent | Refactored: **composition root** — inline AgentSpec fragment + CR references + cluster glue + runtime config (§3) |
| Capability | **New** (§4) |
| Model | **Kept**: named, shareable model config (model string + **typed ModelSettings** with an `extra` passthrough for provider-specific knobs + ModelProvider ref) → compiles to AgentSpec `model`/`model_settings`. Fleet-wide model rotation with one edit. |
| Instruction | **Kept**: versioned, shareable instruction blocks → compile (ordered) into AgentSpec `instructions`. Enables prompt reuse, versioning, canary rollout across agents. |
| AgentTool | **Kept, repositioned**: a declarative MCP endpoint — in-cluster Service ref or URL + auth Secret ref — compiling to an `MCP` capability entry. Expresses what raw AgentSpec cannot: cluster-resource references. |
| ModelProvider | Survives as the secret/endpoint projection mechanism behind Model |
| AgentTrigger | Survives, gains importance (Argo Events; fires Agents and later SwarmRuns) |
| AgentWorkflow | **Kept frozen** as the existing template-only resource, repositioned as static A2A composition *between deployed Agents* (distinct from SwarmRun's dynamic in-sandbox swarms). No new features until SwarmRun ships, then reassess overlap. |
| SwarmRun | **New, P2** (§6) |
| Pool CR | **New, P1** — warm-pool lifecycle (§5) |

Also removed: framework-agnostic SDK abstractions, google-adk support, integrations registry, **Marvin/flokoa-managed-task (confirmed)**, RWX-PVC filesystem plans, Vue/Nuxt SDK ambitions (no code exists; remove from roadmap docs only).

The Python SDK narrows to: runner bootstrap (compiled-spec hydration, capability install, secret resolution), A2A serving, the existing direct/event invocation-mode switching, platform-capability implementations, and flokoa context helpers.

## 8. Decisions resolved (adopted defaults — veto window open)

1. **Gateway on the critical path**: ACCEPTED. The session router is a new build; the virtual-endpoint identity constraint is a **P0a deliverable** (§3).
2. **CRD pruning**: OVERRULED by Dani in favor of **composability** — leaf CRDs (Model, Instruction, AgentTool) are kept as composable AgentSpec building blocks with **fully typed schemas** (they map to core pydantic-ai 1.x concepts under the strict version policy, not harness 0.x surface); AgentWorkflow kept frozen as template-only static A2A composition; only Marvin/flokoa-managed-task, google-adk, and the integrations registry are deleted. Harness churn is isolated in the Capability CRD by construction.
3. **Swarm-in-a-box** as the v1 swarm model, A2A as boundary protocol: CONFIRMED (§6).
4. **InitContainer-default / ImageVolume-optimization** delivery: ACCEPTED (§4).
5. **Runner pinning**: one runner version per flokoa release, per-Agent `runnerVersion` override: ACCEPTED (§4).
6. **Sequencing amendment**: ACCEPTED — Phase 0 (release engineering: published images, Helm chart, README) precedes Pillar 1 publicly; old specs 05 (auth) and 11 (gateway) thread into Pillar 3's router, 06 (control-plane hardening) and 08 (observability) thread through all pillars rather than disappearing.

## 9. Phasing

| Phase | Scope |
|-----------|-------------|
| **Phase 0** | Release engineering: published images, Helm chart, README/docs baseline. In-repo deletions: google-adk, integrations registry, **Marvin/flokoa-managed-task (confirmed)**. |
| **P0a** | Runtime contract document + first runner release (contract ships the pinned AgentSpec JSON Schema). Agent CRD refactor as **composition root** + spec compiler in the controller (merge semantics, `SpecValid` condition, resolved-spec hash). Leaf CRD repositioning per §7 with **fully typed schemas for core pydantic-ai fields** (opacity only for capability config + model-settings `extra`). Secret/Service refs, **virtual endpoint identity**. Generic runner bootstrap + runner-side secret resolution. Platform-injected capabilities mechanism (telemetry first). |
| **P0b** | Capability CRD + admission webhook (schema validation, `requires` check, dep-conflict detection). OCI wheelhouse format, initContainer delivery + ImageVolume fast path. CLI: build/push/import/search with introspection-derived schemas + permissive escape hatch. Seed registry. |
| **P1** | **Pod-churn spike (gate).** Session router (data-plane gateway, SSE-capable) behind the virtual endpoint. Sessions Postgres layer (first real implementation — storage decision closes here). Isolation tiers 0/1, pool CR + pool controller, TTL lifecycle, injected session-persistence + budget-guardrail capabilities. Auth (old 05) lands in the router. |
| **P2** | SwarmRun CRD + reference orchestrator (gated on harness sub-agents maturity; Teams when shipped). AgentTrigger→SwarmRun wiring. |

## 10. Non-goals

- Hosting/managing LLM providers or a model gateway (Pydantic AI Gateway exists).
- Owning multi-agent orchestration logic (upstream capabilities own it).
- Standalone sandbox-as-a-service competing with E2B/Modal — isolation serves flokoa agents only.
- Frameworks other than pydantic-ai.
- "Secure" claims before a published threat model.

## 11. Risks the plan must design around

- **The session router is HA- and latency-critical** — it is the one component where flokoa sits in the data path; design for horizontal scaling and graceful degradation (shared-tier traffic must not depend on it in v1).
- **K8s pod churn at session scale** — gated on the P1 spike.
- **Upstream churn** — scoped precisely: the Capabilities API and native capabilities are core pydantic-ai (1.x, baseline); harness/third-party capability *implementations* (0.x) ship only as digest-pinned artifacts with per-version schemas, so their breaking changes are opt-in upgrades caught at admission, never runtime surprises.
- **Harness Teams not shipped** — P2 dependency risk; sub-agents-only reference orchestrator is the fallback.
- **ImageVolume/gVisor availability** — both are tiers with feature detection, never assumptions.
- **Marketplace supply** — seeding is in-scope P0b work.
- **State-loss surprise on sandbox reap** — mitigated by injected session persistence + loud documentation.
