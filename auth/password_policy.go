package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/argon2"
)

// PasswordVerification is the complete credential-verification result.
type PasswordVerification uint8

const (
	// PasswordInvalid indicates that credential verification failed.
	PasswordInvalid PasswordVerification = iota
	// PasswordValid indicates a valid credential using the current policy.
	PasswordValid
	// PasswordValidNeedsRehash indicates valid credentials stored with an outdated policy.
	PasswordValidNeedsRehash
)

// ErrPasswordFormatNotRecognized permits the policy to try the next explicitly
// configured verification-only compatibility adapter.
var ErrPasswordFormatNotRecognized = errors.New("auth: password hash format not recognized")

// LegacyPasswordVerifier verifies one application-private historical hash format.
// It must never generate hashes and must use constant-time secret comparison.
type LegacyPasswordVerifier interface {
	VerifyPassword(password, encodedHash string) (bool, error)
}

// PasswordCompatibility bounds one temporary application-private verifier.
type PasswordCompatibility struct {
	Verifier    LegacyPasswordVerifier
	AcceptUntil time.Time
}

// LegacyPasswordConfig explicitly enables GoPlusPlus's pre-v1.11 HMAC hashes.
type LegacyPasswordConfig struct {
	AcceptUntil time.Time
}

// PasswordPolicyConfig defines immutable password hashing and migration policy.
type PasswordPolicyConfig struct {
	Pepper        []byte
	Argon2id      PasswordConfig
	LegacyV1      *LegacyPasswordConfig
	Compatibility []PasswordCompatibility
	Now           func() time.Time
}

type passwordCompatibility struct {
	verifier    LegacyPasswordVerifier
	acceptUntil time.Time
}

type passwordDeriver func(string, []byte, []byte, PasswordConfig) []byte

// PasswordPolicy owns Argon2id hashing, verification, missing-account timing
// protection, and explicit verification-only migration adapters.
type PasswordPolicy struct {
	pepper        []byte
	config        PasswordConfig
	dummyHash     string
	compatibility []passwordCompatibility
	now           func() time.Time
	derive        passwordDeriver
}

// NewPasswordPolicy validates and copies all configuration before constructing
// a dummy Argon2id hash used for missing-account and non-current-hash paths.
func NewPasswordPolicy(config PasswordPolicyConfig) (*PasswordPolicy, error) {
	return newPasswordPolicy(config, derivePasswordKey)
}

func newPasswordPolicy(config PasswordPolicyConfig, derive passwordDeriver) (*PasswordPolicy, error) {
	if derive == nil || len(config.Pepper) < 16 || len(config.Pepper) > 1024 || validatePasswordConfig(config.Argon2id) != nil {
		return nil, errors.New("auth: password policy configuration is invalid")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	compatibility := make([]passwordCompatibility, 0, len(config.Compatibility)+1)
	if config.LegacyV1 != nil {
		if config.LegacyV1.AcceptUntil.IsZero() {
			return nil, errors.New("auth: legacy password compatibility requires an acceptance deadline")
		}
		compatibility = append(compatibility, passwordCompatibility{
			verifier:    &legacyV1PasswordVerifier{pepper: append([]byte(nil), config.Pepper...)},
			acceptUntil: config.LegacyV1.AcceptUntil.UTC(),
		})
	}
	for _, entry := range config.Compatibility {
		if entry.Verifier == nil || entry.AcceptUntil.IsZero() {
			return nil, errors.New("auth: password compatibility requires a verifier and acceptance deadline")
		}
		compatibility = append(compatibility, passwordCompatibility{verifier: entry.Verifier, acceptUntil: entry.AcceptUntil.UTC()})
	}
	policy := &PasswordPolicy{
		pepper: append([]byte(nil), config.Pepper...), config: config.Argon2id,
		compatibility: compatibility, now: config.Now, derive: derive,
	}
	dummyHash, err := policy.hash("goplusplus-password-timing-guard")
	if err != nil {
		return nil, errors.New("auth: password timing guard initialization failed")
	}
	policy.dummyHash = dummyHash
	return policy, nil
}

// Hash creates a new randomized Argon2id PHC hash using the configured policy.
func (policy *PasswordPolicy) Hash(password string) (string, error) {
	if policy == nil {
		return "", errors.New("auth: password policy is unavailable")
	}
	return policy.hash(password)
}

func (policy *PasswordPolicy) hash(password string) (string, error) {
	if err := validatePasswordInputs(password, policy.pepper, policy.config); err != nil {
		return "", err
	}
	salt := make([]byte, policy.config.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", errors.New("auth: password salt generation failed")
	}
	key := policy.derive(password, policy.pepper, salt, policy.config)
	return encodeArgon2idHash(policy.config, salt, key), nil
}

// Verify performs exactly one bounded Argon2id verification operation. Empty,
// missing, malformed, and legacy hashes use the policy's dummy hash first.
func (policy *PasswordPolicy) Verify(password, encodedHash string) PasswordVerification {
	if policy == nil || len(password) == 0 || len(password) > maxPasswordBytes || len(encodedHash) > maxEncodedPasswordHashBytes {
		return PasswordInvalid
	}
	if len(encodedHash) >= len("$argon2id$") && encodedHash[:len("$argon2id$")] == "$argon2id$" {
		valid, config := verifyArgon2idPasswordWithDeriver(password, policy.pepper, encodedHash, policy.derive)
		if !valid || config == nil {
			return PasswordInvalid
		}
		if configNeedsRehash(*config, policy.config) {
			return PasswordValidNeedsRehash
		}
		return PasswordValid
	}
	_, _ = verifyArgon2idPasswordWithDeriver(password, policy.pepper, policy.dummyHash, policy.derive)
	if encodedHash == "" {
		return PasswordInvalid
	}
	for _, entry := range policy.compatibility {
		if !policy.now().UTC().Before(entry.acceptUntil) {
			continue
		}
		valid, err := entry.verifier.VerifyPassword(password, encodedHash)
		if err == nil {
			if valid {
				return PasswordValidNeedsRehash
			}
			return PasswordInvalid
		}
		if !errors.Is(err, ErrPasswordFormatNotRecognized) {
			return PasswordInvalid
		}
	}
	return PasswordInvalid
}

// VerifyMissing executes the same bounded dummy Argon2id path used when Verify
// receives an empty account hash.
func (policy *PasswordPolicy) VerifyMissing(password string) PasswordVerification {
	return policy.Verify(password, "")
}

func encodeArgon2idHash(config PasswordConfig, salt, key []byte) string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, config.Memory, config.Iterations, config.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key),
	)
}
