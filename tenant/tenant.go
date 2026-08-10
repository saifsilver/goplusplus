package tenant

import (
	"fmt"
	"strings"

	"github.com/saifsilver/goplusplus"
)

// Middleware returns multi-tenancy extraction middleware searching X-Tenant-ID header or subdomain.
func Middleware() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			host := c.Request.Host
			parts := strings.Split(host, ".")
			if len(parts) > 2 {
				tenantID = parts[0]
			}
		}
		if tenantID == "" {
			tenantID = "default"
		}
		c.Set("tenant_id", tenantID)
		c.SetHeader("X-Tenant-ID", tenantID)
		return c.Next()
	}
}

// GetTenantID extracts the current tenant ID from context.
func GetTenantID(c *gpp.Context) string {
	if val, ok := c.Get("tenant_id"); ok {
		if id, ok := val.(string); ok {
			return id
		}
	}
	return "default"
}

// ScopeQuery returns a SQL query string scoped to tenant_id column.
func ScopeQuery(query string, tenantID string) string {
	return fmt.Sprintf("%s /* tenant:%s */", query, tenantID)
}
