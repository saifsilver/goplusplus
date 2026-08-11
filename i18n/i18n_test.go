package i18n

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saifsilver/goplusplus"
)

func TestBundleTranslationAndMiddleware(t *testing.T) {
	bundle := NewBundle("en")

	if msg := bundle.Translate("es", "welcome"); msg != "¡Bienvenido a la plataforma empresarial goplusplus!" {
		t.Errorf("Translate(es) = %s", msg)
	}
	if msg := bundle.Translate("missing_lang", "welcome"); msg != "Welcome to goplusplus Enterprise Platform!" {
		t.Errorf("Translate fallback = %s", msg)
	}

	app := gpp.New()
	app.Use(bundle.Middleware())

	var detectedLang string
	app.GET("/hello", func(c *gpp.Context) error {
		detectedLang = c.GetString("lang")
		return c.String(http.StatusOK, "%s", bundle.Translate(detectedLang, "welcome"))
	})

	// Query param ?lang=es
	req1 := httptest.NewRequest(http.MethodGet, "/hello?lang=es", nil)
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)
	if detectedLang != "es" {
		t.Errorf("expected lang 'es', got '%s'", detectedLang)
	}

	// Accept-Language header
	req2 := httptest.NewRequest(http.MethodGet, "/hello", nil)
	req2.Header.Set("Accept-Language", "fr-FR,fr;q=0.9,en-US;q=0.8")
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)
	if detectedLang != "fr-FR" && detectedLang != "fr" {
		t.Errorf("expected lang 'fr-FR' or 'fr', got '%s'", detectedLang)
	}
}

func TestFormatCurrency(t *testing.T) {
	if s := FormatCurrency(100.50, "USD"); s != "$100.50" {
		t.Errorf("USD format = %s; want $100.50", s)
	}
	if s := FormatCurrency(100.50, "EUR"); s != "€100.50" {
		t.Errorf("EUR format = %s; want €100.50", s)
	}
	if s := FormatCurrency(100.50, "GBP"); s != "£100.50" {
		t.Errorf("GBP format = %s; want £100.50", s)
	}
}
