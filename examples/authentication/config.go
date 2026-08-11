package authentication

import (
	"errors"
	"os"
	"time"

	"github.com/saifsilver/goplusplus/auth"
)

// AuthComponentsFromEnvironment constructs immutable production auth policy.
// Delete the GPP_LEGACY_* configuration after every pre-upgrade credential has expired or migrated.
func AuthComponentsFromEnvironment() (*auth.TokenManager, *auth.PasswordPolicy, error) {
	issuer, audience, activeKeyID := os.Getenv("GPP_AUTH_ISSUER"), os.Getenv("GPP_AUTH_AUDIENCE"), os.Getenv("GPP_AUTH_ACTIVE_KEY_ID")
	activeKey, pepper := []byte(os.Getenv("GPP_AUTH_SIGNING_KEY")), []byte(os.Getenv("GPP_PASSWORD_PEPPER"))
	if issuer == "" || audience == "" || activeKeyID == "" || len(activeKey) < 32 || len(pepper) < 16 {
		return nil, nil, errors.New("authentication example: required auth environment is missing or invalid")
	}
	keys, err := rotationKeys(activeKeyID, activeKey)
	if err != nil {
		return nil, nil, err
	}
	legacyTokens, err := legacyTokenConfig()
	if err != nil {
		return nil, nil, err
	}
	tokens, err := auth.NewTokenManager(auth.TokenConfig{
		Issuer: issuer, Audience: audience, ActiveKeyID: activeKeyID, Keys: keys,
		MaxTTL: 24 * time.Hour, ClockSkew: time.Minute, LegacyV1: legacyTokens,
	})
	if err != nil {
		return nil, nil, err
	}
	legacyPasswords, err := legacyPasswordConfig()
	if err != nil {
		return nil, nil, err
	}
	passwords, err := auth.NewPasswordPolicy(auth.PasswordPolicyConfig{
		Pepper: pepper, Argon2id: auth.DefaultPasswordConfig(), LegacyV1: legacyPasswords,
	})
	if err != nil {
		return nil, nil, err
	}
	return tokens, passwords, nil
}

func rotationKeys(activeKeyID string, activeKey []byte) (map[string][]byte, error) {
	keys := map[string][]byte{activeKeyID: activeKey}
	previousID, previousKey := os.Getenv("GPP_AUTH_PREVIOUS_KEY_ID"), []byte(os.Getenv("GPP_AUTH_PREVIOUS_SIGNING_KEY"))
	if previousID == "" && len(previousKey) == 0 {
		return keys, nil
	}
	if previousID == "" || len(previousKey) < 32 || previousID == activeKeyID {
		return nil, errors.New("authentication example: previous signing key configuration is invalid")
	}
	keys[previousID] = previousKey
	return keys, nil
}

func legacyTokenConfig() (*auth.LegacyTokenConfig, error) {
	key := []byte(os.Getenv("GPP_LEGACY_TOKEN_SIGNING_KEY"))
	deadline := os.Getenv("GPP_LEGACY_TOKEN_ACCEPT_UNTIL")
	if len(key) == 0 && deadline == "" {
		return nil, nil
	}
	acceptUntil, err := time.Parse(time.RFC3339, deadline)
	if err != nil || len(key) < 32 {
		return nil, errors.New("authentication example: legacy token compatibility requires a strong key and RFC3339 deadline")
	}
	return &auth.LegacyTokenConfig{SigningKey: key, AcceptUntil: acceptUntil, MaxTTL: 24 * time.Hour, MaxTokenBytes: 1024}, nil
}

func legacyPasswordConfig() (*auth.LegacyPasswordConfig, error) {
	deadline := os.Getenv("GPP_LEGACY_PASSWORD_ACCEPT_UNTIL")
	if deadline == "" {
		return nil, nil
	}
	acceptUntil, err := time.Parse(time.RFC3339, deadline)
	if err != nil {
		return nil, errors.New("authentication example: legacy password compatibility requires an RFC3339 deadline")
	}
	return &auth.LegacyPasswordConfig{AcceptUntil: acceptUntil}, nil
}
