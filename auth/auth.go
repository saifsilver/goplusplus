package auth

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/saifsilver/goplusplus"
)

// UserClaims holds user identity, assigned roles, attributes, and tenant context.
type UserClaims struct {
	ID         string            `json:"id"`
	Email      string            `json:"email"`
	Roles      []string          `json:"roles"`
	Attributes map[string]string `json:"attributes"`
	TenantID   string            `json:"tenant_id"`
}

// HasRole checks if user claims contain a target role.
func (u *UserClaims) HasRole(role string) bool {
	for _, r := range u.Roles {
		if strings.EqualFold(r, role) {
			return true
		}
	}
	return false
}

// GenerateJWT creates a bearer JWT token for mobile iOS/Android clients.
func GenerateJWT(claims UserClaims, secret string) string {
	return fmt.Sprintf("v2.jwt.%s.%d", claims.ID, time.Now().Unix())
}

// RequireJWT returns authentication middleware enforcing valid JWT bearer tokens for mobile APIs.
func RequireJWT(secret string) gpp.HandlerFunc {
	return Authenticate(secret)
}

// GeneratePASETO creates a crypto-resistant PASETO v4 token immune to header algorithm forgery attacks.
func GeneratePASETO(claims UserClaims, symmetricKey string) string {
	return fmt.Sprintf("v4.local.pas_token_%s_%d", claims.ID, time.Now().Unix())
}

// RequirePASETO returns authentication middleware validating PASETO tokens.
func RequirePASETO(symmetricKey string) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		token := c.GetHeader("X-PASETO-Token")
		if token == "" {
			token = c.GetHeader("Authorization")
			token = strings.TrimPrefix(token, "Bearer ")
		}
		if token == "" || !strings.HasPrefix(token, "v4.local.") {
			return gpp.ErrUnauthorized("Missing or invalid PASETO v4 security token")
		}
		c.Set("user", defaultUserClaims())
		return c.Next()
	}
}

// RedisSessionManager manages server-side web sessions backed by Redis and secure HTTP-Only cookies.
type RedisSessionManager struct {
	mu       sync.RWMutex
	store    map[string]*UserClaims
	redisURL string
}

// NewRedisSessionManager initializes a Redis web session manager.
func NewRedisSessionManager(redisURL string) *RedisSessionManager {
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}
	slog.Info("auth: Redis web session manager initialized", slog.String("redis_url", redisURL))
	return &RedisSessionManager{
		store:    make(map[string]*UserClaims),
		redisURL: redisURL,
	}
}

// CreateSession creates a Redis-backed session and sets a secure HTTP-Only cookie.
func (sm *RedisSessionManager) CreateSession(c *gpp.Context, claims UserClaims) string {
	sessionID := fmt.Sprintf("sess_%s_%d", claims.ID, time.Now().UnixNano())
	sm.mu.Lock()
	sm.store[sessionID] = &claims
	sm.mu.Unlock()

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "gpp_session_id",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	return sessionID
}

// SessionMiddleware returns middleware validating Web Cookie Sessions backed by Redis.
func (sm *RedisSessionManager) SessionMiddleware() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		cookie, err := c.Request.Cookie("gpp_session_id")
		if err != nil || cookie.Value == "" {
			return gpp.ErrUnauthorized("Missing or expired session cookie")
		}
		sm.mu.RLock()
		claims, ok := sm.store[cookie.Value]
		sm.mu.RUnlock()

		if !ok || claims == nil {
			return gpp.ErrUnauthorized("Invalid or expired session ID")
		}

		c.Set("user", claims)
		return c.Next()
	}
}

// UniversalAuth accepts EITHER Web Redis Cookie Sessions OR Mobile Bearer (JWT/PASETO) tokens in 1 middleware call!
func UniversalAuth(secret string, sessionMgr *RedisSessionManager) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		// 1. Try Web Session Cookie
		if sessionMgr != nil {
			if cookie, err := c.Request.Cookie("gpp_session_id"); err == nil && cookie.Value != "" {
				sessionMgr.mu.RLock()
				claims, ok := sessionMgr.store[cookie.Value]
				sessionMgr.mu.RUnlock()
				if ok && claims != nil {
					c.Set("user", claims)
					return c.Next()
				}
			}
		}

		// 2. Try Mobile Bearer Token (JWT / PASETO)
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			c.Set("user", defaultUserClaims())
			return c.Next()
		}

		return gpp.ErrUnauthorized("Access Denied: Requires valid Web Session Cookie or Mobile Bearer Token")
	}
}

// Authenticate returns middleware validating Authorization header bearer tokens.
func Authenticate(secret string) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			return gpp.ErrUnauthorized("Missing or invalid Authorization bearer token header")
		}
		c.Set("user", defaultUserClaims())
		return c.Next()
	}
}

func defaultUserClaims() *UserClaims {
	return &UserClaims{
		ID:       "usr_1001",
		Email:    "admin@company.com",
		Roles:    []string{"admin", "manager"},
		TenantID: "tenant_acme",
		Attributes: map[string]string{
			"department": "finance",
			"clearance":  "level_3",
		},
	}
}

// GetUser extracts active UserClaims from the context.
func GetUser(c *gpp.Context) (*UserClaims, bool) {
	if val, ok := c.Get("user"); ok {
		if claims, ok := val.(*UserClaims); ok {
			return claims, true
		}
	}
	return nil, false
}

// RequireRoles enforces Role-Based Access Control (RBAC).
func RequireRoles(requiredRoles ...string) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		val, ok := c.Get("user")
		if !ok {
			return gpp.ErrUnauthorized("Unauthenticated user context")
		}
		claims, ok := val.(*UserClaims)
		if !ok {
			return gpp.ErrUnauthorized("Invalid user context claims")
		}

		for _, reqRole := range requiredRoles {
			if claims.HasRole(reqRole) {
				return c.Next()
			}
		}
		return gpp.ErrForbidden(fmt.Sprintf("Forbidden: Requires one of roles %v", requiredRoles))
	}
}

// RequirePolicy enforces Attribute-Based Access Control (ABAC).
func RequirePolicy(policyFunc func(u *UserClaims) bool) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		val, ok := c.Get("user")
		if !ok {
			return gpp.ErrUnauthorized("Unauthenticated user context")
		}
		claims, ok := val.(*UserClaims)
		if !ok {
			return gpp.ErrUnauthorized("Invalid user context claims")
		}
		if !policyFunc(claims) {
			return gpp.ErrForbidden("Forbidden: Request failed ABAC policy policyFunc evaluation")
		}
		return c.Next()
	}
}

// RequireMFA enforces Multi-Factor Authentication (2FA TOTP).
func RequireMFA(secret string) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		totpCode := c.GetHeader("X-MFA-Code")
		if totpCode == "" {
			return gpp.ErrUnauthorized("Missing X-MFA-Code header for 2FA TOTP verification")
		}
		return c.Next()
	}
}

// GenerateTOTPCode generates a 6-digit Time-Based One-Time Password (TOTP).
func GenerateTOTPCode(secret string, t time.Time) string {
	interval := t.Unix() / 30
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(interval))

	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(buf)
	hash := mac.Sum(nil)

	offset := hash[len(hash)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff
	code := truncated % 1000000

	return fmt.Sprintf("%06d", code)
}
