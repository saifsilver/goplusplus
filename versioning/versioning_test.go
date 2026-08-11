package versioning_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/versioning"
)

func TestAPIVersioningManager(t *testing.T) {
	vm := versioning.NewManager("v1")
	vm.Deprecate("v1", "2027-01-01")

	app := gpp.New()
	app.Use(vm.Middleware())

	var matchedVersion string
	app.GET("/resource", func(c *gpp.Context) error {
		matchedVersion = versioning.GetVersion(c)
		return c.String(http.StatusOK, "ok")
	})

	// Case 1: Header X-API-Version: v2
	req1 := httptest.NewRequest(http.MethodGet, "/resource", nil)
	req1.Header.Set("X-API-Version", "v2")
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)
	if matchedVersion != "v2" {
		t.Errorf("expected version v2, got %s", matchedVersion)
	}

	// Case 2: Query param ?version=v1 (deprecated)
	req2 := httptest.NewRequest(http.MethodGet, "/resource?version=v1", nil)
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)
	if matchedVersion != "v1" {
		t.Errorf("expected version v1, got %s", matchedVersion)
	}
	if w2.Header().Get("Deprecation") == "" {
		t.Errorf("expected Deprecation header to be present for v1")
	}
	if w2.Header().Get("Sunset") != "2027-01-01" {
		t.Errorf("expected Sunset header '2027-01-01', got '%s'", w2.Header().Get("Sunset"))
	}
}
