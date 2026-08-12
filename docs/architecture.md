# Architecture

## Purpose

GoPlusPlus provides a small HTTP framework core plus optional infrastructure
packages. Applications retain ownership of domain rules, data, deployment,
authorization policy, and operational configuration.

## Dependency direction

```text
applications and examples
        ↓
optional adapters and platform packages
        ↓
framework contracts and middleware
        ↓
root HTTP kernel
```

The root package must not import optional provider packages. Provider packages
may depend on stable root contracts but must not depend on application code.
Domain modules communicate through explicit interfaces or versioned events and
must not write directly to another module's private tables.

## Runtime ownership

- `Engine` owns HTTP routing, request contexts, server lifecycle, and framework
  error rendering.
- Applications own startup ordering, dependency construction, authorization,
  migrations, and graceful closure of injected resources.
- Provider clients own the connections they create. Constructors that wrap an
  application-owned client document whether `Close` transfers ownership.
- Process-local stores are optimization or development facilities, never
  multi-instance correctness dependencies.

## Correctness rules

- External input is bounded and validated before use.
- Cancellation and deadlines cross network and database boundaries.
- Errors affecting persisted state, counts, compensation, or delivery are never
  logged and discarded.
- At-least-once consumers are idempotent. Database writes and event publication
  use an outbox when atomicity is required.
- Unbounded goroutines, maps, queues, retries, payloads, and cardinality are not
  allowed in GA components.
- Public errors are stable and sanitized; causal errors are retained for logs
  and `errors.Is`/`errors.As`.

## Public API policy

Every exported package and declaration has Go documentation covering its
contract. Configurations describe zero values, defaults, limits, ownership,
security-sensitive fields, and failure behavior. Examples are compiled in CI.
Compatibility follows `docs/release-policy.md` and readiness follows
`docs/readiness.md`.

## Observability

Framework signals use structured logs, stable metric names, request or trace
correlation, and bounded attributes. Secrets, credentials, tokens, raw request
bodies, and unclassified SQL arguments must not be emitted. Applications own
sampling, export, retention, alert routing, and sensitive-data controls.

## Decision records

Material changes to dependency direction, compatibility, error contracts,
delivery semantics, or production readiness require an ADR in `docs/adr`.
