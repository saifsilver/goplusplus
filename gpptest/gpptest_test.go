package gpptest_test

import (
	"net/http"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/gpptest"
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
