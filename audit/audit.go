package audit

import (
	"context"
	"log/slog"
	"time"
)

// Log emits a structured audit event through the configured slog handler.
// Durability, tamper evidence, access control, and retention are responsibilities
// of the application's log pipeline and audit store.
func Log(ctx context.Context, actor, action, resource string, details map[string]any) {
	slog.Info("AUDIT EVENT RECORDED",
		slog.String("actor", actor),
		slog.String("action", action),
		slog.String("resource", resource),
		slog.Time("timestamp", time.Now()),
		slog.Any("details", details),
	)
}
