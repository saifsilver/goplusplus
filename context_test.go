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
		// Test GetInt64 with type coercion
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

		// Test GetInt
		if c.GetInt("int_val") != 42 {
			t.Errorf("GetInt(int_val) = %d; want 42", c.GetInt("int_val"))
		}
		if c.GetInt("int64_val") != 1001 {
			t.Errorf("GetInt(int64_val) = %d; want 1001", c.GetInt("int64_val"))
		}

		// Test GetFloat64
		if c.GetFloat64("float_val") != 99.9 {
			t.Errorf("GetFloat64(float_val) = %f; want 99.9", c.GetFloat64("float_val"))
		}

		// Test GetBool
		if !c.GetBool("bool_val") {
			t.Errorf("GetBool(bool_val) = false; want true")
		}
		if !c.GetBool("str_bool") {
			t.Errorf("GetBool(str_bool) = false; want true")
		}

		// Test GetAny & Value (single return value for type assertion)
		if id, ok := c.GetAny("int64_val").(int64); !ok || id != 1001 {
			t.Errorf("GetAny(int64_val) assertion failed, got %v", c.GetAny("int64_val"))
		}
		if id, ok := c.Value("int64_val").(int64); !ok || id != 1001 {
			t.Errorf("Value(int64_val) assertion failed, got %v", c.Value("int64_val"))
		}

		// Test GetString
		if c.GetString("str_num") != "12345" {
			t.Errorf("GetString(str_num) = %s; want '12345'", c.GetString("str_num"))
		}
		if c.GetString("int_val") != "42" {
			t.Errorf("GetString(int_val) = %s; want '42'", c.GetString("int_val"))
		}

		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestGenericContextGetters(t *testing.T) {
	app := gpp.New()

	type CustomUser struct {
		Name string
	}

	app.Use(func(c *gpp.Context) error {
		c.Set("user", CustomUser{Name: "Alice"})
		c.Set("user_id", int64(777))
		return c.Next()
	})

	app.GET("/generic", func(c *gpp.Context) error {
		user, ok := gpp.GetAs[CustomUser](c, "user")
		if !ok || user.Name != "Alice" {
			t.Errorf("GetAs[CustomUser] failed: %+v", user)
		}

		id := gpp.GetOrDefault[int64](c, "user_id", 0)
		if id != 777 {
			t.Errorf("GetOrDefault[int64] = %d; want 777", id)
		}

		defaultVal := gpp.GetOrDefault[string](c, "missing", "default_str")
		if defaultVal != "default_str" {
			t.Errorf("GetOrDefault[string] = %s; want 'default_str'", defaultVal)
		}

		mustVal := gpp.MustGetAs[int64](c, "user_id")
		if mustVal != 777 {
			t.Errorf("MustGetAs[int64] = %d; want 777", mustVal)
		}

		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/generic", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}
