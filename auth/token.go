package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	gpp "github.com/saifsilver/goplusplus"
)

const (
	defaultTokenIssuer   = "goplusplus"
	defaultTokenAudience = "goplusplus-api"
	defaultTokenKeyID    = "default"
	maxEncodedTokenBytes = 16 << 10
)

// ErrTokenFormatNotRecognized is the only error that permits verification to
// continue to the next explicitly configured compatibility verifier.
var ErrTokenFormatNotRecognized = errors.New("auth: token format not recognized")

// TokenVerifier verifies one bounded token format and returns trusted claims.
// Implementations must return ErrTokenFormatNotRecognized only when the token
// is unambiguously not their format; recognized invalid tokens must return a
// different error so the chain fails closed.
type TokenVerifier interface {
	VerifyToken(context.Context, string) (UserClaims, error)
}

// TokenCompatibility configures one temporary application-private verifier.
// AcceptUntil and MaxTokenBytes are mandatory so compatibility is bounded.
type TokenCompatibility struct {
	Verifier      TokenVerifier
	AcceptUntil   time.Time
	MaxTokenBytes int
}

// LegacyTokenConfig enables verification of GoPlusPlus's signed pre-v1.11
// two-part HMAC token. It never enables token generation.
type LegacyTokenConfig struct {
	SigningKey    []byte
	AcceptUntil   time.Time
	MaxTTL        time.Duration
	MaxTokenBytes int
}

type tokenCompatibility struct {
	verifier      TokenVerifier
	acceptUntil   time.Time
	maxTokenBytes int
}

// TokenClaims contains verified JWT claims.
type TokenClaims struct {
	Subject      string            `json:"sub"`
	UserID       int64             `json:"-"`
	UserIDString string            `json:"-"`
	Email        string            `json:"email,omitempty"`
	Roles        []string          `json:"roles,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	TenantID     string            `json:"tenant_id,omitempty"`
	Issuer       string            `json:"iss"`
	Audience     string            `json:"aud"`
	ExpiresAt    int64             `json:"exp"`
	NotBefore    int64             `json:"nbf"`
	IssuedAt     int64             `json:"iat"`
	JWTID        string            `json:"jti"`
}

type tokenClaimsWire struct {
	Subject    string            `json:"sub"`
	UserID     json.RawMessage   `json:"user_id,omitempty"`
	Email      string            `json:"email,omitempty"`
	Roles      []string          `json:"roles,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	TenantID   string            `json:"tenant_id,omitempty"`
	Issuer     string            `json:"iss"`
	Audience   string            `json:"aud"`
	ExpiresAt  int64             `json:"exp"`
	NotBefore  int64             `json:"nbf"`
	IssuedAt   int64             `json:"iat"`
	JWTID      string            `json:"jti"`
}

// MarshalJSON encodes numeric and opaque user identities without ambiguity.
func (claims TokenClaims) MarshalJSON() ([]byte, error) {
	var userID json.RawMessage
	if claims.UserIDString != "" {
		encoded, err := json.Marshal(claims.UserIDString)
		if err != nil {
			return nil, err
		}
		userID = encoded
	} else if claims.UserID != 0 {
		userID = json.RawMessage(strconv.FormatInt(claims.UserID, 10))
	}
	return json.Marshal(tokenClaimsWire{
		Subject: claims.Subject, UserID: userID, Email: claims.Email, Roles: claims.Roles,
		Attributes: claims.Attributes, TenantID: claims.TenantID, Issuer: claims.Issuer, Audience: claims.Audience,
		ExpiresAt: claims.ExpiresAt, NotBefore: claims.NotBefore, IssuedAt: claims.IssuedAt, JWTID: claims.JWTID,
	})
}

// UnmarshalJSON strictly decodes token claims and preserves identity shape.
func (claims *TokenClaims) UnmarshalJSON(data []byte) error {
	var wire tokenClaimsWire
	if err := strictJSON(data, &wire); err != nil {
		return err
	}
	*claims = TokenClaims{
		Subject: wire.Subject, Email: wire.Email, Roles: wire.Roles, Attributes: wire.Attributes, TenantID: wire.TenantID,
		Issuer: wire.Issuer, Audience: wire.Audience, ExpiresAt: wire.ExpiresAt,
		NotBefore: wire.NotBefore, IssuedAt: wire.IssuedAt, JWTID: wire.JWTID,
	}
	if len(wire.UserID) == 0 || string(wire.UserID) == "null" {
		return nil
	}
	if wire.UserID[0] == '"' {
		return json.Unmarshal(wire.UserID, &claims.UserIDString)
	}
	return json.Unmarshal(wire.UserID, &claims.UserID)
}

type tokenHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
	KeyID     string `json:"kid"`
}

type legacyV1TokenClaims struct {
	UserID    int64  `json:"user_id"`
	Email     string `json:"email,omitempty"`
	ExpiresAt int64  `json:"exp"`
}

type legacyV1TokenVerifier struct {
	key    []byte
	maxTTL time.Duration
	now    func() time.Time
}

func newLegacyV1TokenVerifier(config LegacyTokenConfig, now func() time.Time) (*legacyV1TokenVerifier, error) {
	if len(config.SigningKey) < 32 || config.AcceptUntil.IsZero() || config.MaxTTL < time.Second ||
		config.MaxTTL > 30*24*time.Hour || config.MaxTokenBytes < 1 || config.MaxTokenBytes > maxEncodedTokenBytes {
		return nil, errors.New("auth: legacy token compatibility configuration is outside safe bounds")
	}
	return &legacyV1TokenVerifier{key: append([]byte(nil), config.SigningKey...), maxTTL: config.MaxTTL, now: now}, nil
}

// VerifyToken verifies the signed legacy v1 token representation.
func (verifier *legacyV1TokenVerifier) VerifyToken(ctx context.Context, token string) (UserClaims, error) {
	if err := ctx.Err(); err != nil {
		return UserClaims{}, err
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return UserClaims{}, ErrTokenFormatNotRecognized
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || len(signature) != sha256.Size {
		return UserClaims{}, errors.New("auth: invalid legacy token")
	}
	mac := hmac.New(sha256.New, verifier.key)
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return UserClaims{}, errors.New("auth: invalid legacy token")
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil {
		return UserClaims{}, errors.New("auth: invalid legacy token")
	}
	var claims legacyV1TokenClaims
	if err := strictJSON(payload, &claims); err != nil || claims.UserID <= 0 || claims.ExpiresAt <= 0 {
		return UserClaims{}, errors.New("auth: invalid legacy token")
	}
	now := verifier.now().UTC()
	expiresAt := time.Unix(claims.ExpiresAt, 0)
	if !now.Before(expiresAt) || expiresAt.After(now.Add(verifier.maxTTL)) {
		return UserClaims{}, errors.New("auth: invalid legacy token")
	}
	identity := UserClaims{ID: strconv.FormatInt(claims.UserID, 10), Subject: strconv.FormatInt(claims.UserID, 10), Email: claims.Email}
	return identity, nil
}

// TokenConfig defines immutable JWT verification and key-rotation policy.
type TokenConfig struct {
	Issuer        string
	Audience      string
	ActiveKeyID   string
	Keys          map[string][]byte
	MaxTTL        time.Duration
	ClockSkew     time.Duration
	Now           func() time.Time
	LegacyV1      *LegacyTokenConfig
	Compatibility []TokenCompatibility
}

// TokenManager issues and verifies HS256 JWTs using explicit key IDs.
type TokenManager struct {
	issuer        string
	audience      string
	activeKeyID   string
	keys          map[string][]byte
	maxTTL        time.Duration
	clockSkew     time.Duration
	now           func() time.Time
	compatibility []tokenCompatibility
}

// NewTokenManager validates and copies immutable signing and compatibility policy.
func NewTokenManager(config TokenConfig) (*TokenManager, error) {
	if config.Issuer == "" || config.Audience == "" || config.ActiveKeyID == "" {
		return nil, errors.New("auth: issuer, audience, and active key ID are required")
	}
	if config.MaxTTL < time.Second || config.MaxTTL > 30*24*time.Hour || config.ClockSkew < 0 || config.ClockSkew > 5*time.Minute {
		return nil, errors.New("auth: token lifetime configuration is outside safe bounds")
	}
	keys := make(map[string][]byte, len(config.Keys))
	for keyID, key := range config.Keys {
		if keyID == "" || len(key) < 32 {
			return nil, errors.New("auth: every signing key requires an ID and at least 32 bytes")
		}
		keys[keyID] = append([]byte(nil), key...)
	}
	if _, ok := keys[config.ActiveKeyID]; !ok {
		return nil, errors.New("auth: active signing key is missing")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	compatibility, err := buildTokenCompatibility(config, config.Now)
	if err != nil {
		return nil, err
	}
	return &TokenManager{
		issuer: config.Issuer, audience: config.Audience, activeKeyID: config.ActiveKeyID,
		keys: keys, maxTTL: config.MaxTTL, clockSkew: config.ClockSkew, now: config.Now, compatibility: compatibility,
	}, nil
}

func buildTokenCompatibility(config TokenConfig, now func() time.Time) ([]tokenCompatibility, error) {
	entries := make([]tokenCompatibility, 0, len(config.Compatibility)+1)
	if config.LegacyV1 != nil {
		legacy, err := newLegacyV1TokenVerifier(*config.LegacyV1, now)
		if err != nil {
			return nil, err
		}
		entries = append(entries, tokenCompatibility{
			verifier: legacy, acceptUntil: config.LegacyV1.AcceptUntil.UTC(), maxTokenBytes: config.LegacyV1.MaxTokenBytes,
		})
	}
	for _, entry := range config.Compatibility {
		if entry.Verifier == nil || entry.AcceptUntil.IsZero() || entry.MaxTokenBytes < 1 || entry.MaxTokenBytes > maxEncodedTokenBytes {
			return nil, errors.New("auth: token compatibility requires a verifier, deadline, and bounded token size")
		}
		entries = append(entries, tokenCompatibility{
			verifier: entry.Verifier, acceptUntil: entry.AcceptUntil.UTC(), maxTokenBytes: entry.MaxTokenBytes,
		})
	}
	return entries, nil
}

// Issue creates a signed JWT for a positive numeric user ID and explicit bounded TTL.
func (manager *TokenManager) Issue(userID int64, email string, ttl time.Duration) (string, error) {
	return manager.IssueUser(UserClaims{ID: strconv.FormatInt(userID, 10), Email: email}, ttl)
}

// IssueUser creates a signed JWT containing trusted authorization claims.
func (manager *TokenManager) IssueUser(user UserClaims, ttl time.Duration) (string, error) {
	verified, userID, numeric, err := canonicalUserClaims(user)
	if err != nil || ttl < time.Second || ttl > manager.maxTTL {
		return "", errors.New("auth: user ID and explicit token TTL must be valid")
	}
	now := manager.now().UTC()
	jti, err := randomTokenIdentifier()
	if err != nil {
		return "", err
	}
	claims := TokenClaims{
		Subject: verified.Subject, UserID: userID, Email: verified.Email,
		Roles: append([]string(nil), verified.Roles...), Attributes: cloneStringMap(verified.Attributes), TenantID: verified.TenantID,
		Issuer: manager.issuer, Audience: manager.audience, IssuedAt: now.Unix(), NotBefore: now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(), JWTID: jti,
	}
	if !numeric {
		claims.UserIDString = verified.ID
	}
	return manager.sign(claims)
}

// Verify authenticates a JWT signature and all required registered claims.
func (manager *TokenManager) Verify(token string) (*TokenClaims, error) {
	claims, err := manager.verifyJWT(context.Background(), token)
	if errors.Is(err, ErrTokenFormatNotRecognized) {
		return nil, errors.New("auth: invalid token")
	}
	return claims, err
}

func (manager *TokenManager) verifyJWT(ctx context.Context, token string) (*TokenClaims, error) {
	if manager == nil {
		return nil, errors.New("auth: invalid token manager")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(token) == 0 || len(token) > maxEncodedTokenBytes {
		return nil, errors.New("auth: invalid token")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		if len(parts) > 0 && recognizedJWTHeader(parts[0]) {
			return nil, errors.New("auth: invalid token")
		}
		return nil, ErrTokenFormatNotRecognized
	}
	headerBytes, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("auth: invalid token")
	}
	var header tokenHeader
	if err := strictJSON(headerBytes, &header); err != nil || header.Algorithm != "HS256" || header.Type != "JWT" || header.KeyID == "" {
		return nil, errors.New("auth: unsupported token header")
	}
	key, ok := manager.keys[header.KeyID]
	if !ok {
		return nil, errors.New("auth: unknown signing key")
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("auth: invalid token signature")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return nil, errors.New("auth: invalid token signature")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("auth: invalid token claims")
	}
	var claims TokenClaims
	if err := strictJSON(payload, &claims); err != nil || manager.validateClaims(claims) != nil {
		return nil, errors.New("auth: invalid token claims")
	}
	return &claims, nil
}

func recognizedJWTHeader(encoded string) bool {
	headerBytes, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return false
	}
	var header tokenHeader
	if strictJSON(headerBytes, &header) != nil {
		return false
	}
	return header.Type == "JWT" || header.Algorithm != "" || header.KeyID != ""
}

// VerifyToken implements TokenVerifier for current GoPlusPlus JWTs.
func (manager *TokenManager) VerifyToken(ctx context.Context, token string) (UserClaims, error) {
	claims, err := manager.verifyJWT(ctx, token)
	if err != nil {
		return UserClaims{}, err
	}
	return *claims.userClaims(), nil
}

func (manager *TokenManager) verifyCompatible(ctx context.Context, token string) (UserClaims, error) {
	claims, err := manager.VerifyToken(ctx, token)
	if err == nil {
		return claims, nil
	}
	if !errors.Is(err, ErrTokenFormatNotRecognized) {
		return UserClaims{}, err
	}
	for _, entry := range manager.compatibility {
		if err := ctx.Err(); err != nil {
			return UserClaims{}, err
		}
		if !manager.now().UTC().Before(entry.acceptUntil) {
			continue
		}
		if len(token) > entry.maxTokenBytes {
			return UserClaims{}, errors.New("auth: compatibility token is too large")
		}
		claims, err = entry.verifier.VerifyToken(ctx, token)
		if err == nil {
			verified, _, _, identityErr := canonicalUserClaims(claims)
			if identityErr != nil {
				return UserClaims{}, errors.New("auth: compatibility verifier returned invalid identity")
			}
			return verified, nil
		}
		if !errors.Is(err, ErrTokenFormatNotRecognized) {
			return UserClaims{}, errors.New("auth: compatibility token verification failed")
		}
	}
	return UserClaims{}, errors.New("auth: invalid token")
}

func (manager *TokenManager) sign(claims TokenClaims) (string, error) {
	header, err := json.Marshal(tokenHeader{Algorithm: "HS256", Type: "JWT", KeyID: manager.activeKeyID})
	if err != nil {
		return "", errors.New("auth: token encoding failed")
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", errors.New("auth: token encoding failed")
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(header)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := encodedHeader + "." + encodedPayload
	mac := hmac.New(sha256.New, manager.keys[manager.activeKeyID])
	_, _ = mac.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (manager *TokenManager) validateClaims(claims TokenClaims) error {
	now := manager.now().UTC()
	subject, subjectID, numeric, identityErr := canonicalIdentityID(claims.Subject)
	if identityErr != nil || subject != claims.Subject || claims.UserID < 0 ||
		(numeric && (claims.UserID != subjectID || claims.UserIDString != "")) ||
		(!numeric && (claims.UserID != 0 || claims.UserIDString != subject)) || claims.JWTID == "" ||
		claims.Issuer != manager.issuer || claims.Audience != manager.audience ||
		claims.ExpiresAt <= 0 || claims.NotBefore <= 0 || claims.IssuedAt <= 0 {
		return errors.New("invalid required claims")
	}
	if time.Unix(claims.IssuedAt, 0).After(now.Add(manager.clockSkew)) || claims.NotBefore < claims.IssuedAt {
		return errors.New("token timestamps are invalid")
	}
	if !now.Before(time.Unix(claims.ExpiresAt, 0).Add(manager.clockSkew)) || now.Add(manager.clockSkew).Before(time.Unix(claims.NotBefore, 0)) {
		return errors.New("token is expired or not active")
	}
	lifetimeSeconds := claims.ExpiresAt - claims.IssuedAt
	if lifetimeSeconds <= 0 || lifetimeSeconds > int64(manager.maxTTL/time.Second) {
		return errors.New("token lifetime is invalid")
	}
	return nil
}

func defaultTokenManager(secret string) (*TokenManager, error) {
	return NewTokenManager(TokenConfig{
		Issuer: defaultTokenIssuer, Audience: defaultTokenAudience, ActiveKeyID: defaultTokenKeyID,
		Keys: map[string][]byte{defaultTokenKeyID: []byte(secret)}, MaxTTL: 24 * time.Hour, ClockSkew: time.Minute,
	})
}

// GenerateToken creates a verified-compatible token. Exactly one explicit TTL is required.
func GenerateToken(userID int64, secret string, ttl ...time.Duration) string {
	if len(ttl) != 1 {
		return ""
	}
	manager, err := defaultTokenManager(secret)
	if err != nil {
		return ""
	}
	token, err := manager.Issue(userID, "", ttl[0])
	if err != nil {
		return ""
	}
	return token
}

// VerifyToken verifies a token produced by GenerateToken.
func VerifyToken(token, secret string) (*TokenClaims, error) {
	manager, err := defaultTokenManager(secret)
	if err != nil {
		return nil, errors.New("auth: invalid token configuration")
	}
	return manager.Verify(token)
}

// GenerateJWT creates a standards-compliant signed JWT with an explicit TTL.
func GenerateJWT(claims UserClaims, secret string, ttl ...time.Duration) string {
	if len(ttl) != 1 {
		return ""
	}
	manager, err := defaultTokenManager(secret)
	if err != nil {
		return ""
	}
	token, err := manager.IssueUser(claims, ttl[0])
	if err != nil {
		return ""
	}
	return token
}

// RequireJWT verifies JWT bearer tokens using the compatibility single-key manager.
func RequireJWT(secret string) gpp.HandlerFunc { return Authenticate(secret) }

// GeneratePASETO is disabled because GoPlusPlus does not ship a complete PASETO implementation.
// Deprecated: use TokenManager.IssueUser.
func GeneratePASETO(UserClaims, string) string { return "" }

// RequirePASETO fails closed instead of accepting the former placeholder token format.
// Deprecated: use AuthenticateWithManager.
func RequirePASETO(string) gpp.HandlerFunc {
	return func(*gpp.Context) error { return gpp.ErrUnauthorized("PASETO support is unavailable") }
}

// Authenticate verifies bearer JWTs using the compatibility single-key manager.
func Authenticate(secret string) gpp.HandlerFunc {
	manager, err := defaultTokenManager(secret)
	if err != nil {
		return func(*gpp.Context) error { return gpp.ErrUnauthorized("Invalid or expired bearer token") }
	}
	return AuthenticateWithManager(manager)
}

// AuthenticateWithManager installs identity only after complete token verification.
func AuthenticateWithManager(manager *TokenManager) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		clearVerifiedIdentity(c)
		token, ok := bearerToken(c)
		if !ok || manager == nil {
			return gpp.ErrUnauthorized("Missing or invalid bearer token")
		}
		claims, err := manager.verifyCompatible(c.Request.Context(), token)
		if err != nil {
			return gpp.ErrUnauthorized("Invalid or expired bearer token")
		}
		if err := installVerifiedIdentity(c, claims); err != nil {
			return gpp.ErrUnauthorized("Invalid or expired bearer token")
		}
		return c.Next()
	}
}

// OptionalAuthenticateWithManager installs identity if a valid bearer token is present, but permits unauthenticated requests to proceed.
func OptionalAuthenticateWithManager(manager *TokenManager) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		clearVerifiedIdentity(c)
		token, ok := bearerToken(c)
		if ok && manager != nil {
			claims, err := manager.verifyCompatible(c.Request.Context(), token)
			if err == nil {
				_ = installVerifiedIdentity(c, claims)
			}
		}
		return c.Next()
	}
}


// UniversalAuth accepts a valid process-local session or compatibility JWT.
// New applications should prefer UniversalAuthWithManager.
func UniversalAuth(secret string, sessions *RedisSessionManager) gpp.HandlerFunc {
	manager, _ := defaultTokenManager(secret)
	return UniversalAuthWithManager(manager, sessions)
}

// UniversalAuthWithManager accepts a verified session or JWT using explicit token policy.
func UniversalAuthWithManager(manager *TokenManager, sessions *RedisSessionManager) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		clearVerifiedIdentity(c)
		if sessions != nil {
			if claims, ok := sessions.claimsForRequest(c); ok {
				if installVerifiedIdentity(c, *claims) == nil {
					return c.Next()
				}
			}
		}
		token, ok := bearerToken(c)
		if !ok || manager == nil {
			return gpp.ErrUnauthorized("Valid session or bearer token required")
		}
		claims, err := manager.verifyCompatible(c.Request.Context(), token)
		if err != nil {
			return gpp.ErrUnauthorized("Valid session or bearer token required")
		}
		if err := installVerifiedIdentity(c, claims); err != nil {
			return gpp.ErrUnauthorized("Valid session or bearer token required")
		}
		return c.Next()
	}
}

func (claims *TokenClaims) userClaims() *UserClaims {
	return &UserClaims{
		ID: claims.Subject, Subject: claims.Subject, Email: claims.Email,
		Roles: append([]string(nil), claims.Roles...), Attributes: cloneStringMap(claims.Attributes), TenantID: claims.TenantID,
	}
}

func bearerToken(c *gpp.Context) (string, bool) {
	if c == nil || c.Request == nil {
		return "", false
	}
	values := c.Request.Header.Values("Authorization")
	if len(values) != 1 {
		return "", false
	}
	header := values[0]
	if len(header) <= len("Bearer ") || len(header) > len("Bearer ")+maxEncodedTokenBytes || !strings.HasPrefix(header, "Bearer ") {
		return "", false
	}
	token := header[len("Bearer "):]
	if strings.ContainsAny(token, " \t\r\n") {
		return "", false
	}
	return token, true
}

func randomTokenIdentifier() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", errors.New("auth: secure random generation failed")
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func strictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON data")
	}
	return nil
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
