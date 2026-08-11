// Package authentication demonstrates application-owned registration, login,
// account persistence, and hash migration using framework-owned auth mechanics.
package authentication

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/auth"
	"github.com/saifsilver/goplusplus/id"
)

type Application struct {
	db        *sql.DB
	tokens    *auth.TokenManager
	passwords *auth.PasswordPolicy
}

type account struct {
	ID           string
	Email        string
	PasswordHash string
}

type credentials struct {
	Email    string `json:"email" validate:"required,email,max=320"`
	Password string `json:"password" validate:"required,min=12,max=1024"`
}

func New(db *sql.DB, tokens *auth.TokenManager, passwords *auth.PasswordPolicy) (*Application, error) {
	if db == nil || tokens == nil || passwords == nil {
		return nil, errors.New("authentication example: database, token manager, and password policy are required")
	}
	return &Application{db: db, tokens: tokens, passwords: passwords}, nil
}

func (application *Application) RegisterRoutes(app *gpp.Engine) {
	app.POST("/auth/register", application.register)
	app.POST("/auth/login", application.login)
	app.GET("/account", auth.AuthenticateWithManager(application.tokens), application.account)
}

func (application *Application) register(c *gpp.Context) error {
	var request credentials
	if err := c.BindAndValidate(&request); err != nil {
		return err
	}
	email := normalizeEmail(request.Email)
	hash, err := application.passwords.Hash(request.Password)
	if err != nil {
		return gpp.ErrBadRequest("Password does not satisfy the configured policy")
	}
	accountID := id.NewUUIDv7()
	if _, err := application.db.ExecContext(c.Request.Context(),
		"INSERT INTO accounts (id, email, password_hash) VALUES ($1, $2, $3)", accountID, email, hash); err != nil {
		// Map database-specific uniqueness violations to the application's chosen
		// duplicate-email response before returning from production code.
		return gpp.ErrInternal("Account registration failed")
	}
	return c.Created(gpp.H{"id": accountID, "email": email})
}

func (application *Application) login(c *gpp.Context) error {
	var request credentials
	if err := c.BindAndValidate(&request); err != nil {
		return err
	}
	stored, found, err := application.findAccount(c.Request.Context(), normalizeEmail(request.Email))
	if err != nil {
		return gpp.ErrInternal("Authentication failed")
	}
	if !found {
		application.passwords.VerifyMissing(request.Password)
		return invalidCredentials()
	}
	result := application.passwords.Verify(request.Password, stored.PasswordHash)
	if result == auth.PasswordInvalid {
		return invalidCredentials()
	}
	if result == auth.PasswordValidNeedsRehash {
		if err := application.replaceHash(c.Request.Context(), stored, request.Password); err != nil {
			// This application fails closed when migration persistence fails. An
			// application may choose a different documented operational policy.
			return gpp.ErrInternal("Authentication failed")
		}
	}
	token, err := application.tokens.IssueUser(auth.UserClaims{ID: stored.ID, Email: stored.Email}, 15*time.Minute)
	if err != nil {
		return gpp.ErrInternal("Authentication failed")
	}
	return c.OK(gpp.H{"access_token": token, "token_type": "Bearer", "expires_in": 900})
}

func (application *Application) account(c *gpp.Context) error {
	subject, err := c.RequireUserSubject()
	if err != nil {
		return err
	}
	identity, ok := auth.GetUser(c)
	if !ok {
		return gpp.ErrUnauthorized("Authentication required")
	}
	return c.JSON(http.StatusOK, gpp.H{"id": subject, "email": identity.Email})
}

func (application *Application) findAccount(ctx context.Context, email string) (account, bool, error) {
	var stored account
	err := application.db.QueryRowContext(ctx,
		"SELECT id, email, password_hash FROM accounts WHERE email = $1", email,
	).Scan(&stored.ID, &stored.Email, &stored.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return account{}, false, nil
	}
	return stored, err == nil, err
}

func (application *Application) replaceHash(ctx context.Context, stored account, password string) error {
	replacement, err := application.passwords.Hash(password)
	if err != nil {
		return err
	}
	result, err := application.db.ExecContext(ctx,
		"UPDATE accounts SET password_hash = $1 WHERE id = $2 AND password_hash = $3",
		replacement, stored.ID, stored.PasswordHash,
	)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return errors.New("authentication example: password hash compare-and-swap did not update exactly one account")
	}
	return nil
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func invalidCredentials() error {
	return gpp.ErrUnauthorized("Invalid email or password")
}
