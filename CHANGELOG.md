# Changelog

Notable user-visible changes are recorded here. Release tags remain the source
of truth for exact source state. Security-sensitive entries may be published
after coordinated disclosure.

## Unreleased

- Record and gate the public API breaks relative to v1.11.5; release these only
  in the next major version unless compatibility shims are added.
- Propagate ORM pagination count failures.
- Fail ephemeral materialization when source reads or row persistence fail.
- Return saga compensation failures instead of reporting successful recovery.
- Clarify process-local and experimental capability boundaries.
- Add governance, architecture, readiness, and operational documentation.
- Expand automated enterprise quality gates.

## v1.11.6

- Enforced fresh sequential coverage generation in the repository quality gate.

For earlier releases, see the repository's annotated Git tags and migration
guides.
