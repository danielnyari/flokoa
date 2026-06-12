# Design Docs

The system of record for flokoa's design decisions and operating principles. Start here, then
follow the links.

- **[Core Beliefs](core-beliefs.md)** — the golden principles for working in this codebase, plus
  the registry of mechanically-enforced invariants.
- **[Architecture Overview](../architecture.md)** — how the components fit together (CRDs,
  lifecycle, networking, security).
- **[Runtime Contract](../reference/runtime-contract.md)** — the normative operator↔runner
  interface (compiled spec, secret projection, skew detection, platform capabilities).
- **[Operator Conventions & Architecture](../reference/operator-conventions.md)** — the layered
  architecture, code conventions, testing tiers, and provider implementations.

The longer-form, sequenced implementation plans live in [`../roadmap/`](../roadmap/).
