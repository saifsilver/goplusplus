// Package resilience provides circuit breaking, adaptive concurrency limiting,
// and retry helpers. Callers must set bounded deadlines and ensure retried
// operations are safe or idempotent.
package resilience
