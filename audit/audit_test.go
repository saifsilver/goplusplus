package audit_test

import (
	"context"
	"testing"

	"github.com/saifsilver/goplusplus/audit"
)

func TestAuditLog(t *testing.T) {
	ctx := context.Background()
	audit.Log(ctx, "usr_101", "DELETE_USER", "users/usr_202", map[string]any{"reason": "gdpr_request"})
}
