# GoPlusPlus v1.12 production infrastructure migration

Version 1.12 replaces the database placeholders with real SQLite and
PostgreSQL connections and adds explicit production request/configuration
interfaces. Authentication is unchanged.

## Database adoption

SQLite applications should replace the legacy constructor with explicit,
context-aware opening and closing:

```go
database, err := dbcore.OpenSQLite(ctx, dbcore.SQLiteConfig{
	Path:               "data/application.db",
	BusyTimeout:        5 * time.Second,
	MaxOpenConnections: 4,
})
if err != nil { return err }
defer database.Close()
```

The adapter creates missing directories with mode `0750`, enables foreign keys
and a bounded busy timeout on every pooled connection, and never falls back to
another database. Use `InMemory: true` explicitly for tests. `NewSQLiteClient`
remains available with the deterministic `goplusplus.db` default.

PostgreSQL applications can keep `NewClient`, but it now opens the configured
DSNs instead of an internal fake database. Prefer the explicit name:

```go
database, err := dbcore.NewPostgresClient(ctx, dbcore.Config{
	RWDSN:                 os.Getenv("DATABASE_RW_DSN"),
	RODSN:                 os.Getenv("DATABASE_RO_DSN"), // optional
	MaxOpenConnections:    20,
	MaxIdleConnections:    5,
	ConnectionMaxLifetime: 30 * time.Minute,
	PingTimeout:           5 * time.Second,
})
```

An empty or malformed PostgreSQL DSN is now an error. When `RODSN` is omitted,
reads use the primary. A replica initialization failure closes the primary.
`DB()` exposes the primary `*sql.DB`; framework ORM adoption is not required.

Database failures can be mapped without driver imports using
`dbcore.IsErrorKind(err, dbcore.ErrorUniqueConstraint)` and the other stable
constraint, busy, cancellation, and unknown categories. The original error is
available through `errors.Is`/`errors.As` but must not be returned to clients.

## Migrations

`AutoMigrate` and `MigrateEmbed` accept either adapter through the
`MigrationDatabase` interface. Migrations are validated, sorted by version,
applied transactionally, serialized at startup, and recorded with SHA-256
checksums. Duplicate IDs/versions, empty migrations, failed SQL, and changes to
already-applied SQL fail startup. Applications continue to own all schema SQL.

## Request and error adoption

- Return `gpp.NewInternalError("operation.name", err)` for unexpected failures.
  Causes are logged once and 5xx responses are sanitized with the request ID.
- Use `c.BindNormalizeAndValidate(&request, normalize)` when application values
  must be normalized between strict JSON decoding and validation.
- Use `ParamInt64Strict`, `ParamPositiveInt64`, or `QueryInt64Strict` when zero,
  missing, malformed, and overflowed values must remain distinguishable.
- Parse offset pagination with an immutable `gpp.PaginationPolicy`; the result
  includes an overflow-checked offset. Existing permissive helpers retain their
  fallback behavior.

## Configuration and health

Use `config.NewLoader` for isolated loading. Environment map/OS values override
optional `.env` values, the loader never mutates the process environment, and
normalization runs before application validation. Legacy package-level helpers
remain for compatibility.

Register dependencies with `health.RegisterReadinessCheck` to reject duplicate
names. Checks run from a lock-free snapshot with a bounded timeout; panics and
raw dependency errors are logged internally and public output contains only
`UP`/`DOWN`. `health.SQLReadiness` adapts a context-aware SQL pinger.

## Tests and frontend transport

`gpptest.Decode[T]`, `AssertProblem`, `AssertViolation`, `AssertHeader`,
`AssertContentType`, `AssertRequestID`, and `AssertJSONPath` remove repeated
response decoding from application tests.

`typescript/transport.ts` provides injectable fetch, Problem Details parsing,
timeouts, cancellation, request IDs, non-JSON error handling, and an
application-owned credential supplier. It never reads browser storage, assumes
bearer authentication, logs credentials, or retries writes. Domain client
generation is deferred because route metadata lacks request/response schemas.
