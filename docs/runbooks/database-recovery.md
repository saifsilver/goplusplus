# Database and migration recovery

Before migration, verify the migration checksum history, backup/restore health,
replica lag, lock impact, expected duration, and rollback or forward-fix plan.
Never edit an already-applied migration.

If startup migration fails:

1. Stop further deploys and keep incompatible application instances out of
   service.
2. Capture the failing migration ID and sanitized database error.
3. Determine whether the transaction rolled back completely.
4. Restore only when integrity cannot be recovered through a reviewed forward
   migration; rehearse restoration before touching production.
5. Re-run readiness, constraint, reconciliation, and application smoke checks.

Constraint, timeout, saturation, and replica-routing failures must remain
visible to the caller and telemetry. Do not convert them into empty results.
