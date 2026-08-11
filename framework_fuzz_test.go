package gpp_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
)

func FuzzStrictQueryAndPagination(f *testing.F) {
	for _, seed := range []string{"", "0", "1", "-1", "abc", "9223372036854775808"} {
		f.Add(seed)
	}
	app := gpp.New()
	app.GET("/items", func(c *gpp.Context) error {
		_, _ = c.QueryOptionalInt64Strict("id")
		_, _ = (gpp.PaginationPolicy{Strict: true}).Parse(c)
		return c.NoContent()
	})
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 128 {
			return
		}
		target := "/items?id=" + url.QueryEscape(input) + "&page=" + url.QueryEscape(input) + "&limit=" + url.QueryEscape(input)
		app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, target, nil))
	})
}
