// Package user demonstrates a self-contained modular-monolith user domain.
package user

import (
	"net/http"

	"github.com/saifsilver/goplusplus"
)

// UserModule represents the self-contained User domain module.
type UserModule struct{}

// New creates a new UserModule instance.
func New() *UserModule {
	return &UserModule{}
}

// Name returns the identifier of the module.
func (m *UserModule) Name() string {
	return "UserModule"
}

// Register registers all User domain endpoints onto the supplied RouterGroup.
func (m *UserModule) Register(group *gpp.RouterGroup) {
	group.GET("/profile/:id", m.getProfile)
	group.POST("/login", m.login)
}

func (m *UserModule) getProfile(c *gpp.Context) error {
	id := c.Param("id")
	return c.JSON(http.StatusOK, gpp.H{
		"module":   m.Name(),
		"user_id":  id,
		"username": "alex_dev",
		"email":    "alex@example.com",
		"status":   "active",
	})
}

func (m *UserModule) login(c *gpp.Context) error {
	type loginReq struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	var req loginReq
	if err := c.BindJSON(&req); err != nil {
		return gpp.NewHTTPError(http.StatusBadRequest, "Invalid login credentials body")
	}

	return c.JSON(http.StatusOK, gpp.H{
		"module":  m.Name(),
		"message": "Login successful",
		"token":   "jwt_token_sample_12345",
	})
}
