# Capability readiness

Readiness labels describe support and operational expectations, not code
quality alone.

- **GA:** documented public contract, compatibility commitment, production
  failure behavior, and required automated tests.
- **Beta:** usable with explicit review; contracts may change with migration
  guidance and operational coverage may be incomplete.
- **Experimental:** evaluation only; no compatibility or production support.
- **Development-only:** local testing, scaffolding, or examples; never a
  production correctness dependency.

| Package or capability | Readiness | Production boundary |
| --- | --- | --- |
| Root HTTP engine, routing, binding, validation, errors | GA | Applications own endpoint authorization and deployment policy. |
| `auth` token manager, password policy, verified identity | GA | Applications own accounts, identity providers, recovery, lockout, and secret storage. |
| `config.Loader` | GA | Applications own secret retrieval and rotation. Legacy global helpers are compatibility APIs. |
| `dbcore` PostgreSQL/SQLite, migrations, error classification | GA | Applications own schemas, SQL review, backups, and online migration planning. |
| `gpptest`, `health`, `id`, `versioning` | GA | Health checks must remain bounded and redact dependency errors. |
| Core `middleware` | GA | In-memory rate limiting, idempotency, and singleflight are single-process only. Redis idempotency is Beta. |
| `cache` memory store | GA | Process-local and non-durable. Redis store is Beta pending sustained failure testing. |
| `queue` Kafka adapter and transactional outbox | Beta | Requires broker/database operations, replay policy, and monitoring. |
| `pubsub` Redis/RabbitMQ adapters | Beta | Delivery semantics depend on the provider and consumer idempotency. |
| `notify`, `search`, `storage`, `tracing` adapters | Beta | Provider-specific limits, credentials, retention, and regional controls remain application-owned. |
| `i18n`, `tenant`, `realtime`, `resilience`, `di`, `features`, `bloom` | Beta | Validate application-specific correctness, capacity, and multi-instance behavior. |
| `audit.Log` | Experimental | Emits structured events only; durability and tamper evidence require an external audit pipeline. |
| `saga.Coordinator` | Experimental | Process-local compensation only; not a durable distributed workflow. |
| `dbcore` ephemeral pagination | Experimental | Process-local registry and per-session tables; bounded to 10,000 active sessions. |
| `queue.TaskTracker` and in-memory `pubsub.Bus` | Development-only | State and messages are lost on restart and do not provide backpressure. |
| `dbcore/seed`, CLI generators, examples | Development-only | Generated output requires review before production use. |

Changes to this table require the tests, documentation, and operational evidence
defined in `docs/architecture.md`. A component cannot be promoted through
marketing copy alone.
