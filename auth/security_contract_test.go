package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gpp "github.com/saifsilver/goplusplus"
)

var testSecret = strings.Repeat("k", 32)

func TestArgon2idPasswordContract(t *testing.T) {
	config := PasswordConfig{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}
	first, err := HashPasswordWithConfig("correct horse battery staple", []byte(testSecret), config)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	second, err := HashPasswordWithConfig("correct horse battery staple", []byte(testSecret), config)
	if err != nil {
		t.Fatalf("hash password again: %v", err)
	}
	if first == second || !strings.HasPrefix(first, "$argon2id$v=19$") {
		t.Fatalf("expected unique PHC Argon2id hashes, got %q and %q", first, second)
	}
	valid, parsedConfig := verifyArgon2idPassword("correct horse battery staple", []byte(testSecret), first)
	if !valid || parsedConfig == nil {
		t.Fatal("correct password did not verify")
	}
	if valid, _ := verifyArgon2idPassword("wrong", []byte(testSecret), first); valid {
		t.Fatal("wrong password verified")
	}
	if valid, _ := verifyArgon2idPassword("correct horse battery staple", []byte(testSecret), first+"garbage"); valid {
		t.Fatal("malformed hash verified")
	}
	if !NeedsRehash(first, DefaultPasswordConfig()) {
		t.Fatal("weaker parameters should need rehash")
	}
}

func TestPasswordLegacyMigrationIsExplicit(t *testing.T) {
	legacy := HashLegacyPassword("password", testSecret)
	if VerifyPassword("password", testSecret, legacy) {
		t.Fatal("normal verification must not accept legacy HMAC")
	}
	if !VerifyLegacyPassword("password", testSecret, legacy) {
		t.Fatal("explicit legacy verifier rejected valid migration hash")
	}
	valid, upgrade := VerifyPasswordWithMigration("password", testSecret, legacy)
	if !valid || !upgrade {
		t.Fatalf("expected valid legacy hash requiring upgrade, got %v %v", valid, upgrade)
	}
}

func TestPasswordParameterBounds(t *testing.T) {
	unsafe := []PasswordConfig{
		{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32},
		{Memory: 8 * 1024, Iterations: 0, Parallelism: 1, SaltLength: 16, KeyLength: 32},
		{Memory: 8 * 1024, Iterations: 1, Parallelism: 0, SaltLength: 16, KeyLength: 32},
		{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 4, KeyLength: 32},
	}
	for _, config := range unsafe {
		if _, err := HashPasswordWithConfig("password", []byte(testSecret), config); err == nil {
			t.Fatalf("expected unsafe config rejection: %+v", config)
		}
	}
}

func TestTokenClaimsTamperingExpiryAndRotation(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	oldKey := []byte(strings.Repeat("o", 32))
	newKey := []byte(strings.Repeat("n", 32))
	manager := mustTokenManager(t, TokenConfig{
		Issuer: "issuer", Audience: "audience", ActiveKeyID: "old",
		Keys: map[string][]byte{"old": oldKey}, MaxTTL: time.Hour, ClockSkew: 0, Now: func() time.Time { return now },
	})
	token, err := manager.Issue(42, "user@example.com", 15*time.Minute)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	claims, err := manager.Verify(token)
	if err != nil || claims.UserID != 42 || claims.Issuer != "issuer" || claims.Audience != "audience" {
		t.Fatalf("verify token: %+v %v", claims, err)
	}

	tampered := tamperToken(token)
	if _, err := manager.Verify(tampered); err == nil {
		t.Fatal("tampered token verified")
	}
	now = now.Add(16 * time.Minute)
	if _, err := manager.Verify(token); err == nil {
		t.Fatal("expired token verified")
	}
	now = now.Add(-16 * time.Minute)

	rotated := mustTokenManager(t, TokenConfig{
		Issuer: "issuer", Audience: "audience", ActiveKeyID: "new",
		Keys: map[string][]byte{"old": oldKey, "new": newKey}, MaxTTL: time.Hour, Now: func() time.Time { return now },
	})
	if _, err := rotated.Verify(token); err != nil {
		t.Fatalf("rotated manager should accept explicitly retained old key: %v", err)
	}
	withoutOld := mustTokenManager(t, TokenConfig{
		Issuer: "issuer", Audience: "audience", ActiveKeyID: "new",
		Keys: map[string][]byte{"new": newKey}, MaxTTL: time.Hour, Now: func() time.Time { return now },
	})
	if _, err := withoutOld.Verify(token); err == nil {
		t.Fatal("manager accepted token through an unsafe fallback key")
	}
}

func TestTokenRejectsWrongPolicyAndMalformedClaims(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	key := []byte(testSecret)
	manager := mustTokenManager(t, TokenConfig{
		Issuer: "issuer", Audience: "audience", ActiveKeyID: "key",
		Keys: map[string][]byte{"key": key}, MaxTTL: time.Hour, Now: func() time.Time { return now },
	})
	validClaims := TokenClaims{
		Subject: "7", UserID: 7, Issuer: "issuer", Audience: "audience",
		IssuedAt: now.Unix(), NotBefore: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(), JWTID: "id",
	}
	invalidClaims := []TokenClaims{
		func() TokenClaims { value := validClaims; value.UserID = 0; return value }(),
		func() TokenClaims { value := validClaims; value.UserID = -7; return value }(),
		func() TokenClaims { value := validClaims; value.Subject = "-7"; return value }(),
		func() TokenClaims { value := validClaims; value.Issuer = "wrong"; return value }(),
		func() TokenClaims { value := validClaims; value.Audience = "wrong"; return value }(),
		func() TokenClaims { value := validClaims; value.NotBefore = now.Add(time.Minute).Unix(); return value }(),
		func() TokenClaims { value := validClaims; value.ExpiresAt = int64(^uint64(0) >> 1); return value }(),
	}
	for _, claims := range invalidClaims {
		token, err := manager.sign(claims)
		if err != nil {
			t.Fatalf("sign malformed claims: %v", err)
		}
		if _, err := manager.Verify(token); err == nil {
			t.Fatalf("invalid claims verified: %+v", claims)
		}
	}
	wrongKey := mustTokenManager(t, TokenConfig{
		Issuer: "issuer", Audience: "audience", ActiveKeyID: "key",
		Keys: map[string][]byte{"key": []byte(strings.Repeat("x", 32))}, MaxTTL: time.Hour, Now: func() time.Time { return now },
	})
	token, _ := manager.sign(validClaims)
	if _, err := wrongKey.Verify(token); err == nil {
		t.Fatal("wrong signing key verified")
	}
}

func TestTokenRejectsSubsecondTTL(t *testing.T) {
	manager := mustTokenManager(t, TokenConfig{
		Issuer: "issuer", Audience: "audience", ActiveKeyID: "key",
		Keys: map[string][]byte{"key": []byte(testSecret)}, MaxTTL: time.Hour,
	})
	if _, err := manager.Issue(1, "", time.Millisecond); err == nil {
		t.Fatal("subsecond TTL produced a token with second-granularity claims")
	}
}

func TestAuthenticateUsesOnlyVerifiedIdentity(t *testing.T) {
	manager := mustTokenManager(t, TokenConfig{
		Issuer: "issuer", Audience: "audience", ActiveKeyID: "key", Keys: map[string][]byte{"key": []byte(testSecret)},
		MaxTTL: time.Hour,
	})
	token, _ := manager.IssueUser(UserClaims{ID: "99", Roles: []string{"member"}}, 10*time.Minute)
	app := gpp.New()
	app.Use(AuthenticateWithManager(manager))
	app.GET("/protected", func(c *gpp.Context) error {
		user, ok := GetUser(c)
		if !ok || user.ID != "99" || user.HasRole("admin") {
			return gpp.ErrUnauthorized("wrong identity")
		}
		return c.NoContent()
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer arbitrary")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != 401 {
		t.Fatalf("arbitrary bearer token authenticated: %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response = httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != 204 {
		t.Fatalf("verified token rejected: %d %s", response.Code, response.Body.String())
	}
}

func TestSecureSessionRotationExpiryAndRevocation(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	manager, err := NewSessionManager(SessionConfig{
		TTL: 10 * time.Minute, SameSite: http.SameSiteStrictMode, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("session manager: %v", err)
	}
	app := gpp.New()
	app.GET("/login", func(c *gpp.Context) error {
		return c.String(200, "%s", manager.CreateSession(c, UserClaims{ID: "7"}))
	})
	app.GET("/private", manager.SessionMiddleware(), func(c *gpp.Context) error { return c.NoContent() })

	first := requestCookie(t, app, "/login", nil)
	if !first.HttpOnly || !first.Secure || first.SameSite != http.SameSiteStrictMode || first.MaxAge <= 0 || len(first.Value) < 40 {
		t.Fatalf("insecure session cookie: %+v", first)
	}
	second := requestCookie(t, app, "/login", first)
	if first.Value == second.Value {
		t.Fatal("session was not rotated")
	}
	assertSessionStatus(t, app, first, 401)
	assertSessionStatus(t, app, second, 204)
	manager.RevokeSession(second.Value)
	assertSessionStatus(t, app, second, 401)

	third := requestCookie(t, app, "/login", nil)
	now = now.Add(11 * time.Minute)
	assertSessionStatus(t, app, third, 401)
}

func TestSessionStoreCapacityIsBounded(t *testing.T) {
	manager, err := NewSessionManager(SessionConfig{TTL: time.Hour, MaxSessions: 1})
	if err != nil {
		t.Fatalf("session manager: %v", err)
	}
	firstContext := &gpp.Context{Writer: httptest.NewRecorder(), Request: httptest.NewRequest(http.MethodGet, "/", nil)}
	if manager.CreateSession(firstContext, UserClaims{ID: "1"}) == "" {
		t.Fatal("first session was not created")
	}
	secondContext := &gpp.Context{Writer: httptest.NewRecorder(), Request: httptest.NewRequest(http.MethodGet, "/", nil)}
	if manager.CreateSession(secondContext, UserClaims{ID: "2"}) != "" {
		t.Fatal("session store exceeded configured capacity")
	}
}

func TestTOTPAndDisabledPASETO(t *testing.T) {
	secret := "12345678901234567890"
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	code := GenerateTOTPCode(secret, now)
	if !VerifyTOTPCode(secret, code, now) || VerifyTOTPCode(secret, "000000", now) {
		t.Fatal("TOTP verification contract failed")
	}
	if GeneratePASETO(UserClaims{ID: "7"}, testSecret) != "" {
		t.Fatal("placeholder PASETO must remain disabled")
	}
}

func TestRBACAndABAC(t *testing.T) {
	claims := &UserClaims{ID: "7", Roles: []string{"admin"}, Attributes: map[string]string{"department": "finance"}}
	app := gpp.New()
	app.Use(func(c *gpp.Context) error {
		if err := installVerifiedIdentity(c, *claims); err != nil {
			return err
		}
		return c.Next()
	})
	app.GET("/admin", RequireRoles("admin"), RequirePolicy(func(user *UserClaims) bool {
		return user.Attributes["department"] == "finance"
	}), func(c *gpp.Context) error { return c.NoContent() })
	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != 204 {
		t.Fatalf("authorized request rejected: %d", response.Code)
	}
}

func mustTokenManager(t *testing.T, config TokenConfig) *TokenManager {
	t.Helper()
	manager, err := NewTokenManager(config)
	if err != nil {
		t.Fatalf("token manager: %v", err)
	}
	return manager
}

func requestCookie(t *testing.T, app *gpp.Engine, path string, previous *http.Cookie) *http.Cookie {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if previous != nil {
		request.AddCookie(previous)
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	return cookies[0]
}

func assertSessionStatus(t *testing.T, app *gpp.Engine, cookie *http.Cookie, status int) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/private", nil)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("expected session status %d, got %d", status, response.Code)
	}
}
