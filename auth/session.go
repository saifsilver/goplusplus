package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	gpp "github.com/saifsilver/goplusplus"
)

type sessionRecord struct {
	claims    UserClaims
	expiresAt time.Time
}

// SessionConfig defines cookie and server-side session lifetime policy.
type SessionConfig struct {
	TTL        time.Duration
	CookieName string
	// AllowInsecureHTTP disables Secure cookies for explicit local-development use only.
	AllowInsecureHTTP bool
	SameSite          http.SameSite
	Now               func() time.Time
	MaxSessions       int
}

// RedisSessionManager retains its public name for compatibility. Sessions are
// process-local unless applications replace this primitive with their shared store.
type RedisSessionManager struct {
	mu          sync.RWMutex
	store       map[string]sessionRecord
	ttl         time.Duration
	cookieName  string
	secure      bool
	sameSite    http.SameSite
	now         func() time.Time
	maxSessions int
}

func NewSessionManager(config SessionConfig) (*RedisSessionManager, error) {
	if config.TTL < 5*time.Minute || config.TTL > 30*24*time.Hour {
		return nil, errors.New("auth: session TTL is outside safe bounds")
	}
	if config.CookieName == "" {
		config.CookieName = "gpp_session_id"
	}
	if !isSafeCookieName(config.CookieName) {
		return nil, errors.New("auth: session cookie name is invalid")
	}
	if config.SameSite == 0 {
		config.SameSite = http.SameSiteLaxMode
	}
	if config.SameSite < http.SameSiteDefaultMode || config.SameSite > http.SameSiteNoneMode {
		return nil, errors.New("auth: SameSite policy is invalid")
	}
	if config.MaxSessions == 0 {
		config.MaxSessions = 100_000
	}
	if config.MaxSessions < 1 || config.MaxSessions > 1_000_000 {
		return nil, errors.New("auth: maximum session count is outside safe bounds")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &RedisSessionManager{
		store: make(map[string]sessionRecord), ttl: config.TTL, cookieName: config.CookieName,
		secure: !config.AllowInsecureHTTP, sameSite: config.SameSite, now: config.Now, maxSessions: config.MaxSessions,
	}, nil
}

// NewRedisSessionManager returns the secure compatibility session manager.
// The URL is retained for source compatibility and is not logged or treated as a credential.
func NewRedisSessionManager(string) *RedisSessionManager {
	manager, _ := NewSessionManager(SessionConfig{
		TTL: 8 * time.Hour, CookieName: "gpp_session_id", SameSite: http.SameSiteLaxMode,
	})
	return manager
}

// CreateSession rotates any presented session, creates a cryptographically random ID,
// and sets a bounded HttpOnly cookie.
func (manager *RedisSessionManager) CreateSession(c *gpp.Context, claims UserClaims) string {
	if manager == nil || c == nil {
		return ""
	}
	if c.Request != nil {
		if old, err := c.Request.Cookie(manager.cookieName); err == nil {
			manager.RevokeSession(old.Value)
		}
	}
	identifierBytes := make([]byte, 32)
	if _, err := rand.Read(identifierBytes); err != nil {
		return ""
	}
	identifier := base64.RawURLEncoding.EncodeToString(identifierBytes)
	now := manager.now().UTC()
	expiresAt := now.Add(manager.ttl)
	manager.mu.Lock()
	manager.purgeExpiredLocked(now)
	if len(manager.store) >= manager.maxSessions {
		manager.mu.Unlock()
		return ""
	}
	manager.store[identifier] = sessionRecord{claims: cloneUserClaims(claims), expiresAt: expiresAt}
	manager.mu.Unlock()

	http.SetCookie(c.Writer, &http.Cookie{
		Name: manager.cookieName, Value: identifier, Path: "/", HttpOnly: true,
		Secure: manager.secure, SameSite: manager.sameSite, Expires: expiresAt,
		MaxAge: int(manager.ttl / time.Second),
	})
	return identifier
}

func (manager *RedisSessionManager) RevokeSession(identifier string) {
	if manager == nil || identifier == "" {
		return
	}
	manager.mu.Lock()
	delete(manager.store, identifier)
	manager.mu.Unlock()
}

func (manager *RedisSessionManager) SessionMiddleware() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		claims, ok := manager.claimsForRequest(c)
		if !ok {
			return gpp.ErrUnauthorized("Missing, invalid, or expired session")
		}
		c.Set("user", claims)
		return c.Next()
	}
}

func (manager *RedisSessionManager) claimsForRequest(c *gpp.Context) (*UserClaims, bool) {
	if manager == nil || c == nil || c.Request == nil {
		return nil, false
	}
	cookie, err := c.Request.Cookie(manager.cookieName)
	if err != nil || cookie.Value == "" {
		return nil, false
	}
	manager.mu.RLock()
	record, ok := manager.store[cookie.Value]
	manager.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if !manager.now().UTC().Before(record.expiresAt) {
		manager.RevokeSession(cookie.Value)
		return nil, false
	}
	claims := cloneUserClaims(record.claims)
	return &claims, true
}

func cloneUserClaims(source UserClaims) UserClaims {
	return UserClaims{
		ID: source.ID, Email: source.Email, Roles: append([]string(nil), source.Roles...),
		Attributes: cloneStringMap(source.Attributes), TenantID: source.TenantID,
	}
}

func (manager *RedisSessionManager) purgeExpiredLocked(now time.Time) {
	for identifier, record := range manager.store {
		if !now.Before(record.expiresAt) {
			delete(manager.store, identifier)
		}
	}
}

func isSafeCookieName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for _, character := range name {
		if character <= 0x20 || character >= 0x7f || strings.ContainsRune("()<>@,;:\\\"/[]?={}", character) {
			return false
		}
	}
	return true
}
