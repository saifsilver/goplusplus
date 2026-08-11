package auth

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPasswordPolicyHashVerifyAndRehash(t *testing.T) {
	config := testPasswordConfig()
	policy, err := NewPasswordPolicy(PasswordPolicyConfig{Pepper: []byte(testSecret), Argon2id: config})
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}
	first, err := policy.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	second, err := policy.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash again: %v", err)
	}
	if first == second {
		t.Fatal("password hashes reused a salt")
	}
	if result := policy.Verify("correct horse battery staple", first); result != PasswordValid {
		t.Fatalf("current hash result = %v", result)
	}
	if result := policy.Verify("wrong", first); result != PasswordInvalid {
		t.Fatalf("wrong password result = %v", result)
	}
	if result := policy.Verify("correct horse battery staple", first+"corrupt"); result != PasswordInvalid {
		t.Fatalf("malformed hash result = %v", result)
	}
	parts := strings.Split(first, "$")
	for name, index := range map[string]int{"salt": 4, "derived key": 5} {
		corrupt := append([]string(nil), parts...)
		corrupt[index] = "%%%"
		if result := policy.Verify("correct horse battery staple", strings.Join(corrupt, "$")); result != PasswordInvalid {
			t.Fatalf("corrupt %s result = %v", name, result)
		}
	}

	weaker := PasswordConfig{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 16}
	weakHash, err := HashPasswordWithConfig("password", []byte(testSecret), weaker)
	if err != nil {
		t.Fatalf("weak hash: %v", err)
	}
	if result := policy.Verify("password", weakHash); result != PasswordValidNeedsRehash {
		t.Fatalf("weak valid hash result = %v", result)
	}
}

func TestPasswordPolicyTimingGuardPerformsOneKDF(t *testing.T) {
	var calls atomic.Int64
	derive := func(password string, pepper, salt []byte, config PasswordConfig) []byte {
		calls.Add(1)
		return derivePasswordKey(password, pepper, salt, config)
	}
	policy, err := newPasswordPolicy(PasswordPolicyConfig{Pepper: []byte(testSecret), Argon2id: testPasswordConfig()}, derive)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}
	hash, err := policy.Hash("password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	calls.Store(0)
	if result := policy.VerifyMissing("password"); result != PasswordInvalid || calls.Load() != 1 {
		t.Fatalf("missing account result=%v KDF calls=%d", result, calls.Load())
	}
	calls.Store(0)
	if result := policy.Verify("password", hash); result != PasswordValid || calls.Load() != 1 {
		t.Fatalf("existing account result=%v KDF calls=%d", result, calls.Load())
	}
	calls.Store(0)
	if result := policy.Verify("password", "malformed"); result != PasswordInvalid || calls.Load() != 1 {
		t.Fatalf("malformed hash result=%v KDF calls=%d", result, calls.Load())
	}
}

func TestPasswordPolicyLegacyCompatibility(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	policy, err := NewPasswordPolicy(PasswordPolicyConfig{
		Pepper: []byte(testSecret), Argon2id: testPasswordConfig(),
		LegacyV1: &LegacyPasswordConfig{AcceptUntil: now.Add(time.Hour)}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}
	legacy := HashLegacyPassword("password", testSecret)
	if result := policy.Verify("password", legacy); result != PasswordValidNeedsRehash {
		t.Fatalf("valid legacy result = %v", result)
	}
	if result := policy.Verify("wrong", legacy); result != PasswordInvalid {
		t.Fatalf("invalid legacy result = %v", result)
	}
	now = now.Add(2 * time.Hour)
	if result := policy.Verify("password", legacy); result != PasswordInvalid {
		t.Fatalf("expired legacy compatibility result = %v", result)
	}

	disabled, err := NewPasswordPolicy(PasswordPolicyConfig{Pepper: []byte(testSecret), Argon2id: testPasswordConfig()})
	if err != nil {
		t.Fatalf("new disabled policy: %v", err)
	}
	if result := disabled.Verify("password", legacy); result != PasswordInvalid {
		t.Fatalf("disabled legacy compatibility result = %v", result)
	}
}

func TestPasswordPolicyApplicationCompatibilityIsBoundedAndOrdered(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	var secondCalls atomic.Int64
	first := legacyPasswordVerifierFunc(func(string, string) (bool, error) {
		return false, errors.New("recognized invalid")
	})
	second := legacyPasswordVerifierFunc(func(password, hash string) (bool, error) {
		secondCalls.Add(1)
		return password == "password" && hash == "app$hash", nil
	})
	policy, err := NewPasswordPolicy(PasswordPolicyConfig{
		Pepper: []byte(testSecret), Argon2id: testPasswordConfig(), Now: func() time.Time { return now },
		Compatibility: []PasswordCompatibility{
			{Verifier: first, AcceptUntil: now.Add(time.Hour)},
			{Verifier: second, AcceptUntil: now.Add(time.Hour)},
		},
	})
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}
	if result := policy.Verify("password", "app$hash"); result != PasswordInvalid || secondCalls.Load() != 0 {
		t.Fatalf("recognized invalid adapter fell through: result=%v calls=%d", result, secondCalls.Load())
	}

	policy, err = NewPasswordPolicy(PasswordPolicyConfig{
		Pepper: []byte(testSecret), Argon2id: testPasswordConfig(), Now: func() time.Time { return now },
		Compatibility: []PasswordCompatibility{{Verifier: second, AcceptUntil: now.Add(time.Hour)}},
	})
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}
	if result := policy.Verify("password", "app$hash"); result != PasswordValidNeedsRehash {
		t.Fatalf("application legacy result = %v", result)
	}
	now = now.Add(2 * time.Hour)
	if result := policy.Verify("password", "app$hash"); result != PasswordInvalid {
		t.Fatalf("expired application compatibility result = %v", result)
	}
}

func TestPasswordPolicyBoundsAndErrorRedaction(t *testing.T) {
	password := "sensitive-password-value"
	hash := "sensitive-hash-value"
	invalidConfigs := []PasswordPolicyConfig{
		{Pepper: []byte("short"), Argon2id: testPasswordConfig()},
		{Pepper: []byte(strings.Repeat("p", 1025)), Argon2id: testPasswordConfig()},
		{Pepper: []byte(testSecret), Argon2id: PasswordConfig{}},
		{Pepper: []byte(testSecret), Argon2id: testPasswordConfig(), LegacyV1: &LegacyPasswordConfig{}},
	}
	for _, config := range invalidConfigs {
		if _, err := NewPasswordPolicy(config); err == nil {
			t.Fatal("invalid password policy accepted")
		} else if strings.Contains(err.Error(), password) || strings.Contains(err.Error(), hash) || strings.Contains(err.Error(), testSecret) {
			t.Fatalf("configuration error leaked secret: %v", err)
		}
	}
	policy, err := NewPasswordPolicy(PasswordPolicyConfig{Pepper: []byte(testSecret), Argon2id: testPasswordConfig()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Hash(strings.Repeat("p", maxPasswordBytes+1)); err == nil {
		t.Fatal("oversized password hashed")
	} else if strings.Contains(err.Error(), password) || strings.Contains(err.Error(), testSecret) {
		t.Fatalf("hash error leaked secret: %v", err)
	}
	if result := policy.Verify(password, strings.Repeat("h", maxEncodedPasswordHashBytes+1)); result != PasswordInvalid {
		t.Fatalf("oversized hash result = %v", result)
	}
}

func TestPasswordPolicyConcurrentUse(t *testing.T) {
	policy, err := NewPasswordPolicy(PasswordPolicyConfig{Pepper: []byte(testSecret), Argon2id: testPasswordConfig()})
	if err != nil {
		t.Fatal(err)
	}
	hash, err := policy.Hash("password")
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range 4 {
				if result := policy.Verify("password", hash); result != PasswordValid {
					t.Errorf("concurrent result = %v", result)
				}
			}
		}()
	}
	wait.Wait()
}

type legacyPasswordVerifierFunc func(string, string) (bool, error)

func (function legacyPasswordVerifierFunc) VerifyPassword(password, encodedHash string) (bool, error) {
	return function(password, encodedHash)
}

func testPasswordConfig() PasswordConfig {
	return PasswordConfig{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}
}
