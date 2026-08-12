# ADR 0004: Compatibility and deprecation

- Status: Accepted
- Date: 2026-08-12

## Decision

GA v1 APIs follow semantic-version source compatibility. Deprecations identify
a replacement and migration path. Unsafe compatibility may fail closed or be
removed with explicit security and migration notes.

## Consequences

CI records the exported API and fails unexpected removals. Beta and
Experimental status must be visible before adoption.
