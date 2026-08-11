package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	maxPasswordBytes            = 1024
	maxEncodedPasswordHashBytes = 1024
)

// PasswordConfig defines bounded Argon2id cost and output parameters.
type PasswordConfig struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// DefaultPasswordConfig returns RFC 9106's memory-constrained recommendation.
func DefaultPasswordConfig() PasswordConfig {
	return PasswordConfig{Memory: 64 * 1024, Iterations: 3, Parallelism: 4, SaltLength: 16, KeyLength: 32}
}

// HashPassword hashes a password with Argon2id and a random salt. The secret is
// used as an application pepper. It returns an empty string on invalid input or entropy failure.
func HashPassword(password, secret string) string {
	hash, err := HashPasswordWithConfig(password, []byte(secret), DefaultPasswordConfig())
	if err != nil {
		return ""
	}
	return hash
}

// HashPasswordWithConfig returns a versioned PHC-style Argon2id hash.
func HashPasswordWithConfig(password string, pepper []byte, config PasswordConfig) (string, error) {
	if err := validatePasswordInputs(password, pepper, config); err != nil {
		return "", err
	}
	salt := make([]byte, config.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", errors.New("auth: password salt generation failed")
	}
	key := derivePasswordKey(password, pepper, salt, config)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, config.Memory, config.Iterations, config.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword verifies only Argon2id hashes. Use VerifyLegacyPassword explicitly for migration.
func VerifyPassword(password, secret, encodedHash string) bool {
	valid, _ := verifyArgon2idPassword(password, []byte(secret), encodedHash)
	return valid
}

// VerifyPasswordWithMigration verifies Argon2id or an explicitly recognized legacy HMAC hash.
// needsUpgrade is true only for a valid legacy hash or weaker Argon2id parameters.
func VerifyPasswordWithMigration(password, secret, encodedHash string) (valid, needsUpgrade bool) {
	if strings.HasPrefix(encodedHash, "$argon2id$") {
		valid, config := verifyArgon2idPassword(password, []byte(secret), encodedHash)
		return valid, valid && config != nil && configNeedsRehash(*config, DefaultPasswordConfig())
	}
	valid = VerifyLegacyPassword(password, secret, encodedHash)
	return valid, valid
}

// NeedsRehash reports whether a valid PHC hash uses parameters below the desired configuration.
func NeedsRehash(encodedHash string, desired PasswordConfig) bool {
	config, _, _, err := parseArgon2idHash(encodedHash)
	return err != nil || configNeedsRehash(config, desired)
}

// HashLegacyPassword reproduces the pre-v1.11 HMAC format for controlled migrations only.
func HashLegacyPassword(password, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(password))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyLegacyPassword verifies the pre-v1.11 HMAC format in constant time.
func VerifyLegacyPassword(password, secret, expectedHash string) bool {
	if expectedHash == "" || len(secret) < 16 {
		return false
	}
	actual := HashLegacyPassword(password, secret)
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expectedHash)) == 1
}

func verifyArgon2idPassword(password string, pepper []byte, encodedHash string) (bool, *PasswordConfig) {
	config, salt, expected, err := parseArgon2idHash(encodedHash)
	if err != nil || len(password) == 0 || len(password) > maxPasswordBytes || len(pepper) < 16 || len(pepper) > 1024 {
		return false, nil
	}
	actual := derivePasswordKey(password, pepper, salt, config)
	return subtle.ConstantTimeCompare(actual, expected) == 1, &config
}

func parseArgon2idHash(encodedHash string) (PasswordConfig, []byte, []byte, error) {
	if len(encodedHash) == 0 || len(encodedHash) > maxEncodedPasswordHashBytes {
		return PasswordConfig{}, nil, nil, errors.New("auth: malformed Argon2id hash")
	}
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return PasswordConfig{}, nil, nil, errors.New("auth: malformed Argon2id hash")
	}
	var config PasswordConfig
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &config.Memory, &config.Iterations, &config.Parallelism); err != nil {
		return PasswordConfig{}, nil, nil, errors.New("auth: malformed Argon2id parameters")
	}
	if parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", config.Memory, config.Iterations, config.Parallelism) {
		return PasswordConfig{}, nil, nil, errors.New("auth: malformed Argon2id parameters")
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil {
		return PasswordConfig{}, nil, nil, errors.New("auth: malformed Argon2id salt")
	}
	key, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil {
		return PasswordConfig{}, nil, nil, errors.New("auth: malformed Argon2id key")
	}
	config.SaltLength = uint32(len(salt))
	config.KeyLength = uint32(len(key))
	if err := validatePasswordConfig(config); err != nil {
		return PasswordConfig{}, nil, nil, err
	}
	return config, salt, key, nil
}

func derivePasswordKey(password string, pepper, salt []byte, config PasswordConfig) []byte {
	material := make([]byte, 0, len(password)+len(pepper))
	material = append(material, password...)
	material = append(material, pepper...)
	key := argon2.IDKey(material, salt, config.Iterations, config.Memory, config.Parallelism, config.KeyLength)
	for index := range material {
		material[index] = 0
	}
	return key
}

func validatePasswordInputs(password string, pepper []byte, config PasswordConfig) error {
	if len(password) == 0 || len(password) > maxPasswordBytes {
		return errors.New("auth: password length is invalid")
	}
	if len(pepper) < 16 || len(pepper) > 1024 {
		return errors.New("auth: password pepper must contain between 16 and 1024 bytes")
	}
	return validatePasswordConfig(config)
}

func validatePasswordConfig(config PasswordConfig) error {
	if config.Memory < 8*1024 || config.Memory > 128*1024 || config.Iterations < 1 || config.Iterations > 5 ||
		config.Parallelism < 1 || config.Parallelism > 16 || config.SaltLength < 16 || config.SaltLength > 64 ||
		config.KeyLength < 16 || config.KeyLength > 64 {
		return errors.New("auth: Argon2id parameters are outside safe bounds")
	}
	return nil
}

func configNeedsRehash(actual, desired PasswordConfig) bool {
	return actual.Memory != desired.Memory || actual.Iterations != desired.Iterations ||
		actual.Parallelism != desired.Parallelism || actual.SaltLength != desired.SaltLength ||
		actual.KeyLength != desired.KeyLength
}
