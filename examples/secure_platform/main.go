package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/auth"
	"github.com/saifsilver/goplusplus/middleware"
)

func main() {
	// 1. Initialize secure session and JWT policy from deployment secrets.
	signingKey := os.Getenv("GPP_JWT_SIGNING_KEY")
	if len(signingKey) < 32 {
		panic("GPP_JWT_SIGNING_KEY must contain at least 32 bytes")
	}
	sessionMgr, err := auth.NewSessionManager(auth.SessionConfig{
		TTL: 8 * time.Hour, SameSite: http.SameSiteLaxMode,
	})
	if err != nil {
		panic(err)
	}
	tokens, err := auth.NewTokenManager(auth.TokenConfig{
		Issuer: "goplusplus-secure-platform", Audience: "secure-platform-api",
		ActiveKeyID: "primary", Keys: map[string][]byte{"primary": []byte(signingKey)}, MaxTTL: 24 * time.Hour,
	})
	if err != nil {
		panic(err)
	}

	// 2. Initialize goplusplus Application Engine
	app := gpp.New()

	app.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
	)

	// Web Auth Endpoint (creates a server-side session and HTTP-Only cookie)
	app.POST("/api/v1/auth/web-login", func(c *gpp.Context) error {
		claims := auth.UserClaims{
			ID:    "1001",
			Email: "user@webapp.com",
			Roles: []string{"user"},
		}
		if sessionMgr.CreateSession(c, claims) == "" {
			return gpp.ErrInternal("session creation failed")
		}
		return c.JSON(http.StatusOK, gpp.H{
			"status": "authenticated",
			"method": "HTTP-Only SameSite Cookie",
		})
	})

	// Mobile Auth Endpoint (issues a verified JWT with an explicit TTL).
	app.POST("/api/v1/auth/mobile-login", func(c *gpp.Context) error {
		claims := auth.UserClaims{
			ID:    "1002",
			Email: "mobile@app.com",
			Roles: []string{"user"},
		}
		jwtToken, err := tokens.IssueUser(claims, 15*time.Minute)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, gpp.H{
			"status":    "authenticated",
			"jwt_token": jwtToken,
			"method":    "Authorization: Bearer <token>",
		})
	})

	// Unified Secure API Group (Accepts EITHER Web Session Cookie OR Mobile Bearer Token in 1 call!)
	secureAPI := app.Group("/api/v1/secure")
	secureAPI.Use(auth.UniversalAuthWithManager(tokens, sessionMgr))

	secureAPI.GET("/profile", func(c *gpp.Context) error {
		user, _ := c.Get("user")
		return c.JSON(http.StatusOK, gpp.H{
			"status": "authenticated_access_granted",
			"user":   user,
		})
	})

	fmt.Println("🚀 Starting goplusplus Universal Security Engine on http://localhost:8080")
	fmt.Println("   • Web Login Endpoint:    POST http://localhost:8080/api/v1/auth/web-login")
	fmt.Println("   • Mobile Login Endpoint: POST http://localhost:8080/api/v1/auth/mobile-login")
	fmt.Println("   • Secure Profile API:    GET  http://localhost:8080/api/v1/secure/profile")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
