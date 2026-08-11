package gpp_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	var pageInt int
	var activeBool bool

	app.GET("/search", func(c *gpp.Context) error {
		queryVal = c.Query("q")
		defaultVal = c.QueryDefault("page", "1")
		pageInt = c.QueryInt("page", 1)
		activeBool = c.QueryBool("active", true)
		return c.String(http.StatusOK, "%s", "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/search?q=golang&page=5&active=true", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if queryVal != "golang" {
		t.Fatalf("expected query q='golang', got '%s'", queryVal)
	}
	if defaultVal != "5" {
		t.Fatalf("expected default page='5', got '%s'", defaultVal)
	}
	if pageInt != 5 {
		t.Fatalf("expected pageInt=5, got %d", pageInt)
	}
	if !activeBool {
		t.Fatalf("expected activeBool=true")
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
		mustVal := c.MustGet("user_id").(int)
		if mustVal != 100 {
			t.Errorf("MustGet failed")
		}
		return c.JSON(http.StatusOK, gpp.H{"id": retrievedID})
	})

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if retrievedID != 100 {
		t.Fatalf("expected context stored user_id=100, got %d", retrievedID)
	}
}

func TestTypedContextGetters(t *testing.T) {
	app := gpp.New()

	app.Use(func(c *gpp.Context) error {
		c.Set("int_val", 42)
		c.Set("int64_val", int64(1001))
		c.Set("float_val", 99.9)
		c.Set("str_num", "12345")
		c.Set("bool_val", true)
		c.Set("str_bool", "true")
		return c.Next()
	})

	app.GET("/test", func(c *gpp.Context) error {
		if c.GetInt64("int64_val") != 1001 {
			t.Errorf("GetInt64(int64_val) = %d; want 1001", c.GetInt64("int64_val"))
		}
		if c.GetInt64("int_val") != 42 {
			t.Errorf("GetInt64(int_val) = %d; want 42", c.GetInt64("int_val"))
		}
		if c.GetInt64("str_num") != 12345 {
			t.Errorf("GetInt64(str_num) = %d; want 12345", c.GetInt64("str_num"))
		}
		if c.GetInt64("missing_key") != 0 {
			t.Errorf("GetInt64(missing_key) = %d; want 0", c.GetInt64("missing_key"))
		}

		if c.GetInt("int_val") != 42 {
			t.Errorf("GetInt(int_val) = %d; want 42", c.GetInt("int_val"))
		}
		if c.GetInt("int64_val") != 1001 {
			t.Errorf("GetInt(int64_val) = %d; want 1001", c.GetInt("int64_val"))
		}

		if c.GetFloat64("float_val") != 99.9 {
			t.Errorf("GetFloat64(float_val) = %f; want 99.9", c.GetFloat64("float_val"))
		}

		if !c.GetBool("bool_val") {
			t.Errorf("GetBool(bool_val) = false; want true")
		}
		if !c.GetBool("str_bool") {
			t.Errorf("GetBool(str_bool) = false; want true")
		}

		if id, ok := c.GetAny("int64_val").(int64); !ok || id != 1001 {
			t.Errorf("GetAny(int64_val) assertion failed, got %v", c.GetAny("int64_val"))
		}
		if id, ok := c.Value("int64_val").(int64); !ok || id != 1001 {
			t.Errorf("Value(int64_val) assertion failed, got %v", c.Value("int64_val"))
		}

		if c.GetString("str_num") != "12345" {
			t.Errorf("GetString(str_num) = %s; want '12345'", c.GetString("str_num"))
		}
		if c.GetString("int_val") != "42" {
			t.Errorf("GetString(int_val) = %s; want '42'", c.GetString("int_val"))
		}

		return c.String(http.StatusOK, "%s", "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestContextResponseFormattersAndPagination(t *testing.T) {
	app := gpp.New()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "static.txt")
	_ = os.WriteFile(filePath, []byte("static_file_content"), 0644)

	app.GET("/users/:id", func(c *gpp.Context) error {
		if c.PathValue("id") != "42" {
			t.Errorf("PathValue failed")
		}
		if c.IsCancelled() {
			t.Errorf("IsCancelled returned true unexpectedly")
		}
		return c.Paginate(http.StatusOK, []string{"user1", "user2"}, 1, 10, 2)
	})

	app.GET("/cursor", func(c *gpp.Context) error {
		return c.PaginateCursor(http.StatusOK, []string{"item1"}, "next_cursor_123", true, 10)
	})

	app.GET("/html", func(c *gpp.Context) error {
		return c.HTML(http.StatusOK, "<h1>Hello</h1>")
	})

	app.GET("/file", func(c *gpp.Context) error {
		return c.File(filePath)
	})

	app.GET("/abort", func(c *gpp.Context) error {
		return c.AbortWithStatusJSON(http.StatusForbidden, gpp.H{"error": "blocked"})
	})

	// Paginate test
	req1 := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Errorf("Paginate status 200 failed: %d", w1.Code)
	}

	// PaginateCursor test
	req2 := httptest.NewRequest(http.MethodGet, "/cursor", nil)
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("PaginateCursor status 200 failed: %d", w2.Code)
	}

	// HTML test
	req3 := httptest.NewRequest(http.MethodGet, "/html", nil)
	w3 := httptest.NewRecorder()
	app.ServeHTTP(w3, req3)
	if w3.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("HTML content type failed: %s", w3.Header().Get("Content-Type"))
	}

	// File test
	req4 := httptest.NewRequest(http.MethodGet, "/file", nil)
	w4 := httptest.NewRecorder()
	app.ServeHTTP(w4, req4)
	if w4.Body.String() != "static_file_content" {
		t.Errorf("File serve failed: %s", w4.Body.String())
	}

	// AbortWithStatusJSON test
	req5 := httptest.NewRequest(http.MethodGet, "/abort", nil)
	w5 := httptest.NewRecorder()
	app.ServeHTTP(w5, req5)
	if w5.Code != http.StatusForbidden {
		t.Errorf("AbortWithStatusJSON failed: %d", w5.Code)
	}
}
