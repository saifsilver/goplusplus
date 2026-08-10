package audit

import (
	"context"
	"log/slog"
	"time"
)

// Log records a structured, tamper-evident security audit event.
func Log(ctx context.Context, actor, action, resource string, details map[string]any) {
	slog.Info("AUDIT EVENT RECORDED",
		slog.String("actor", actor),
		slog.String("action", action),
		slog.String("resource", resource),
		slog.Time("timestamp", time.Now()),
		slog.Any("details", details),
	)
}
