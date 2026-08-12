# ADR 0001: Layering and dependency direction

- Status: Accepted
- Date: 2026-08-12

## Decision

Dependencies point from applications to optional adapters to stable framework
contracts and the HTTP kernel. The root package does not import provider
implementations. Cross-domain access uses interfaces or versioned events.

## Consequences

Provider replacement and modular-monolith extraction remain possible without
moving domain rules into handlers. CI will reject dependency cycles and future
architecture checks may enforce forbidden imports.
