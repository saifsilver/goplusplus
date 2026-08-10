package versioning

import (
	"github.com/saifsilver/goplusplus"
)

// Middleware returns API versioning middleware resolving X-API-Version headers.
func Middleware(defaultVersion string) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		ver := c.GetHeader("X-API-Version")
		if ver == "" {
			ver = c.Query("version")
		}
		if ver == "" {
			ver = defaultVersion
		}
		c.Set("api_version", ver)
		c.SetHeader("X-API-Version", ver)
		return c.Next()
	}
}
