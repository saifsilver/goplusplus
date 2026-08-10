package i18n

import (
	"fmt"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"strings"

	"github.com/saifsilver/goplusplus"
)

type Bundle struct {
	defaultLang string
	translations map[string]map[string]string
}

// NewBundle creates a new i18n translation bundle.
func NewBundle(defaultLang string) *Bundle {
	return &Bundle{
		defaultLang: defaultLang,
		translations: map[string]map[string]string{
			"en": {
				"welcome": "Welcome to goplusplus Enterprise Platform!",
			},
			"es": {
				"welcome": "¡Bienvenido a la plataforma empresarial goplusplus!",
			},
			"fr": {
				"welcome": "Bienvenue sur la plateforme goplusplus Enterprise !",
			},
		},
	}
}

// Middleware creates a language detection middleware parsing Accept-Language or ?lang= parameter.
func (b *Bundle) Middleware() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		lang := c.Query("lang")
		if lang == "" {
			accept := c.GetHeader("Accept-Language")
			if accept != "" {
				parts := strings.Split(accept, ",")
				if len(parts) > 0 {
					lang = strings.TrimSpace(strings.Split(parts[0], ";")[0])
				}
			}
		}
		if lang == "" {
			lang = b.defaultLang
		}
		c.Set("lang", lang)
		return c.Next()
	}
}

// Translate retrieves a localized string for a key.
func (b *Bundle) Translate(lang, key string) string {
	if dict, ok := b.translations[lang]; ok {
		if val, ok := dict[key]; ok {
			return val
		}
	}
	if dict, ok := b.translations[b.defaultLang]; ok {
		if val, ok := dict[key]; ok {
			return val
		}
	}
	return key
}

// FormatCurrency formats a float64 monetary amount according to currency code rules.
func FormatCurrency(amount float64, currencyCode string) string {
	p := message.NewPrinter(language.English)
	switch strings.ToUpper(currencyCode) {
	case "EUR":
		return fmt.Sprintf("€%.2f", amount)
	case "GBP":
		return fmt.Sprintf("£%.2f", amount)
	case "JPY":
		return p.Sprintf("¥%.0f", amount)
	default:
		return fmt.Sprintf("$%.2f", amount)
	}
}
