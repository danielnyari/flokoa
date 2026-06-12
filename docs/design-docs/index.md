# Design Docs

The system of record for flokoa's design decisions and operating principles. Start here, then
follow the links.

- **[Core Beliefs](core-beliefs.md)** — the golden principles for working in this codebase, plus
  the registry of mechanically-enforced invariants.
- **[ADR-001: Salvage of the 2192bdbd deletions](adr-001-salvage-2192bdbd.md)** — what the v2.1
  pivot deletion retired for good, and what was salvaged into `flokoa-common` and
  `flokoa-openapi`.
- **[Decision: A2A serving stack](../decisions/a2a-stack.md)** — a2a-sdk vs FastA2A/`to_a2a()`
  for the generic runner (tracked by roadmap unit 05; open).
- **[Architecture Overview](../architecture.md)** — how the components fit together (CRDs,
  lifecycle, networking, security).
- **[Runtime Contract](../reference/runtime-contract.md)** — the normative operator↔runner
  interface (compiled spec, secret projection, skew detection, platform capabilities).
- **[Operator Conventions & Architecture](../reference/operator-conventions.md)** — the layered
  architecture, code conventions, testing tiers, and provider implementations.

The longer-form, sequenced implementation plans live in [`../roadmap/`](../roadmap/).
