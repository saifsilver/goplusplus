# Release and compatibility policy

GoPlusPlus uses semantic versions. A tagged v1 release preserves source
compatibility for documented GA APIs unless a security or correctness defect
requires a breaking change. Such exceptions require migration notes and an
explicit release entry.

Beta APIs may evolve in minor releases with migration guidance. Experimental
and development-only APIs have no compatibility guarantee. Deprecations must
identify the replacement, appear in Go documentation, and remain for at least
one supported minor release unless retaining them would be unsafe.

`make api-compat` compares the current module with v1.11.5. Existing breaking
changes are recorded in `api/compatibility-exceptions-v1.11.5.txt`; any change to
that reviewed exception set requires explicit release-owner approval. Because
the unreleased tree contains those breaks, it must not be published as another
v1 minor or patch release without compatibility shims. The default release path
is the next major version.

Release candidates must pass the required CI workflow, produce a clean module,
publish a changelog entry, and contain no undocumented public API changes.
Release artifacts should include checksums, an SBOM, provenance, and signatures.
Rollback uses the preceding verified tag; database changes require the recovery
procedure in `docs/runbooks/release-rollback.md`.
