package auth

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	gpp "github.com/saifsilver/goplusplus"
)

// UserClaims holds verified user identity, roles, attributes, and tenant context.
type UserClaims struct {
	ID         string            `json:"id"`
	Email      string            `json:"email"`
	Roles      []string          `json:"roles"`
	Attributes map[string]string `json:"attributes"`
	TenantID   string            `json:"tenant_id"`
}

// HasRole checks if user claims contain a target role.
func (u *UserClaims) HasRole(role string) bool {
	for _, assigned := range u.Roles {
		if strings.EqualFold(assigned, role) {
			return true
		}
	}
	return false
}

// GetUser extracts verified UserClaims from the context.
func GetUser(c *gpp.Context) (*UserClaims, bool) {
	value, ok := c.Get("user")
	if !ok {
		return nil, false
	}
	claims, ok := value.(*UserClaims)
	return claims, ok && claims != nil
}

// RequireRoles enforces Role-Based Access Control (RBAC).
func RequireRoles(requiredRoles ...string) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		claims, ok := GetUser(c)
		if !ok {
			return gpp.ErrUnauthorized("Unauthenticated user context")
		}
		for _, required := range requiredRoles {
			if claims.HasRole(required) {
				return c.Next()
			}
		}
		return gpp.ErrForbidden(fmt.Sprintf("Forbidden: Requires one of roles %v", requiredRoles))
	}
}

// RequirePolicy enforces Attribute-Based Access Control (ABAC).
func RequirePolicy(policy func(u *UserClaims) bool) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		claims, ok := GetUser(c)
		if !ok {
			return gpp.ErrUnauthorized("Unauthenticated user context")
		}
		if policy == nil || !policy(claims) {
			return gpp.ErrForbidden("Forbidden: Request failed ABAC policy")
		}
		return c.Next()
	}
}

// RequireMFA verifies an RFC 6238-style six-digit TOTP within one clock step.
func RequireMFA(secret string) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		code := c.GetHeader("X-MFA-Code")
		if !VerifyTOTPCode(secret, code, time.Now()) {
			return gpp.ErrUnauthorized("Missing or invalid MFA code")
		}
		return c.Next()
	}
}

// GenerateTOTPCode generates a six-digit Time-Based One-Time Password.
func GenerateTOTPCode(secret string, at time.Time) string {
	interval := at.Unix() / 30
	buffer := make([]byte, 8)
	binary.BigEndian.PutUint64(buffer, uint64(interval))

	mac := hmac.New(sha1.New, []byte(secret))
	_, _ = mac.Write(buffer)
	hash := mac.Sum(nil)
	offset := hash[len(hash)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", truncated%1_000_000)
}

// VerifyTOTPCode compares a code in constant time and permits one step of clock skew.
func VerifyTOTPCode(secret, code string, at time.Time) bool {
	if len(secret) < 16 || len(code) != 6 {
		return false
	}
	for _, character := range code {
		if character < '0' || character > '9' {
			return false
		}
	}
	valid := 0
	for offset := -1; offset <= 1; offset++ {
		expected := GenerateTOTPCode(secret, at.Add(time.Duration(offset)*30*time.Second))
		valid |= subtle.ConstantTimeCompare([]byte(code), []byte(expected))
	}
	return valid == 1
}
