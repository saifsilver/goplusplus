package main

import (
	"fmt"
	"net/http"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/auth"
	"github.com/saifsilver/goplusplus/middleware"
)

func main() {
	// 1. Initialize Redis Web Session Manager & Secrets
	sessionMgr := auth.NewRedisSessionManager("redis://localhost:6379/0")
	jwtSecret := "super_secret_jwt_key_991823"

	// 2. Initialize goplusplus Application Engine
	app := gpp.New()

	app.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
	)

	// Web Auth Endpoint (Creates Redis Session & sets HTTP-Only Cookie)
	app.POST("/api/v1/auth/web-login", func(c *gpp.Context) error {
		claims := auth.UserClaims{
			ID:    "usr_web_1",
			Email: "user@webapp.com",
			Roles: []string{"user"},
		}
		sessionID := sessionMgr.CreateSession(c, claims)
		return c.JSON(http.StatusOK, gpp.H{
			"status":     "authenticated",
			"session_id": sessionID,
			"method":     "HTTP-Only SameSite Cookie",
		})
	})

	// Mobile Auth Endpoint (Issues PASETO & JWT tokens)
	app.POST("/api/v1/auth/mobile-login", func(c *gpp.Context) error {
		claims := auth.UserClaims{
			ID:    "usr_mob_1",
			Email: "mobile@app.com",
			Roles: []string{"user"},
		}
		pasToken := auth.GeneratePASETO(claims, jwtSecret)
		jwtToken := auth.GenerateJWT(claims, jwtSecret)
		return c.JSON(http.StatusOK, gpp.H{
			"status":       "authenticated",
			"paseto_token": pasToken,
			"jwt_token":    jwtToken,
			"method":       "Authorization: Bearer <token>",
		})
	})

	// Unified Secure API Group (Accepts EITHER Web Session Cookie OR Mobile Bearer Token in 1 call!)
	secureAPI := app.Group("/api/v1/secure")
	secureAPI.Use(auth.UniversalAuth(jwtSecret, sessionMgr))

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
