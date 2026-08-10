package versioning

import (
	"strings"
	"sync"
	"time"

	"github.com/saifsilver/goplusplus"
)

type DeprecationInfo struct {
	SunsetDate string
}

// Manager handles API version negotiation (Header, Query, Path) and RFC 8594 Deprecation/Sunset headers.
type Manager struct {
	mu             sync.RWMutex
	defaultVersion string
	deprecated     map[string]DeprecationInfo
}

// NewManager initializes an API Versioning Manager.
func NewManager(defaultVersion string) *Manager {
	if defaultVersion == "" {
		defaultVersion = "v1"
	}
	return &Manager{
		defaultVersion: defaultVersion,
		deprecated:     make(map[string]DeprecationInfo),
	}
}

// Deprecate marks an API version as deprecated with an RFC 8594 Sunset Date (e.g. "2027-01-01").
func (m *Manager) Deprecate(version string, sunsetDate string) {
	m.mu.Lock()
	m.deprecated[version] = DeprecationInfo{SunsetDate: sunsetDate}
	m.mu.Unlock()
}

// Middleware returns API Versioning Middleware performing header negotiation and deprecation warnings.
func (m *Manager) Middleware() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		ver := c.GetHeader("X-API-Version")
		if ver == "" {
			ver = c.Query("version")
		}
		if ver == "" {
			ver = c.Query("v")
		}
		if ver == "" {
			accept := c.GetHeader("Accept")
			if strings.Contains(accept, "vnd.") && strings.Contains(accept, "+json") {
				parts := strings.Split(accept, "vnd.")
				if len(parts) > 1 {
					ver = strings.Split(parts[1], "+")[0]
				}
			}
		}
		if ver == "" {
			ver = m.defaultVersion
		}

		c.Set("api_version", ver)
		c.SetHeader("X-API-Version", ver)

		// RFC 8594 Deprecation & Sunset headers
		m.mu.RLock()
		info, isDeprecated := m.deprecated[ver]
		m.mu.RUnlock()

		if isDeprecated {
			c.SetHeader("Deprecation", "@"+time.Now().Format(time.RFC1123))
			if info.SunsetDate != "" {
				c.SetHeader("Sunset", info.SunsetDate)
			}
		}

		return c.Next()
	}
}

// GetVersion retrieves the negotiated API version from the context.
func GetVersion(c *gpp.Context) string {
	if val, ok := c.Get("api_version"); ok {
		if ver, ok := val.(string); ok {
			return ver
		}
	}
	return "v1"
}
