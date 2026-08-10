package gpp_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saifsilver/goplusplus"
)

type samplePayload struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func TestContextJSONBinding(t *testing.T) {
	app := gpp.New()

	var received samplePayload
	app.POST("/users", func(c *gpp.Context) error {
		if err := c.BindJSON(&received); err != nil {
			return c.JSON(http.StatusBadRequest, gpp.H{"error": err.Error()})
		}
		return c.JSON(http.StatusCreated, received)
	})

	payload := samplePayload{Name: "Alice", Email: "alice@example.com"}
	bodyBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", w.Code)
	}
	if received.Name != "Alice" || received.Email != "alice@example.com" {
		t.Fatalf("unexpected received payload: %+v", received)
	}
}

func TestContextQueryParams(t *testing.T) {
	app := gpp.New()

	var queryVal, defaultVal string
	app.GET("/search", func(c *gpp.Context) error {
		queryVal = c.Query("q")
		defaultVal = c.QueryDefault("page", "1")
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/search?q=golang", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if queryVal != "golang" {
		t.Fatalf("expected query q='golang', got '%s'", queryVal)
	}
	if defaultVal != "1" {
		t.Fatalf("expected default page='1', got '%s'", defaultVal)
	}
}

func TestContextStorage(t *testing.T) {
	app := gpp.New()

	app.Use(func(c *gpp.Context) error {
		c.Set("user_id", 100)
		return c.Next()
	})

	var retrievedID int
	app.GET("/me", func(c *gpp.Context) error {
		val, _ := c.Get("user_id")
		retrievedID = val.(int)
		return c.JSON(http.StatusOK, gpp.H{"id": retrievedID})
	})

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if retrievedID != 100 {
		t.Fatalf("expected context stored user_id=100, got %d", retrievedID)
	}
}
