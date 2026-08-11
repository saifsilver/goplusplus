package gpptest_test

import (
	"net/http"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/gpptest"
	"github.com/saifsilver/goplusplus/middleware"
)

func TestTesterHTTPRequests(t *testing.T) {
	app := gpp.New()

	app.GET("/users/:id", func(c *gpp.Context) error {
		return c.JSON(http.StatusOK, gpp.H{"id": c.Param("id"), "name": "Alex Dev"})
	})

	app.POST("/users", func(c *gpp.Context) error {
		return c.JSON(http.StatusCreated, gpp.H{"status": "created"})
	})

	tester := gpptest.New(t, app)

	tester.GET("/users/42").
		AssertStatus(http.StatusOK).
		AssertJSON("id", "42").
		AssertContains("Alex Dev")

	tester.POST("/users", map[string]string{"name": "Jane"}).
		AssertStatus(http.StatusCreated).
		AssertJSON("status", "created")
}

func TestTypedAndProblemHelpers(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.RequestID())
	app.GET("/nested", func(c *gpp.Context) error {
		return c.OK(gpp.H{"items": []gpp.H{{"id": 42}}})
	})
	app.GET("/problem", func(*gpp.Context) error {
		return gpp.ErrValidation([]gpp.FieldViolation{{Field: "email", Rule: "required", Message: "required"}})
	})
	tester := gpptest.New(t, app)
	type response struct {
		Items []struct {
			ID int `json:"id"`
		} `json:"items"`
	}
	decoded := gpptest.Decode[response](tester.GET("/nested").AssertStatus(200).AssertJSONPath("items.0.id", 42))
	if decoded.Items[0].ID != 42 {
		t.Fatalf("unexpected decode: %#v", decoded)
	}
	tester.GET("/problem").AssertProblem(http.StatusBadRequest, "https://goplusplus.dev/errors/validation").
		AssertViolation("email", "required").AssertContentType("application/problem+json").AssertRequestID()
}
