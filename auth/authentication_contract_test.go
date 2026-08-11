package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gpp "github.com/saifsilver/goplusplus"
)

func TestCanonicalBearerIdentityContract(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	manager := contractTokenManager(t, now, nil)
	expected := UserClaims{
		ID: "42", Subject: "42", Email: "person@example.com", Roles: []string{"admin"},
		Attributes: map[string]string{"department": "finance"}, TenantID: "tenant-1",
	}
	token, err := manager.IssueUser(expected, time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	var captured *UserClaims
	app := gpp.New()
	app.Use(AuthenticateWithManager(manager), RequireRoles("admin"), RequirePolicy(func(user *UserClaims) bool {
		return user.Attributes["department"] == "finance"
	}))
	app.GET("/private", func(c *gpp.Context) error {
		userID, err := c.RequireUserID()
		if err != nil || userID != 42 || c.UserID() != 42 {
			return gpp.ErrUnauthorized("identity mismatch")
		}
		captured, _ = GetUser(c)
		return c.NoContent()
	})
	response := performBearerRequest(app, token)
	if response.Code != http.StatusNoContent {
		t.Fatalf("verified identity rejected: %d %s", response.Code, response.Body.String())
	}
	if captured == nil || captured.ID != expected.ID || captured.Subject != expected.Subject || captured.Email != expected.Email ||
		captured.TenantID != expected.TenantID || len(captured.Roles) != 1 || captured.Attributes["department"] != "finance" {
		t.Fatalf("incomplete installed claims: %+v", captured)
	}
}

func TestCanonicalUUIDIdentityContract(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	manager := contractTokenManager(t, now, nil)
	userID := "018f47a6-7b5c-7c8d-9e0f-123456789abc"
	token, err := manager.IssueUser(UserClaims{ID: userID, Email: "uuid@example.com", Roles: []string{"member"}}, time.Hour)
	if err != nil {
		t.Fatalf("issue UUID identity: %v", err)
	}
	verified, err := manager.Verify(token)
	if err != nil || verified.Subject != userID || verified.UserIDString != userID || verified.UserID != 0 {
		t.Fatalf("UUID JWT claim mismatch: %+v %v", verified, err)
	}
	app := gpp.New()
	app.Use(AuthenticateWithManager(manager), RequireRoles("member"))
	app.GET("/private", func(c *gpp.Context) error {
		subject, err := c.RequireUserSubject()
		user, ok := GetUser(c)
		if err != nil || !ok || subject != userID || user.ID != userID || user.Subject != userID {
			return gpp.ErrUnauthorized("UUID identity mismatch")
		}
		if c.UserID() != 0 {
			return errors.New("numeric accessor must not coerce UUID identity")
		}
		return c.NoContent()
	})
	response := performBearerRequest(app, token)
	if response.Code != http.StatusNoContent {
		t.Fatalf("UUID identity rejected: %d %s", response.Code, response.Body.String())
	}
}

func TestCanonicalSessionAndUniversalIdentityContract(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	claims := UserClaims{ID: "7", Subject: "7", Email: "seven@example.com", Roles: []string{"member"}, Attributes: map[string]string{"tier": "pro"}, TenantID: "t7"}
	sessions, err := NewSessionManager(SessionConfig{TTL: time.Hour, AllowInsecureHTTP: true, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("session manager: %v", err)
	}
	tokens := contractTokenManager(t, now, nil)
	token, err := tokens.IssueUser(claims, time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	app := gpp.New()
	app.GET("/login", func(c *gpp.Context) error {
		if sessions.CreateSession(c, claims) == "" {
			return errors.New("create session")
		}
		return c.NoContent()
	})
	policy := RequirePolicy(func(user *UserClaims) bool { return user.Attributes["tier"] == "pro" })
	app.GET("/session", sessions.SessionMiddleware(), RequireRoles("member"), policy, identityResponse)
	app.GET("/universal", UniversalAuthWithManager(tokens, sessions), RequireRoles("member"), policy, identityResponse)

	login := httptest.NewRecorder()
	app.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/login", nil))
	cookie := login.Result().Cookies()[0]
	sessionResponse := authenticatedRequest(app, "/session", cookie, "")
	universalSession := authenticatedRequest(app, "/universal", cookie, "")
	universalBearer := authenticatedRequest(app, "/universal", nil, token)
	for name, response := range map[string]*httptest.ResponseRecorder{
		"session": sessionResponse, "universal session": universalSession, "universal bearer": universalBearer,
	} {
		if response.Code != http.StatusOK {
			t.Fatalf("%s rejected: %d %s", name, response.Code, response.Body.String())
		}
	}
	if sessionResponse.Body.String() != universalSession.Body.String() || sessionResponse.Body.String() != universalBearer.Body.String() {
		t.Fatalf("identity transport mismatch: %q %q %q", sessionResponse.Body, universalSession.Body, universalBearer.Body)
	}
}

func TestInvalidIdentityFailsClosed(t *testing.T) {
	for _, id := range []string{"", "0", "-1", "invalid identity"} {
		t.Run(fmt.Sprintf("id_%q", id), func(t *testing.T) {
			if _, _, _, err := canonicalUserClaims(UserClaims{ID: id}); err == nil {
				t.Fatal("invalid identity accepted")
			}
		})
	}
	if _, _, _, err := canonicalUserClaims(UserClaims{ID: "1", Subject: "2"}); err == nil {
		t.Fatal("conflicting subject accepted")
	}
	for _, value := range []any{int64(0), int64(-1), "invalid"} {
		c := &gpp.Context{}
		c.Set("user_id", value)
		if c.UserID() != 0 {
			t.Fatalf("invalid context identity accepted: %v", value)
		}
		if _, err := c.RequireUserID(); err == nil {
			t.Fatalf("invalid context identity required successfully: %v", value)
		}
	}
}

func TestFailedAuthenticationClearsStaleIdentity(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	manager := contractTokenManager(t, now, nil)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer invalid")
	c := &gpp.Context{Request: request, Writer: httptest.NewRecorder()}
	c.Set("user", &UserClaims{ID: "99"})
	c.Set("user_id", int64(99))
	c.Set("sub", "99")
	if err := AuthenticateWithManager(manager)(c); err == nil {
		t.Fatal("invalid token authenticated")
	}
	if _, ok := GetUser(c); ok || c.UserID() != 0 {
		t.Fatal("stale identity survived failed authentication")
	}
	for _, key := range []string{"user", "user_id", "sub"} {
		if _, ok := c.Get(key); ok {
			t.Fatalf("stale key %q remains installed", key)
		}
	}
}

func TestStrictBearerParsing(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	manager := contractTokenManager(t, now, nil)
	token, _ := manager.Issue(1, "", time.Hour)
	tests := []struct {
		name   string
		values []string
		status int
	}{
		{name: "missing", status: 401},
		{name: "empty", values: []string{"Bearer "}, status: 401},
		{name: "wrong scheme", values: []string{"Basic " + token}, status: 401},
		{name: "lowercase scheme", values: []string{"bearer " + token}, status: 401},
		{name: "mixed case scheme", values: []string{"BeArEr " + token}, status: 401},
		{name: "extra leading space", values: []string{" Bearer " + token}, status: 401},
		{name: "multiple spaces", values: []string{"Bearer  " + token}, status: 401},
		{name: "extra token", values: []string{"Bearer " + token + " extra"}, status: 401},
		{name: "multiple values", values: []string{"Bearer " + token, "Basic other"}, status: 401},
		{name: "oversized", values: []string{"Bearer " + strings.Repeat("x", maxEncodedTokenBytes+1)}, status: 401},
		{name: "malformed", values: []string{"Bearer a.b.c"}, status: 401},
		{name: "valid", values: []string{"Bearer " + token}, status: 204},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := gpp.New()
			app.Use(AuthenticateWithManager(manager))
			app.GET("/private", func(c *gpp.Context) error { return c.NoContent() })
			request := httptest.NewRequest(http.MethodGet, "/private", nil)
			for _, value := range test.values {
				request.Header.Add("Authorization", value)
			}
			response := httptest.NewRecorder()
			app.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("expected %d, got %d: %s", test.status, response.Code, response.Body.String())
			}
		})
	}
}

func TestJWTRejectsUnsupportedAlgorithmAndUnknownKeyID(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	manager := contractTokenManager(t, now, nil)
	token, _ := manager.Issue(1, "", time.Hour)
	for name, header := range map[string]tokenHeader{
		"unsupported algorithm": {Algorithm: "none", Type: "JWT", KeyID: "active"},
		"unknown key ID":        {Algorithm: "HS256", Type: "JWT", KeyID: "missing"},
		"wrong type":            {Algorithm: "HS256", Type: "OTHER", KeyID: "active"},
	} {
		t.Run(name, func(t *testing.T) {
			parts := strings.Split(token, ".")
			encoded, err := json.Marshal(header)
			if err != nil {
				t.Fatal(err)
			}
			parts[0] = base64.RawURLEncoding.EncodeToString(encoded)
			if _, err := manager.Verify(strings.Join(parts, ".")); err == nil {
				t.Fatal("invalid JWT header accepted")
			}
		})
	}
}

func TestBearerErrorsAreGenericProblemDetails(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	manager := contractTokenManager(t, now, nil)
	token := "credential-that-must-not-appear"
	app := gpp.New()
	app.Use(AuthenticateWithManager(manager))
	app.GET("/private", func(c *gpp.Context) error { return c.NoContent() })
	response := performBearerRequest(app, token)
	if response.Code != http.StatusUnauthorized || strings.Contains(response.Body.String(), token) ||
		!strings.Contains(response.Body.String(), "Invalid or expired bearer token") {
		t.Fatalf("unsafe authentication response: %d %s", response.Code, response.Body.String())
	}
}

func TestTokenCompatibilityChain(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	var calls atomic.Int64
	adapter := tokenVerifierFunc(func(context.Context, string) (UserClaims, error) {
		calls.Add(1)
		return UserClaims{ID: "88"}, nil
	})
	manager := contractTokenManager(t, now, []TokenCompatibility{{Verifier: adapter, AcceptUntil: now.Add(time.Hour), MaxTokenBytes: 128}})
	current, _ := manager.Issue(1, "", time.Hour)
	if _, err := manager.verifyCompatible(context.Background(), current); err != nil || calls.Load() != 0 {
		t.Fatalf("current JWT invoked compatibility: calls=%d err=%v", calls.Load(), err)
	}
	tampered := tamperToken(current)
	_, _ = manager.verifyCompatible(context.Background(), tampered)
	if calls.Load() != 0 {
		t.Fatal("recognized invalid current JWT fell through")
	}
	claims, err := manager.verifyCompatible(context.Background(), "app-private")
	if err != nil || claims.ID != "88" || calls.Load() != 1 {
		t.Fatalf("compatibility token failed: %+v %v calls=%d", claims, err, calls.Load())
	}
	before := calls.Load()
	if _, err := manager.verifyCompatible(context.Background(), strings.Repeat("x", 129)); err == nil || calls.Load() != before {
		t.Fatal("oversized compatibility token reached adapter")
	}
}

func TestTokenCompatibilityDeadlineOrderingAndIdentityValidation(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	var secondCalls atomic.Int64
	first := tokenVerifierFunc(func(context.Context, string) (UserClaims, error) {
		return UserClaims{}, errors.New("recognized but invalid")
	})
	second := tokenVerifierFunc(func(context.Context, string) (UserClaims, error) {
		secondCalls.Add(1)
		return UserClaims{ID: "9"}, nil
	})
	manager := contractTokenManager(t, now, []TokenCompatibility{
		{Verifier: first, AcceptUntil: now.Add(time.Hour), MaxTokenBytes: 32},
		{Verifier: second, AcceptUntil: now.Add(time.Hour), MaxTokenBytes: 32},
	})
	if _, err := manager.verifyCompatible(context.Background(), "private"); err == nil || secondCalls.Load() != 0 {
		t.Fatal("recognized invalid adapter result fell through")
	}

	expired := contractTokenManager(t, now, []TokenCompatibility{{Verifier: second, AcceptUntil: now, MaxTokenBytes: 32}})
	if _, err := expired.verifyCompatible(context.Background(), "private"); err == nil {
		t.Fatal("expired compatibility accepted")
	}
	invalidIdentity := tokenVerifierFunc(func(context.Context, string) (UserClaims, error) { return UserClaims{ID: "0"}, nil })
	invalid := contractTokenManager(t, now, []TokenCompatibility{{Verifier: invalidIdentity, AcceptUntil: now.Add(time.Hour), MaxTokenBytes: 32}})
	if _, err := invalid.verifyCompatible(context.Background(), "private"); err == nil {
		t.Fatal("adapter installed invalid identity")
	}
}

func TestLegacyV1TokenCompatibility(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	key := []byte(strings.Repeat("l", 32))
	config := &LegacyTokenConfig{SigningKey: key, AcceptUntil: now.Add(2 * time.Hour), MaxTTL: 24 * time.Hour, MaxTokenBytes: 1024}
	manager := contractTokenManagerWithLegacy(t, &now, config)
	token := signLegacyV1Token(t, key, legacyV1TokenClaims{UserID: 31, Email: "legacy@example.com", ExpiresAt: now.Add(time.Hour).Unix()})
	claims, err := manager.verifyCompatible(context.Background(), token)
	if err != nil || claims.ID != "31" || claims.Email != "legacy@example.com" {
		t.Fatalf("legacy token rejected: %+v %v", claims, err)
	}

	disabled := contractTokenManager(t, now, nil)
	if _, err := disabled.verifyCompatible(context.Background(), token); err == nil {
		t.Fatal("disabled legacy compatibility accepted token")
	}
	now = now.Add(3 * time.Hour)
	if _, err := manager.verifyCompatible(context.Background(), token); err == nil {
		t.Fatal("legacy token accepted after deadline")
	}
}

func TestLegacyV1TokenRejectsMalformedClaimsAndSignatures(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	key := []byte(strings.Repeat("l", 32))
	manager := contractTokenManagerWithLegacy(t, &now, &LegacyTokenConfig{
		SigningKey: key, AcceptUntil: now.Add(2 * time.Hour), MaxTTL: time.Hour, MaxTokenBytes: 1024,
	})
	tests := map[string]string{
		"zero identity":     signLegacyV1Token(t, key, legacyV1TokenClaims{UserID: 0, ExpiresAt: now.Add(time.Minute).Unix()}),
		"negative identity": signLegacyV1Token(t, key, legacyV1TokenClaims{UserID: -1, ExpiresAt: now.Add(time.Minute).Unix()}),
		"missing expiry":    signLegacyV1Token(t, key, legacyV1TokenClaims{UserID: 1}),
		"expired":           signLegacyV1Token(t, key, legacyV1TokenClaims{UserID: 1, ExpiresAt: now.Add(-time.Second).Unix()}),
		"excess lifetime":   signLegacyV1Token(t, key, legacyV1TokenClaims{UserID: 1, ExpiresAt: now.Add(2 * time.Hour).Unix()}),
		"invalid encoding":  "%%%.%%%",
	}
	valid := signLegacyV1Token(t, key, legacyV1TokenClaims{UserID: 1, ExpiresAt: now.Add(time.Minute).Unix()})
	tests["tampered signature"] = tamperToken(valid)
	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := manager.verifyCompatible(context.Background(), token); err == nil {
				t.Fatal("invalid legacy token accepted")
			}
		})
	}
}

func TestTokenVerificationCancellationAndConcurrency(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	manager := contractTokenManager(t, now, nil)
	token, _ := manager.Issue(42, "", time.Hour)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.verifyCompatible(cancelled, token); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	var wait sync.WaitGroup
	for range 32 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range 20 {
				claims, err := manager.Verify(token)
				if err != nil || claims.UserID != 42 {
					t.Errorf("concurrent verification failed: %+v %v", claims, err)
				}
			}
		}()
	}
	wait.Wait()
}

func TestSessionManagerConcurrentUse(t *testing.T) {
	manager, err := NewSessionManager(SessionConfig{TTL: time.Hour, AllowInsecureHTTP: true, MaxSessions: 256})
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for index := 1; index <= 64; index++ {
		wait.Add(1)
		go func(userID int) {
			defer wait.Done()
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			response := httptest.NewRecorder()
			context := &gpp.Context{Request: request, Writer: response}
			identifier := manager.CreateSession(context, UserClaims{ID: strconv.Itoa(userID)})
			if identifier == "" {
				t.Error("concurrent session creation failed")
				return
			}
			manager.RevokeSession(identifier)
		}(index)
	}
	wait.Wait()
}

type tokenVerifierFunc func(context.Context, string) (UserClaims, error)

func (function tokenVerifierFunc) VerifyToken(ctx context.Context, token string) (UserClaims, error) {
	return function(ctx, token)
}

func contractTokenManager(t *testing.T, now time.Time, compatibility []TokenCompatibility) *TokenManager {
	t.Helper()
	return mustTokenManager(t, TokenConfig{
		Issuer: "contract", Audience: "contract-api", ActiveKeyID: "active",
		Keys: map[string][]byte{"active": []byte(strings.Repeat("a", 32))}, MaxTTL: 24 * time.Hour,
		Now: func() time.Time { return now }, Compatibility: compatibility,
	})
}

func contractTokenManagerWithLegacy(t *testing.T, now *time.Time, legacy *LegacyTokenConfig) *TokenManager {
	t.Helper()
	return mustTokenManager(t, TokenConfig{
		Issuer: "contract", Audience: "contract-api", ActiveKeyID: "active",
		Keys: map[string][]byte{"active": []byte(strings.Repeat("a", 32))}, MaxTTL: 24 * time.Hour,
		Now: func() time.Time { return *now }, LegacyV1: legacy,
	})
}

func signLegacyV1Token(t *testing.T, key []byte, claims legacyV1TokenClaims) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal legacy token: %v", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func tamperToken(token string) string {
	replacement := byte('A')
	if token[len(token)-1] == replacement {
		replacement = 'B'
	}
	return token[:len(token)-1] + string(replacement)
}

func performBearerRequest(app http.Handler, token string) *httptest.ResponseRecorder {
	return authenticatedRequest(app, "/private", nil, token)
}

func authenticatedRequest(app http.Handler, path string, cookie *http.Cookie, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	return response
}

func identityResponse(c *gpp.Context) error {
	userID, err := c.RequireUserID()
	if err != nil {
		return err
	}
	user, ok := GetUser(c)
	if !ok {
		return gpp.ErrUnauthorized("missing identity")
	}
	return c.String(http.StatusOK, "%s|%s|%s|%s|%s|%d", user.ID, user.Subject, user.Email, user.TenantID, user.Attributes["tier"], userID)
}
