# Core Beliefs

The operating principles for the flokoa codebase — the "golden principles" an agent or a new
engineer should internalize before changing anything. These are deliberately opinionated. Where
a belief can be enforced mechanically, it is (see the [Enforced-invariants registry](#enforced-invariants-registry)
at the bottom); the rest are conventions we hold by hand until they earn a check.

> Anything an agent can't read in the repository effectively doesn't exist. Decisions that live
> only in chat threads or someone's head aren't part of the system. Encode them here.

## 1. Kubernetes-native

Everything is a CRD or a standard Kubernetes resource. Desired state lives in etcd; there is no
side-channel state store. The seven CRDs (`Agent`, `AgentTool`, `AgentWorkflow`, `AgentTrigger`,
`Model`, `ModelProvider`, `Instruction`) **are** the platform's configuration surface.

## 2. Declarative reconciliation

Controllers converge desired state; they don't perform imperative one-shot mutations. A change
is expressed by editing a resource and letting the reconciler act. No CLI side effects.

## 3. Layered architecture, enforced

The operator has a fixed layering with a mechanically enforced dependency direction:

- `api/v1alpha1` depends on nothing internal (CRD types stay importable standalone).
- `internal/domain` is a **leaf** — pure models and functions, no sibling-layer imports.
- `internal/infra` sits below `app`/`controller`/`server`.

This is enforced by `depguard` (`operator/.golangci.yml`, run by `make lint`). The constraint
is what lets agents move fast without architectural drift — see
[operator-conventions](../reference/operator-conventions.md) for the full table.

## 4. Type-safe pipeline; never hand-edit generated code

Go CRD types are the single source of truth. They flow outward automatically:

```
api/v1alpha1/*_types.go  --(make manifests)-->  config/crd/bases/*.yaml
                         --(make generate)--->  zz_generated.deepcopy.go
   CRD YAML schemas       --(generate-python-models)-->  sdk/python/flokoa-types/src/flokoa_types/*.py
```

Generated files (`zz_generated.*.go`, `flokoa_types/*.py`, CRD YAML) are never edited by hand.
After changing a CRD type, run `make manifests generate generate-python-models`. CI enforces
freshness with `make verify-codegen`, so stale generated artifacts can't merge.

## 5. Parse and validate at boundaries — no YOLO shape-guessing

Don't probe data structurally and hope. Use typed SDKs (controller-runtime, `a2a-go`,
pydantic / pydantic-ai) and validate at the edges. The compiled `AgentSpec` is validated against
the runner's pinned JSON Schema before it's ever delivered to a pod.

## 6. The runtime contract is normative

[`docs/reference/runtime-contract.md`](../reference/runtime-contract.md) governs everything
operator↔runner. Changes to it are PR-blocking review items, and the runner pin
(`sdk/python/flokoa-runner`) is regenerated with `make runner-contract` — guarded in CI by
`verify-runner-contract`.

## 7. Prefer boring, internalizable dependencies

Favor dependencies an agent can fully reason about in-repo: controller-runtime, kubebuilder,
`a2a-go`, pydantic-ai. Composable, API-stable, well-represented libraries beat clever ones.
When a generic library would add opaque behavior for a small benefit, prefer a small,
fully-owned, fully-tested helper.

## 8. Structured logging

Go uses controller-runtime's `logf`/`logr` with structured key/value fields, not `fmt.Print*`.
(The Python SDK currently uses stdlib logging — see the not-yet-enforced list below.)

## Enforced-invariants registry

The mechanical checks that keep these beliefs true. "Promote the rule into code" — when a
convention starts getting violated, add it here.

| Invariant | Mechanism | Where it runs |
|-----------|-----------|---------------|
| Operator layer boundaries (belief 3) | `depguard` rules in `operator/.golangci.yml` | `make lint` → `lint.yml` |
| Published endpoint, never the `-runtime` Service | `make verify-endpoint-refs` (grep gate) | `lint.yml` |
| Runner contract coherence (pin ↔ lock ↔ embedded schema) | `make verify-runner-contract` | `test-python.yml` |
| Generated artifacts are fresh (belief 4) | `make verify-codegen` | `test.yml` (`verify-generated` job) |

### Intended, not yet enforced

Candidates to promote into checks as the codebase grows (do **not** assume they hold today):

- **Python package import boundaries** — `flokoa-types` ← `flokoa` ← `flokoa-runner` dependency
  DAG (would use `import-linter`/`tach`).
- **Tighter Go layering** — e.g. `controller` not importing `infra/builder` directly,
  `app` not importing `controller`. Add as depguard rules once verified against the current graph.
- **Structured logging in the Python SDK** — currently stdlib string logging, not structured.
- **Docs link-check in CI** — cross-link validation for `docs/`.
