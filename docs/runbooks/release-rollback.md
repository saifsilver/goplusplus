# Release and rollback runbook

## Release

1. Confirm a clean tree, reviewed changelog, readiness classifications, and
   migration notes.
2. Run `make release-verify` and inspect dependency, API, benchmark, and security
   reports.
3. Create the annotated semantic-version tag from the verified commit.
4. Publish checksums, SBOM, provenance, and signatures with release artifacts.
5. Roll out gradually and watch the indicators in `docs/slo.md`.

## Rollback

1. Stop rollout and select the preceding verified tag.
2. Check schema and event compatibility before reverting binaries.
3. Prefer a reviewed forward migration when the database cannot safely roll
   back.
4. Restore traffic gradually, validate readiness and critical workflows, and
   watch error budget consumption.
5. Record the reason, affected versions, and follow-up work.
