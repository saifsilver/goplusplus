package i18n

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
)

func TestBundleMessagesFallbackAndInterpolation(t *testing.T) {
	bundle := NewBundle("en")
	if got := bundle.Translate("es-MX", "welcome"); got != "¡Bienvenido a la plataforma empresarial goplusplus!" {
		t.Fatalf("Spanish parent fallback = %q", got)
	}
	if got := bundle.Translate("missing", "welcome"); got != "Welcome to goplusplus Enterprise Platform!" {
		t.Fatalf("default fallback = %q", got)
	}
	if got := bundle.Translate("en", "missing %s", "message"); got != "missing message" {
		t.Fatalf("missing key = %q", got)
	}

	if err := bundle.AddMessages("de-DE", map[string]string{"hello %s": "Hallo %s"}); err != nil {
		t.Fatal(err)
	}
	if err := bundle.AddMessages("de-DE", map[string]string{"bye": "Tschüss"}); err != nil {
		t.Fatal(err)
	}
	if got := bundle.Translate("de-AT", "hello %s", "Ada"); got != "Hallo Ada" {
		t.Fatalf("interpolation = %q", got)
	}
	if got := bundle.Translate("de-DE", "welcome"); got != "Welcome to goplusplus Enterprise Platform!" {
		t.Fatalf("per-key fallback = %q", got)
	}
	if err := bundle.AddMessages("not_a_tag", nil); !errors.Is(err, ErrInvalidLanguage) {
		t.Fatalf("invalid tag error = %v", err)
	}
	for _, invalid := range []string{"***", "und", ""} {
		if err := bundle.AddMessages(invalid, nil); !errors.Is(err, ErrInvalidLanguage) {
			t.Fatalf("invalid tag %q error = %v", invalid, err)
		}
	}

	locales := bundle.SupportedLocales()
	gotTags := make([]string, len(locales))
	for i, locale := range locales {
		gotTags[i] = locale.Tag
	}
	if !slices.Equal(gotTags, []string{"en", "es", "fr", "de-DE"}) {
		t.Fatalf("supported tags = %v", gotTags)
	}
}

func TestBundlePluralRules(t *testing.T) {
	bundle := NewBundle("en")
	if err := bundle.AddPlural("en", "files", PluralForms{}); !errors.Is(err, ErrMissingPlural) {
		t.Fatalf("missing other error = %v", err)
	}
	if err := bundle.AddPlural("not_a_tag", "files", PluralForms{Other: "%d files"}); !errors.Is(err, ErrInvalidLanguage) {
		t.Fatalf("invalid language error = %v", err)
	}
	forms := PluralForms{One: "one file", Other: "%d files"}
	if err := bundle.AddPlural("en", "files", forms); err != nil {
		t.Fatal(err)
	}
	if got := bundle.Translate("en", "files", 1); got != "one file" {
		t.Fatalf("singular = %q", got)
	}
	if got := bundle.Translate("en", "files", 5); got != "5 files" {
		t.Fatalf("plural = %q", got)
	}
	if err := bundle.AddPlural("en", "invalid", PluralForms{Zero: "zero", Other: "other"}); !errors.Is(err, ErrInvalidPlural) {
		t.Fatalf("unsupported plural category error = %v", err)
	}
	all := pluralCases(PluralForms{Zero: "0", One: "1", Two: "2", Few: "few", Many: "many", Other: "other"})
	if len(all) != 12 {
		t.Fatalf("plural cases length = %d", len(all))
	}
}

func TestLanguageResolutionAndDirection(t *testing.T) {
	bundle := NewBundle("invalid_tag")
	if got := bundle.Resolve().Tag; got != "en" {
		t.Fatalf("invalid default resolved to %q", got)
	}
	if got := bundle.Resolve("bad_tag", "fr-CA"); got.Tag != "fr" || got.Language != "fr" || got.Direction != LeftToRight {
		t.Fatalf("resolved locale = %+v", got)
	}
	if got := bundle.ResolveHeader("en;q=0.2, fr-FR;q=0.9"); got.Tag != "fr" {
		t.Fatalf("weighted header = %+v", got)
	}
	if got := bundle.ResolveHeader("***"); got.Tag != "en" {
		t.Fatalf("malformed header = %+v", got)
	}
	if err := bundle.AddMessages("ar-EG", map[string]string{"welcome": "أهلاً"}); err != nil {
		t.Fatal(err)
	}
	rtl := bundle.Resolve("ar-SA")
	if rtl.Tag != "ar-EG" || rtl.Language != "ar" || rtl.Region != "EG" || rtl.Direction != RightToLeft {
		t.Fatalf("RTL locale = %+v", rtl)
	}
}

func TestMiddlewareNegotiatesAndEmitsMetadata(t *testing.T) {
	bundle := NewBundle("en")
	app := gpp.New()
	app.Use(func(c *gpp.Context) error {
		c.SetHeader("Vary", "Origin, Accept-Language")
		return c.Next()
	}, bundle.Middleware())

	var detected string
	var locale Locale
	app.GET("/hello", func(c *gpp.Context) error {
		detected = c.GetString(ContextLanguageKey)
		locale, _ = gpp.GetAs[Locale](c, ContextLocaleKey)
		return c.String(http.StatusOK, "%s", bundle.Translate(detected, "welcome"))
	})

	request := httptest.NewRequest(http.MethodGet, "/hello?lang=es-MX", nil)
	request.Header.Set("Accept-Language", "fr;q=1")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if detected != "es" || locale.Tag != "es" {
		t.Fatalf("query override detected=%q locale=%+v", detected, locale)
	}
	if got := response.Header().Get("Content-Language"); got != "es" {
		t.Fatalf("Content-Language = %q", got)
	}
	if got := response.Header().Values("Vary"); !slices.Equal(got, []string{"Origin, Accept-Language"}) {
		t.Fatalf("Vary = %v", got)
	}

	request = httptest.NewRequest(http.MethodGet, "/hello", nil)
	request.Header.Set("Accept-Language", "fr-FR,fr;q=0.9,en;q=0.1")
	response = httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if detected != "fr" || response.Body.String() != "Bienvenue sur la plateforme goplusplus Enterprise !" {
		t.Fatalf("header negotiation detected=%q body=%q", detected, response.Body.String())
	}
}

func TestAddVaryAddsMissingValue(t *testing.T) {
	context := &gpp.Context{Writer: httptest.NewRecorder()}
	addVary(context, "Accept-Language")
	if got := context.Writer.Header().Get("Vary"); got != "Accept-Language" {
		t.Fatalf("Vary = %q", got)
	}
}
