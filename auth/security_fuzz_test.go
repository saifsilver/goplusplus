package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gpp "github.com/saifsilver/goplusplus"
)

func FuzzEncodedPasswordHashes(f *testing.F) {
	for _, encoded := range []string{
		"", "$argon2id$", "$argon2id$v=19$m=65536,t=3,p=4$bad$bad", strings.Repeat("x", 2048),
	} {
		f.Add(encoded)
	}
	f.Fuzz(func(t *testing.T, encoded string) {
		if len(encoded) > 4096 {
			t.Skip()
		}
		_ = VerifyPassword("password", strings.Repeat("p", 32), encoded)
		_, _ = VerifyPasswordWithMigration("password", strings.Repeat("p", 32), encoded)
	})
}

func FuzzBearerHeaders(f *testing.F) {
	for _, header := range []string{"", "Bearer ", "bearer token", "Bearer a.b.c", "Bearer  token", "Basic token"} {
		f.Add(header)
	}
	f.Fuzz(func(t *testing.T, header string) {
		if len(header) > maxEncodedTokenBytes+64 {
			t.Skip()
		}
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Add("Authorization", header)
		context := &gpp.Context{Request: request, Writer: httptest.NewRecorder()}
		_, _ = bearerToken(context)
	})
}

func FuzzBearerTokens(f *testing.F) {
	manager, err := NewTokenManager(TokenConfig{
		Issuer: "fuzz", Audience: "fuzz-api", ActiveKeyID: "key",
		Keys: map[string][]byte{"key": []byte(strings.Repeat("k", 32))}, MaxTTL: time.Hour,
	})
	if err != nil {
		f.Fatalf("token manager: %v", err)
	}
	valid, err := manager.Issue(1, "", time.Minute)
	if err != nil {
		f.Fatalf("issue token: %v", err)
	}
	for _, token := range []string{"", "a.b.c", valid, strings.Repeat("x", maxEncodedTokenBytes+1)} {
		f.Add(token)
	}
	f.Fuzz(func(t *testing.T, token string) {
		if len(token) > maxEncodedTokenBytes+1 {
			t.Skip()
		}
		_, _ = manager.Verify(token)
	})
}

func FuzzJWTParts(f *testing.F) {
	manager, err := NewTokenManager(TokenConfig{
		Issuer: "fuzz", Audience: "fuzz-api", ActiveKeyID: "key",
		Keys: map[string][]byte{"key": []byte(strings.Repeat("k", 32))}, MaxTTL: time.Hour,
	})
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range [][3]string{{"e30", "e30", "bad"}, {"", "", ""}, {"eyJhbGciOiJIUzI1NiJ9", "e30", "signature"}} {
		f.Add(seed[0], seed[1], seed[2])
	}
	f.Fuzz(func(t *testing.T, header, payload, signature string) {
		if len(header)+len(payload)+len(signature) > maxEncodedTokenBytes {
			t.Skip()
		}
		_, _ = manager.Verify(header + "." + payload + "." + signature)
	})
}

func FuzzLegacyV1Tokens(f *testing.F) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	manager, err := NewTokenManager(TokenConfig{
		Issuer: "fuzz", Audience: "fuzz-api", ActiveKeyID: "key",
		Keys: map[string][]byte{"key": []byte(strings.Repeat("k", 32))}, MaxTTL: time.Hour, Now: func() time.Time { return now },
		LegacyV1: &LegacyTokenConfig{
			SigningKey: []byte(strings.Repeat("l", 32)), AcceptUntil: now.Add(time.Hour), MaxTTL: time.Hour, MaxTokenBytes: 1024,
		},
	})
	if err != nil {
		f.Fatal(err)
	}
	for _, token := range []string{"", ".", "a.b", "a.b.c", strings.Repeat("x", 1025)} {
		f.Add(token)
	}
	f.Fuzz(func(t *testing.T, token string) {
		if len(token) > 2048 {
			t.Skip()
		}
		_, _ = manager.verifyCompatible(context.Background(), token)
	})
}

func FuzzIdentityClaims(f *testing.F) {
	for _, seed := range [][2]string{{"1", "1"}, {"uuid-value", "uuid-value"}, {"0", "0"}, {"-1", "-1"}, {"", ""}} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, id, subject string) {
		if len(id) > 512 || len(subject) > 512 {
			t.Skip()
		}
		_, _, _, _ = canonicalUserClaims(UserClaims{ID: id, Subject: subject})
	})
}

func FuzzCompatibilityDispatch(f *testing.F) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	adapter := tokenVerifierFunc(func(context.Context, string) (UserClaims, error) {
		return UserClaims{}, ErrTokenFormatNotRecognized
	})
	manager, err := NewTokenManager(TokenConfig{
		Issuer: "fuzz", Audience: "fuzz-api", ActiveKeyID: "key",
		Keys: map[string][]byte{"key": []byte(strings.Repeat("k", 32))}, MaxTTL: time.Hour, Now: func() time.Time { return now },
		Compatibility: []TokenCompatibility{{Verifier: adapter, AcceptUntil: now.Add(time.Hour), MaxTokenBytes: 512}},
	})
	if err != nil {
		f.Fatal(err)
	}
	f.Add("private")
	f.Fuzz(func(t *testing.T, token string) {
		if len(token) > 1024 {
			t.Skip()
		}
		_, _ = manager.verifyCompatible(context.Background(), token)
	})
}

func FuzzLegacyPasswordHashes(f *testing.F) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	policy, err := NewPasswordPolicy(PasswordPolicyConfig{
		Pepper: []byte(strings.Repeat("p", 32)), Argon2id: testPasswordConfig(),
		LegacyV1: &LegacyPasswordConfig{AcceptUntil: now.Add(time.Hour)}, Now: func() time.Time { return now },
	})
	if err != nil {
		f.Fatal(err)
	}
	for _, hash := range []string{"", "bad", strings.Repeat("x", 43), HashLegacyPassword("password", strings.Repeat("p", 32))} {
		f.Add(hash)
	}
	f.Fuzz(func(t *testing.T, hash string) {
		if len(hash) > maxEncodedPasswordHashBytes+1 {
			t.Skip()
		}
		_ = policy.Verify("password", hash)
	})
}
