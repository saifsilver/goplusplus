package gpptest

import "testing"

func FuzzParseProblemDetails(f *testing.F) {
	f.Add([]byte(`{"type":"about:blank","title":"Bad Request","status":400,"detail":"invalid"}`))
	f.Add([]byte(`{`))
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 1<<20 {
			return
		}
		_, _ = ParseProblemDetails(body)
	})
}
