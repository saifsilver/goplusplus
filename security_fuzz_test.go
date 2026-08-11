package gpp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func FuzzValidationTags(f *testing.F) {
	for _, tag := range []string{
		"required,min=6,max=128", "omitempty,email", "dive,required", "unsupported", "min=bad", "", ",",
	} {
		f.Add(tag)
	}
	f.Fuzz(func(t *testing.T, tag string) {
		if len(tag) > 512 {
			t.Skip()
		}
		_, _ = parseValidationRules(tag)
	})
}

func FuzzJSONBinding(f *testing.F) {
	for _, body := range []string{
		`{"name":"valid"}`, `{`, ``, `{"unknown":true}`, `{"name":"one"}{"name":"two"}`,
	} {
		f.Add(body)
	}
	f.Fuzz(func(t *testing.T, body string) {
		if len(body) > int(DefaultMaxJSONBodyBytes)+1 {
			t.Skip()
		}
		request := httptest.NewRequest(http.MethodPost, "/fuzz", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		context := &Context{Request: request, Writer: httptest.NewRecorder()}
		var target struct {
			Name string `json:"name"`
		}
		_ = bindJSON(context, &target)
	})
}
