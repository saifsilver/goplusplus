package tenant_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/tenant"
)

func TestTenantMiddlewareAndScopeQuery(t *testing.T) {
	app := gpp.New()
	app.Use(tenant.Middleware())

	var extractedTenant string
	app.GET("/data", func(c *gpp.Context) error {
		extractedTenant = tenant.GetTenantID(c)
		return c.String(http.StatusOK, "ok")
	})

	// Test 1: Header X-Tenant-ID
	req1 := httptest.NewRequest(http.MethodGet, "/data", nil)
	req1.Header.Set("X-Tenant-ID", "acme_corp")
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)
	if extractedTenant != "acme_corp" {
		t.Errorf("expected tenant 'acme_corp', got '%s'", extractedTenant)
	}

	// Test 2: Subdomain host
	req2 := httptest.NewRequest(http.MethodGet, "/data", nil)
	req2.Host = "stark.app.com"
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)
	if extractedTenant != "stark" {
		t.Errorf("expected tenant 'stark', got '%s'", extractedTenant)
	}

	// Test 3: Default fallback
	req3 := httptest.NewRequest(http.MethodGet, "/data", nil)
	w3 := httptest.NewRecorder()
	app.ServeHTTP(w3, req3)
	if extractedTenant != "default" {
		t.Errorf("expected default tenant 'default', got '%s'", extractedTenant)
	}

	scopedSQL := tenant.ScopeQuery("SELECT * FROM users", "acme_corp")
	if scopedSQL != "SELECT * FROM users /* tenant:acme_corp */" {
		t.Errorf("unexpected scoped query: %s", scopedSQL)
	}
}
