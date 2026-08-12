package middleware

import (
	gpp "github.com/saifsilver/goplusplus"
)

// SecurityConfig holds configuration options for HTTP security headers.
type SecurityConfig struct {
	XFrameOptions         string
	XContentTypeOptions   string
	XXSSProtection        string
	HSTSMaxAge            string
	ContentSecurityPolicy string
	ReferrerPolicy        string
}

// DefaultSecurityConfig returns OWASP recommended default security headers.
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		XFrameOptions:         "DENY",
		XContentTypeOptions:   "nosniff",
		XXSSProtection:        "1; mode=block",
		HSTSMaxAge:            "max-age=31536000; includeSubDomains",
		ContentSecurityPolicy: "default-src 'self'",
		ReferrerPolicy:        "no-referrer-when-downgrade",
	}
}

// Security returns middleware that applies HTTP security headers to all responses.
func Security(configs ...SecurityConfig) gpp.HandlerFunc {
	cfg := DefaultSecurityConfig()
	if len(configs) > 0 {
		userCfg := configs[0]
		if userCfg.XFrameOptions != "" {
			cfg.XFrameOptions = userCfg.XFrameOptions
		}
		if userCfg.XContentTypeOptions != "" {
			cfg.XContentTypeOptions = userCfg.XContentTypeOptions
		}
		if userCfg.XXSSProtection != "" {
			cfg.XXSSProtection = userCfg.XXSSProtection
		}
		if userCfg.HSTSMaxAge != "" {
			cfg.HSTSMaxAge = userCfg.HSTSMaxAge
		}
		if userCfg.ContentSecurityPolicy != "" {
			cfg.ContentSecurityPolicy = userCfg.ContentSecurityPolicy
		}
		if userCfg.ReferrerPolicy != "" {
			cfg.ReferrerPolicy = userCfg.ReferrerPolicy
		}
	}

	return func(c *gpp.Context) error {
		if cfg.XFrameOptions != "" {
			c.SetHeader("X-Frame-Options", cfg.XFrameOptions)
		}
		if cfg.XContentTypeOptions != "" {
			c.SetHeader("X-Content-Type-Options", cfg.XContentTypeOptions)
		}
		if cfg.XXSSProtection != "" {
			c.SetHeader("X-XSS-Protection", cfg.XXSSProtection)
		}
		if cfg.HSTSMaxAge != "" && c.Request.TLS != nil {
			c.SetHeader("Strict-Transport-Security", cfg.HSTSMaxAge)
		}
		if cfg.ContentSecurityPolicy != "" {
			c.SetHeader("Content-Security-Policy", cfg.ContentSecurityPolicy)
		}
		if cfg.ReferrerPolicy != "" {
			c.SetHeader("Referrer-Policy", cfg.ReferrerPolicy)
		}
		return c.Next()
	}
}
