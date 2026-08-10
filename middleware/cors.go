package middleware

import (
	"net/http"
	"strings"

	"github.com/saifsilver/goplusplus"
)

// CORSConfig defines configuration settings for Cross-Origin Resource Sharing.
type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           string
}

// DefaultCORSConfig returns standard permissive default CORS settings.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		MaxAge:       "86400",
	}
}

// CORS returns middleware that manages CORS headers and preflight OPTIONS requests.
func CORS(configs ...CORSConfig) gpp.HandlerFunc {
	cfg := DefaultCORSConfig()
	if len(configs) > 0 {
		cfg = configs[0]
	}

	methodsStr := strings.Join(cfg.AllowMethods, ", ")
	headersStr := strings.Join(cfg.AllowHeaders, ", ")
	exposeStr := strings.Join(cfg.ExposeHeaders, ", ")

	return func(c *gpp.Context) error {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if len(cfg.AllowOrigins) == 1 && cfg.AllowOrigins[0] == "*" {
				c.SetHeader("Access-Control-Allow-Origin", "*")
			} else {
				for _, allowed := range cfg.AllowOrigins {
					if allowed == "*" || allowed == origin {
						c.SetHeader("Access-Control-Allow-Origin", origin)
						break
					}
				}
			}

			if cfg.AllowCredentials {
				c.SetHeader("Access-Control-Allow-Credentials", "true")
			}

			if exposeStr != "" {
				c.SetHeader("Access-Control-Expose-Headers", exposeStr)
			}
		}

		// Handle OPTIONS preflight requests
		if c.Request.Method == http.MethodOptions {
			c.SetHeader("Access-Control-Allow-Methods", methodsStr)
			c.SetHeader("Access-Control-Allow-Headers", headersStr)
			if cfg.MaxAge != "" {
				c.SetHeader("Access-Control-Max-Age", cfg.MaxAge)
			}
			c.Abort()
			c.Status(http.StatusNoContent)
			return nil
		}

		return c.Next()
	}
}
