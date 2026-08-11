package gpp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/dbcore"
)

type TestItem struct {
	ID   int64  `db:"id" json:"id"`
	Name string `db:"name" json:"name"`
}

func TestContextEnhancementsSuite(t *testing.T) {
	app := gpp.New()

	app.Use(func(c *gpp.Context) error {
		c.Set("user_id", int64(999))
		return c.Next()
	})

	app.GET("/users/:id", func(c *gpp.Context) error {
		if c.ParamInt64("id") != 42 {
			t.Errorf("ParamInt64 failed: got %d", c.ParamInt64("id"))
		}
		if c.ParamInt("id") != 42 {
			t.Errorf("ParamInt failed: got %d", c.ParamInt("id"))
		}
		if c.UserID() != 999 {
			t.Errorf("UserID failed: got %d", c.UserID())
		}
		id, err := c.RequireUserID()
		if err != nil || id != 999 {
			t.Errorf("RequireUserID failed: %v, %d", err, id)
		}
		return c.OK(gpp.H{"status": "ok"})
	})

	app.GET("/responses", func(c *gpp.Context) error {
		return c.Created(gpp.H{"created": true})
	})
	app.GET("/accepted", func(c *gpp.Context) error {
		return c.Accepted(gpp.H{"task": "queued"})
	})
	app.GET("/nocontent", func(c *gpp.Context) error {
		return c.NoContent()
	})

	app.GET("/errors", func(c *gpp.Context) error {
		switch c.Query("type") {
		case "bad":
			return c.BadRequest("bad input")
		case "unauth":
			return c.Unauthorized("unauthorized")
		case "forbid":
			return c.Forbidden("forbidden")
		case "notfound":
			return c.NotFound("not found")
		case "conflict":
			return c.Conflict("conflict")
		default:
			return c.InternalError("internal failure")
		}
	})

	// 1. OK Test
	req1 := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", w1.Code)
	}

	// 2. Created Test
	req2 := httptest.NewRequest(http.MethodGet, "/responses", nil)
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)
	if w2.Code != http.StatusCreated {
		t.Errorf("expected 201 Created, got %d", w2.Code)
	}

	// 3. Accepted Test
	req3 := httptest.NewRequest(http.MethodGet, "/accepted", nil)
	w3 := httptest.NewRecorder()
	app.ServeHTTP(w3, req3)
	if w3.Code != http.StatusAccepted {
		t.Errorf("expected 202 Accepted, got %d", w3.Code)
	}

	// 4. NoContent Test
	req4 := httptest.NewRequest(http.MethodGet, "/nocontent", nil)
	w4 := httptest.NewRecorder()
	app.ServeHTTP(w4, req4)
	if w4.Code != http.StatusNoContent {
		t.Errorf("expected 204 No Content, got %d", w4.Code)
	}

	// 5. Error Shortcuts Test
	for _, path := range []string{
		"/errors?type=bad",
		"/errors?type=unauth",
		"/errors?type=forbid",
		"/errors?type=notfound",
		"/errors?type=conflict",
		"/errors?type=internal",
	} {
		reqErr := httptest.NewRequest(http.MethodGet, path, nil)
		wErr := httptest.NewRecorder()
		app.ServeHTTP(wErr, reqErr)
		if wErr.Code == 0 {
			t.Errorf("error shortcut returned status 0")
		}
	}
}

func TestBindResourceSuite(t *testing.T) {
	ctx := context.Background()
	client, err := dbcore.NewClient(ctx, dbcore.Config{RWDSN: ":memory:"})
	if err != nil {
		t.Fatalf("failed creating memory db: %v", err)
	}
	defer client.Close()

	repo := dbcore.NewRepository[TestItem](client, "test_items")

	app := gpp.New()
	v1 := app.Group("/api/v1")
	gpp.BindResource(v1, "/items", repo)

	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/items", nil)
	wList := httptest.NewRecorder()
	app.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Errorf("BindResource GET list failed: %d", wList.Code)
	}
}
