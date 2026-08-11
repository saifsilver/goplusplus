package auth

import (
	"bytes"
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

// TokenClaims contains verified JWT claims.
type TokenClaims struct {
	Subject    string            `json:"sub"`
	UserID     int64             `json:"user_id"`
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

type tokenHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
	KeyID     string `json:"kid"`
}

// TokenConfig defines immutable JWT verification and key-rotation policy.
type TokenConfig struct {
	Issuer      string
	Audience    string
	ActiveKeyID string
	Keys        map[string][]byte
	MaxTTL      time.Duration
	ClockSkew   time.Duration
	Now         func() time.Time
}

// TokenManager issues and verifies HS256 JWTs using explicit key IDs.
type TokenManager struct {
	issuer      string
	audience    string
	activeKeyID string
	keys        map[string][]byte
	maxTTL      time.Duration
	clockSkew   time.Duration
	now         func() time.Time
}

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
	return &TokenManager{
		issuer: config.Issuer, audience: config.Audience, activeKeyID: config.ActiveKeyID,
		keys: keys, maxTTL: config.MaxTTL, clockSkew: config.ClockSkew, now: config.Now,
	}, nil
}

// Issue creates a signed JWT for a positive numeric user ID and explicit bounded TTL.
func (manager *TokenManager) Issue(userID int64, email string, ttl time.Duration) (string, error) {
	return manager.IssueUser(UserClaims{ID: strconv.FormatInt(userID, 10), Email: email}, ttl)
}

// IssueUser creates a signed JWT containing trusted authorization claims.
func (manager *TokenManager) IssueUser(user UserClaims, ttl time.Duration) (string, error) {
	userID, err := parsePositiveUserID(user.ID)
	if err != nil || ttl < time.Second || ttl > manager.maxTTL {
		return "", errors.New("auth: user ID and explicit token TTL must be valid")
	}
	now := manager.now().UTC()
	jti, err := randomTokenIdentifier()
	if err != nil {
		return "", err
	}
	claims := TokenClaims{
		Subject: strconv.FormatInt(userID, 10), UserID: userID, Email: user.Email,
		Roles: append([]string(nil), user.Roles...), Attributes: cloneStringMap(user.Attributes), TenantID: user.TenantID,
		Issuer: manager.issuer, Audience: manager.audience, IssuedAt: now.Unix(), NotBefore: now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(), JWTID: jti,
	}
	return manager.sign(claims)
}

// Verify authenticates a JWT signature and all required registered claims.
func (manager *TokenManager) Verify(token string) (*TokenClaims, error) {
	if len(token) == 0 || len(token) > maxEncodedTokenBytes {
		return nil, errors.New("auth: invalid token")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("auth: invalid token")
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
	if claims.UserID <= 0 || claims.Subject != strconv.FormatInt(claims.UserID, 10) || claims.JWTID == "" ||
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
		return func(*gpp.Context) error { return gpp.ErrUnauthorized("Invalid authentication configuration") }
	}
	return AuthenticateWithManager(manager)
}

// AuthenticateWithManager installs identity only after complete token verification.
func AuthenticateWithManager(manager *TokenManager) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		token, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok || manager == nil {
			return gpp.ErrUnauthorized("Missing or invalid bearer token")
		}
		claims, err := manager.Verify(token)
		if err != nil {
			return gpp.ErrUnauthorized("Invalid or expired bearer token")
		}
		c.Set("user", claims.userClaims())
		return c.Next()
	}
}

func UniversalAuth(secret string, sessions *RedisSessionManager) gpp.HandlerFunc {
	manager, _ := defaultTokenManager(secret)
	return UniversalAuthWithManager(manager, sessions)
}

// UniversalAuthWithManager accepts a verified session or JWT using explicit token policy.
func UniversalAuthWithManager(manager *TokenManager, sessions *RedisSessionManager) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		if sessions != nil {
			if claims, ok := sessions.claimsForRequest(c); ok {
				c.Set("user", claims)
				return c.Next()
			}
		}
		token, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok || manager == nil {
			return gpp.ErrUnauthorized("Valid session or bearer token required")
		}
		claims, err := manager.Verify(token)
		if err != nil {
			return gpp.ErrUnauthorized("Valid session or bearer token required")
		}
		c.Set("user", claims.userClaims())
		return c.Next()
	}
}

func (claims *TokenClaims) userClaims() *UserClaims {
	return &UserClaims{
		ID: strconv.FormatInt(claims.UserID, 10), Email: claims.Email,
		Roles: append([]string(nil), claims.Roles...), Attributes: cloneStringMap(claims.Attributes), TenantID: claims.TenantID,
	}
}

func bearerToken(header string) (string, bool) {
	if !strings.HasPrefix(header, "Bearer ") || strings.Count(header, " ") != 1 {
		return "", false
	}
	token := strings.TrimPrefix(header, "Bearer ")
	return token, token != ""
}

func parsePositiveUserID(value string) (int64, error) {
	value = strings.TrimPrefix(value, "usr_")
	userID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || userID <= 0 {
		return 0, errors.New("auth: user ID must be positive")
	}
	return userID, nil
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
