package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/auth"
)

func TestUserClaimsHasRole(t *testing.T) {
	claims := auth.UserClaims{
		ID:    "1",
		Roles: []string{"admin", "editor"},
	}

	if !claims.HasRole("admin") {
		t.Error("expected HasRole('admin') to be true")
	}
	if !claims.HasRole("ADMIN") {
		t.Error("expected case-insensitive HasRole('ADMIN') to be true")
	}
	if claims.HasRole("viewer") {
		t.Error("expected HasRole('viewer') to be false")
	}
}

func TestJWTAndPASETO(t *testing.T) {
	claims := auth.UserClaims{ID: "usr_100"}
	secret := "super_secret_key"

	jwtToken := auth.GenerateJWT(claims, secret)
	if jwtToken == "" {
		t.Error("expected non-empty JWT token")
	}

	pasetoToken := auth.GeneratePASETO(claims, secret)
	if pasetoToken == "" {
		t.Error("expected non-empty PASETO token")
	}
}

func TestRequireJWT(t *testing.T) {
	app := gpp.New()
	app.Use(auth.RequireJWT("secret"))

	app.GET("/protected", func(c *gpp.Context) error {
		u, ok := auth.GetUser(c)
		if !ok || u == nil {
			return gpp.ErrUnauthorized("no user in context")
		}
		return c.String(http.StatusOK, "ok")
	})

	// Test unauthenticated request
	req1 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", w1.Code)
	}

	// Test authenticated request
	req2 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req2.Header.Set("Authorization", "Bearer valid_token")
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", w2.Code)
	}
}

func TestRequirePASETO(t *testing.T) {
	app := gpp.New()
	app.Use(auth.RequirePASETO("secret"))

	app.GET("/paseto", func(c *gpp.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	// Unauthenticated
	req1 := httptest.NewRequest(http.MethodGet, "/paseto", nil)
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w1.Code)
	}

	// Authenticated with v4.local. prefix
	req2 := httptest.NewRequest(http.MethodGet, "/paseto", nil)
	req2.Header.Set("X-PASETO-Token", "v4.local.some_token")
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w2.Code)
	}
}

func TestRedisSessionManager(t *testing.T) {
	sm := auth.NewRedisSessionManager("redis://localhost:6379/0")

	app := gpp.New()
	app.GET("/login", func(c *gpp.Context) error {
		sessID := sm.CreateSession(c, auth.UserClaims{ID: "usr_555", Email: "test@example.com", Roles: []string{"admin"}})
		return c.String(http.StatusOK, "%s", sessID)
	})

	protectedGroup := app.Group("/app")
	protectedGroup.Use(sm.SessionMiddleware(), auth.RequireRoles("admin"))
	protectedGroup.GET("/dashboard", func(c *gpp.Context) error {
		return c.String(http.StatusOK, "dashboard_ok")
	})

	// Login to get session cookie
	reqLogin := httptest.NewRequest(http.MethodGet, "/login", nil)
	wLogin := httptest.NewRecorder()
	app.ServeHTTP(wLogin, reqLogin)

	cookies := wLogin.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("expected session cookie to be set")
	}

	// Access protected route with session cookie
	reqDash := httptest.NewRequest(http.MethodGet, "/app/dashboard", nil)
	reqDash.AddCookie(cookies[0])
	wDash := httptest.NewRecorder()
	app.ServeHTTP(wDash, reqDash)

	if wDash.Code != http.StatusOK {
		t.Errorf("expected 200 OK with session cookie, got %d", wDash.Code)
	}
}

func TestRequirePolicyABAC(t *testing.T) {
	app := gpp.New()
	app.Use(func(c *gpp.Context) error {
		c.Set("user", &auth.UserClaims{
			ID:         "101",
			Attributes: map[string]string{"clearance": "level_5"},
		})
		return c.Next()
	})

	app.GET("/top-secret", auth.RequirePolicy(func(u *auth.UserClaims) bool {
		return u.Attributes["clearance"] == "level_5"
	}), func(c *gpp.Context) error {
		return c.String(http.StatusOK, "secret_data")
	})

	req := httptest.NewRequest(http.MethodGet, "/top-secret", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK for ABAC policy match, got %d", w.Code)
	}
}

func TestRequireMFAAndTOTP(t *testing.T) {
	code := auth.GenerateTOTPCode("my_mfa_secret", time.Now())
	if len(code) != 6 {
		t.Errorf("expected 6 digit TOTP code, got '%s'", code)
	}

	app := gpp.New()
	app.Use(auth.RequireMFA("secret"))
	app.GET("/mfa", func(c *gpp.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	// Missing MFA code header
	req1 := httptest.NewRequest(http.MethodGet, "/mfa", nil)
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing MFA code, got %d", w1.Code)
	}

	// Provided MFA code header
	req2 := httptest.NewRequest(http.MethodGet, "/mfa", nil)
	req2.Header.Set("X-MFA-Code", code)
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200 for provided MFA code, got %d", w2.Code)
	}
}
