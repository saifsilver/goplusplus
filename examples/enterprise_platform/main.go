package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/auth"
	"github.com/saifsilver/goplusplus/i18n"
	"github.com/saifsilver/goplusplus/middleware"
	"github.com/saifsilver/goplusplus/notify"
	"github.com/saifsilver/goplusplus/tenant"
)

func main() {
	i18nBundle := i18n.NewBundle("en")

	app := gpp.New()
	signingKey := os.Getenv("GPP_JWT_SIGNING_KEY")
	if len(signingKey) < 32 {
		panic("GPP_JWT_SIGNING_KEY must contain at least 32 bytes")
	}
	tokens, err := auth.NewTokenManager(auth.TokenConfig{
		Issuer: "goplusplus-enterprise-platform", Audience: "enterprise-platform-api",
		ActiveKeyID: "primary", Keys: map[string][]byte{"primary": []byte(signingKey)}, MaxTTL: 24 * time.Hour,
	})
	if err != nil {
		panic(err)
	}

	// Global System & Security Middleware
	app.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
		middleware.CORS(),
		i18nBundle.Middleware(), // Language detection (?lang=es or Accept-Language)
		tenant.Middleware(),     // Multi-Tenancy extraction (X-Tenant-ID or subdomain)
	)

	// Public Health Check Endpoint
	app.GET("/", func(c *gpp.Context) error {
		lang := c.GetString("lang")
		welcomeMsg := i18nBundle.Translate(lang, "welcome")
		tenantID := tenant.GetTenantID(c)

		return c.JSON(http.StatusOK, gpp.H{
			"status":           "online",
			"tenant_id":        tenantID,
			"lang":             lang,
			"welcome_message":  welcomeMsg,
			"sample_price_usd": i18n.FormatCurrency(199.99, "USD"),
			"sample_price_eur": i18n.FormatCurrency(199.99, "EUR"),
			"sample_price_gbp": i18n.FormatCurrency(199.99, "GBP"),
		})
	})

	// Protected Admin API (Requires JWT Auth + RBAC Admin Role + ABAC Finance Policy + MFA)
	adminGroup := app.Group("/api/admin")
	adminGroup.Use(
		auth.AuthenticateWithManager(tokens),
		auth.RequireRoles("admin"),
		auth.RequirePolicy(func(u *auth.UserClaims) bool {
			return u.Attributes["department"] == "finance"
		}),
	)

	adminGroup.POST("/disburse", func(c *gpp.Context) error {
		user, _ := auth.GetUser(c)
		tenantID := tenant.GetTenantID(c)

		// Dispatch Email and SMS Notification
		_ = notify.SendEmail(c.Request.Context(), notify.EmailMessage{
			To:      user.Email,
			Subject: "Funds Disbursed",
			Body:    "Disbursement executed successfully for tenant: " + tenantID,
		})

		_ = notify.SendSMS(c.Request.Context(), notify.SMSMessage{
			ToPhone: "+15550192831",
			Text:    "Alert: Finance disbursement approved by " + user.ID,
		})

		return c.JSON(http.StatusOK, gpp.H{
			"status":    "approved",
			"tenant_id": tenantID,
			"user_id":   user.ID,
			"roles":     user.Roles,
			"amount":    i18n.FormatCurrency(50000.00, "USD"),
		})
	})

	fmt.Println("🚀 Starting goplusplus Enterprise Platform Server on http://localhost:8080")
	fmt.Println("   • Public i18n & Tenant: http://localhost:8080/?lang=es")
	fmt.Println("   • Admin Protected Endpoint: POST http://localhost:8080/api/admin/disburse")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
