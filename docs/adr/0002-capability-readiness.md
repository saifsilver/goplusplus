# ADR 0002: Capability readiness classification

- Status: Accepted
- Date: 2026-08-12

## Decision

Every public capability is GA, Beta, Experimental, or Development-only as
defined in `docs/readiness.md`. Documentation and examples must use the label
and cannot imply stronger durability, scale, security, or compliance.

## Consequences

Promotion requires contract tests, documented failure behavior, operational
signals, and ownership. Process-local helpers cannot be GA substitutes for
distributed infrastructure.
