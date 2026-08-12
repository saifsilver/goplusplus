# ADR 0003: Error propagation and public problems

- Status: Accepted
- Date: 2026-08-12

## Decision

Errors affecting correctness propagate to callers with causal wrapping.
Expected client failures use stable RFC 7807 responses. Unexpected causes are
logged once with correlation data and are not exposed to clients. Cleanup
failures are joined with the primary error when they affect consistency.

## Consequences

Logging an error and returning success is prohibited. Tests cover failure paths
for counts, persistence, compensation, cancellation, and adapter startup.
