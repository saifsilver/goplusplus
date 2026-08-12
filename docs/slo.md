# Service-level objectives and indicators

GoPlusPlus is a library, so downstream applications choose numerical objectives.
GA framework components must expose enough information to implement these
minimum indicators without high-cardinality labels.

| Concern | Required indicator | Example application objective |
| --- | --- | --- |
| HTTP availability | successful requests / eligible requests | 99.9% per 30 days |
| HTTP latency | p50, p95, p99 by normalized route | p99 below 500 ms |
| Database | query latency, errors, pool saturation | less than 1% errors; no sustained saturation |
| Queue/outbox | publish errors, lag, retries, DLQ depth | 99% accepted within 60 seconds |
| Idempotency | claims, conflicts, store failures, replays | no duplicate committed operation |
| Authentication | generic failures by route/category | alert on unexpected rate change, never label tokens/users |
| Readiness | dependency state and check duration | bounded checks complete before probe timeout |

Every production deployment must define alert thresholds, an error-budget owner,
measurement windows, exclusions, and the response runbook. Framework benchmarks
measure regression, not an application SLO.
