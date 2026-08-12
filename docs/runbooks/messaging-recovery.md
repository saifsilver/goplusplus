# Messaging and outbox recovery

Monitor producer errors, consumer lag, retry rate, outbox age, and dead-letter
depth. Consumers must tolerate duplicate and reordered delivery where their
provider contract permits it.

For a stalled pipeline:

1. Stop destructive retries and preserve the failed message and correlation ID.
2. Verify broker connectivity, credentials, quotas, partition ownership, and
   downstream health.
3. Restore the dependency before increasing concurrency.
4. Replay from a bounded checkpoint with idempotency enabled.
5. Confirm outbox drain, consumer lag recovery, and domain reconciliation.

Never discard or manually mark a message complete solely to clear monitoring.
Record any exceptional data repair and require owner review.
