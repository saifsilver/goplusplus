# ADR 0005: Events, outbox, and delivery semantics

- Status: Accepted
- Date: 2026-08-12

## Decision

Durable event publication uses a transactional outbox. Delivery is treated as
at least once unless an adapter explicitly documents a stronger contract.
Consumers are idempotent, event schemas are versioned, and dead-letter/replay
operations are observable and bounded.

## Consequences

The in-memory bus is Development-only. Provider adapters require conformance,
crash, duplicate, cancellation, and replay tests before GA promotion.
