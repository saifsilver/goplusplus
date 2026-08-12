package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/dbcore"
)

// User represents the standard identity model for built-in authentication.
type User struct {
	ID           int64     `json:"id" db:"id,pk,auto_id"`
	Email        string    `json:"email" db:"email,unique" validate:"required,email"`
	PasswordHash string    `json:"-" db:"password_hash"`
	Name         string    `json:"name" db:"name"`
	Role         string    `json:"role" db:"role"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// Config configures the built-in authentication module.
type Config struct {
	PathPrefix string
	Secret     string
	Issuer     string
	MaxTTL     time.Duration
}

type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=6"`
	Name     string `json:"name"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type Response struct {
	Token string `json:"token"`
	User  *User  `json:"user"`
}

// Enable mounts built-in zero-code authentication endpoints (/api/auth/register, /api/auth/login, /api/auth/me).
func Enable(engine *gpp.Engine, configs ...Config) (*TokenManager, error) {
	cfg := Config{
		PathPrefix: "/api/auth",
		Issuer:     "goplusplus-app",
		MaxTTL:     24 * time.Hour,
	}
	if len(configs) > 0 {
		if configs[0].PathPrefix != "" {
			cfg.PathPrefix = configs[0].PathPrefix
		}
		if configs[0].Secret != "" {
			cfg.Secret = configs[0].Secret
		}
		if configs[0].Issuer != "" {
			cfg.Issuer = configs[0].Issuer
		}
		if configs[0].MaxTTL > 0 {
			cfg.MaxTTL = configs[0].MaxTTL
		}
	}

	if cfg.Secret == "" {
		cfg.Secret = "gpp-default-secure-auth-secret-key-32b!"
	}

	tokens, err := NewTokenManager(TokenConfig{
		Issuer:      cfg.Issuer,
		Audience:    cfg.Issuer,
		ActiveKeyID: "primary",
		Keys:        map[string][]byte{"primary": []byte(cfg.Secret)},
		MaxTTL:      cfg.MaxTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("auth: token manager creation failed: %w", err)
	}

	engine.SetAuthManager(tokens)

	// Apply authentication context extractor globally
	engine.Use(OptionalAuthenticateWithManager(tokens))

	client := engine.DBClient()
	if client != nil {
		ctx := context.Background()
		if err := dbcore.AutoMigrateModel(ctx, client, &User{}); err != nil {
			return nil, fmt.Errorf("auth: failed to migrate User table: %w", err)
		}
	}

	group := engine.Group(cfg.PathPrefix)

	group.POST("/register", func(c *gpp.Context) error {
		db := engine.DBClient()
		if db == nil {
			return c.InternalError("Database not initialized")
		}
		var req RegisterRequest
		if err := c.BindAndValidate(&req); err != nil {
			return err
		}

		userORM := dbcore.NewORM[User](db)
		existing, err := userORM.Where("email", strings.ToLower(req.Email)).Find(c.Request.Context())
		if err == nil && len(existing) > 0 {
			return c.Conflict("An account with this email already exists")
		}

		hash := HashPassword(req.Password, cfg.Secret)
		if hash == "" {
			return c.InternalError("Password hashing failed")
		}

		now := time.Now().UTC().Truncate(time.Millisecond)
		user := User{
			Email:        strings.ToLower(req.Email),
			PasswordHash: hash,
			Name:         req.Name,
			Role:         "user",
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		if err := userORM.Save(c.Request.Context(), &user); err != nil {
			return gpp.NewInternalError("auth.register", err, gpp.WithErrorCategory("database"))
		}

		token, err := tokens.IssueUser(UserClaims{
			ID:      fmt.Sprintf("%d", user.ID),
			Subject: fmt.Sprintf("%d", user.ID),
			Email:   user.Email,
			Roles:   []string{user.Role},
		}, cfg.MaxTTL)
		if err != nil {
			return c.InternalError("Token generation failed")
		}

		return c.Created(Response{Token: token, User: &user})
	})

	group.POST("/login", func(c *gpp.Context) error {
		db := engine.DBClient()
		if db == nil {
			return c.InternalError("Database not initialized")
		}
		var req LoginRequest
		if err := c.BindAndValidate(&req); err != nil {
			return err
		}

		userORM := dbcore.NewORM[User](db)
		users, err := userORM.Where("email", strings.ToLower(req.Email)).Find(c.Request.Context())
		if err != nil || len(users) == 0 {
			return c.Unauthorized("Invalid email or password")
		}

		user := users[0]
		if !VerifyPassword(req.Password, cfg.Secret, user.PasswordHash) {
			return c.Unauthorized("Invalid email or password")
		}

		token, err := tokens.IssueUser(UserClaims{
			ID:      fmt.Sprintf("%d", user.ID),
			Subject: fmt.Sprintf("%d", user.ID),
			Email:   user.Email,
			Roles:   []string{user.Role},
		}, cfg.MaxTTL)
		if err != nil {
			return c.InternalError("Token generation failed")
		}

		return c.OK(Response{Token: token, User: &user})
	})

	group.GET("/me", func(c *gpp.Context) error {
		user, err := c.RequireUserSubject()
		if err != nil {
			return c.Unauthorized("Unauthorized")
		}

		db := engine.DBClient()
		if db == nil {
			return c.InternalError("Database not initialized")
		}

		userORM := dbcore.NewORM[User](db)
		item, err := userORM.FindByID(c.Request.Context(), user)
		if err != nil || item == nil {
			return c.NotFound("User profile not found")
		}

		return c.OK(gpp.H{"user": item})
	})

	group.POST("/logout", func(c *gpp.Context) error {
		return c.OK(gpp.H{"message": "Successfully logged out"})
	})

	return tokens, nil
}
