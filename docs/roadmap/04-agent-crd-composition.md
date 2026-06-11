# 04 — Agent CRD as Composition Root + Spec Compiler

**Phase:** P0a · **Size:** XL · **Depends on:** 03 · **Co-delivered with:** 05 (must merge with a working end-to-end path) · **Enables:** 06, 07, 08

## Goal

Refactor the Agent CRD into the **composition root** of brief §3 and the agent controller's app layer into a **compiler**: resolve Model/Instruction/AgentTool/Capability references + an inline AgentSpec fragment into one validated AgentSpec, delivered to the runner as a single ConfigMap. Reposition the leaf CRDs with fully typed schemas for core pydantic-ai concepts.

## Current state

- `AgentSpec` (`operator/api/v1alpha1/agent_types.go:126`): `CardOverride`, `Runtime{Type: standard|template, Standard, Template}`, `Model *AgentModelRef`, `Instruction *InstructionEntry` (single, inline-or-ref), `Framework`, `Tools []ToolEntry` (inline-or-ref OpenAPI tools).
- The app layer (`internal/app/agent/reconcile.go`) already has the right shape to become a compiler: `Service` orchestrating `ToolReconciler`/`ModelReconciler`/`InstructionReconciler` over `repo` interfaces (`Deps` struct), producing **multiple** ConfigMaps consumed by the managed agent via the legacy contract (builder constants in `internal/infra/builder/deployment.go`: `template-config.json`, `instruction.txt`, `tools/<name>/spec.json`, `model.json`, `agent-card.json`).
- Status machinery to reuse: `updateStatusWithRetry` (`internal/controller/status.go`), `metav1.Condition` lists, `flokoaerrors.IsPermanent/IsDependency` requeue taxonomy, `hash.JSONStruct` (`internal/domain/hash`).
- Webhooks per CRD in `api/v1alpha1/*_webhook.go` (CustomValidator pattern).
- This is an **alpha API (v1alpha1, 0.x)**: the breaking reshape happens in-place with a documented migration; a parallel v1alpha2 was considered and rejected (conversion-webhook cost without users to protect).

## Target CRD shapes

### Agent

```go
type AgentSpec struct {
    // Spec is an inline pydantic-ai AgentSpec fragment (typed-where-stable, see AgentSpecFragment).
    // +optional
    Spec *AgentSpecFragment `json:"spec,omitempty"`

    // ModelRef references a Model CR. Wins over spec.model unless absent.
    // +optional
    ModelRef *NamespacedRef `json:"modelRef,omitempty"`

    // InstructionRefs compose Instruction CRs, in order, before spec.instructions.
    // +optional
    InstructionRefs []NamespacedRef `json:"instructionRefs,omitempty"`

    // Tools reference AgentTool CRs (declarative MCP endpoints), compiled to MCP capability entries.
    // +optional
    Tools []NamespacedRef `json:"tools,omitempty"`

    // Capabilities reference Capability CRs with per-agent config (validated against the
    // capability's published schema at admission, unit 08).
    // +optional
    Capabilities []CapabilityAttachment `json:"capabilities,omitempty"`

    // SecretRefs names the secrets resolvable via ${secret:NAME} placeholders (contract 03).
    // +optional
    SecretRefs map[string]corev1.SecretKeySelector `json:"secretRefs,omitempty"`

    // Card keeps the existing A2A card override (unchanged).
    Card AgentCardOverride `json:"card"`

    // Runtime: how to run it (replaces RuntimeSpec).
    Runtime AgentRuntime `json:"runtime"`
}

type CapabilityAttachment struct {
    Ref NamespacedRef `json:"ref"`
    // Config is validated against the Capability's published JSON Schema (08).
    // +kubebuilder:pruning:PreserveUnknownFields
    // +optional
    Config *apiextensionsv1.JSON `json:"config,omitempty"`
}

type AgentRuntime struct {
    // Image overrides the generic runner image (escape hatch; custom images own their bootstrap).
    // +optional
    Image string `json:"image,omitempty"`
    // RunnerVersion pins a runner release (default: operator's current). Brief §4 decision 5.
    // +optional
    RunnerVersion string `json:"runnerVersion,omitempty"`
    // Isolation: shared (default) | session (P1; admission-rejected until 12/14 ship).
    // +kubebuilder:default=shared
    Isolation IsolationTier `json:"isolation,omitempty"`
    // Pool, Resources, Replicas, Env, DeploymentOverrides — carry over existing fields.
    ...
}
```

`AgentSpecFragment` mirrors the *stable* AgentSpec fields with typed schemas — `model` (string), `instructions []string`, `modelSettings` (typed common fields + `extra` RawExtension passthrough), `capabilities` (native-capability entries by name+config; **harness/third-party class paths are rejected here** — they must come through Capability CRs, brief §4 taxonomy), `outputType`/structured-output where stable — plus `extra` passthrough for additive upstream fields. The fragment is the one place schema-tracking work recurs per runner bump; keep it minimal and lean on the compiled-spec JSON Schema validation (03) as the backstop.

### Leaf CRDs (repositioned, fully typed)

- **Model**: `model` (string, e.g. `openai:gpt-…`), `providerRef` (ModelProvider, unchanged secrets/endpoint projection), `settings ModelSettings` — typed common fields (`maxTokens`, `temperature`, `topP`, `timeout`, `parallelToolCalls`, …) + `extra *apiextensionsv1.JSON` for provider-specific knobs (`extra_body`, thinking settings). Compiles to AgentSpec `model` + `model_settings`. Audit current `Model`/`ModelConfig` types and converge.
- **Instruction**: stays content-centric (`content`), gains nothing structural — already right. Compiles into `instructions[]` in declared order.
- **AgentTool**: **MCP endpoint only** — `url` | `serviceRef` (+`path`, default `/mcp`), `transport` (streamableHTTP|sse), `headers`, `headerSecrets []SecretHeader`, `toolPrefix`, `allowedTools`, `timeoutSeconds`. Compiles to an MCP capability entry in the spec with `${secret:…}` placeholders for header secrets. The OpenAPI tool type is **retired** (migration note: front REST APIs with an MCP adapter or a capability); webhook rejects `type: openapi` with that pointer.

## The compiler

New `internal/app/agent/compiler/` (the existing sub-reconcilers refactor into resolvers feeding it):

1. **Resolve**: fetch referenced CRs via existing `repo` readers; missing refs → `flokoaerrors` dependency error (30s requeue, condition message naming the ref).
2. **Merge** (brief §3 rule 2, deterministic): start from `spec` fragment → apply `modelRef` (scalar: ref wins only if fragment's `model` absent — *inline scalars win conflicts*) → prepend `instructionRefs` contents in order before fragment `instructions` (lists append) → append AgentTool-compiled MCP entries and Capability attachments to `capabilities`. Precedence is property-tested, not just documented.
3. **Inject** platform capabilities (07) — last, so user entries can't shadow them.
4. **Validate** against the embedded AgentSpec JSON Schema for the resolved runner version (03). Failure → `SpecValid=False` condition with the JSON-pointer error path; **no Deployment update happens on an invalid spec** (last-good keeps running).
5. **Emit**: one ConfigMap `{agent}-agent-spec` (key `agent-spec.yaml`, placeholders intact), `status.specHash = hash.JSONStruct(resolvedSpec)`, `SpecValid=True`. Builder (`internal/infra/builder`) gains the single mount replacing the five legacy mounts; secret placeholders become `FLOKOA_SECRET_*` env per the 03 grammar; capability initContainers come from 09.

Watches: extend `SetupWithManager` mappers (the `findAgentsFor*` pattern) to cover Capability CRs and `secretRefs` secrets, so edits anywhere in the composition graph re-compile — this *is* the fleet-management story (rotate one Model CR, every referencing Agent recompiles, `specHash` changes, rollout follows via the pod-template hash annotation).

## Webhooks

- Agent: exactly-one-of semantics where applicable; `runtime.isolation: session` rejected until P1; fragment `capabilities` entries restricted to baseline-native names; `${secret:NAME}` placeholders must have matching `secretRefs` keys (cheap static check).
- Leaf CRDs: updated for new shapes; AgentTool openapi rejection with migration message.

## Testing

- Compiler unit tests with `repo/fakes`: merge-precedence property tests (ref/inline/list matrices), missing-ref conditions, schema-validation failure keeps last-good Deployment, specHash stability/change detection.
- Golden compiled specs: Agent CR fixtures → expected `agent-spec.yaml`, validated by the real schema; cross-checked in 05's runner tests (same goldens hydrate a real pydantic-ai Agent inside the runner image — the contract test that binds 03/04/05).
- envtest: full reconcile producing ConfigMap + Deployment + status; fleet propagation (edit Instruction → dependent Agent recompiles).
- `make manifests generate` + `make generate-python-models` artifacts committed.

## Acceptance criteria

- An Agent composing 1 Model + 2 Instructions + 1 AgentTool + inline fragment yields a schema-valid AgentSpec ConfigMap with documented precedence; editing any referenced CR recompiles and rolls the Deployment.
- Invalid composition → `SpecValid=False` with a precise path; running pods untouched.
- `kubectl explain agent.spec` / `model.spec.settings` show typed fields; only capability config and `extra` blocks are opaque.

## Out of scope

- Runner consumption (05). Capability CR semantics + admission (08). Migration tooling for existing CRs beyond a documented mapping table (alpha API).
