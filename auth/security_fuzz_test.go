package auth

import (
	"strings"
	"testing"
	"time"
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
