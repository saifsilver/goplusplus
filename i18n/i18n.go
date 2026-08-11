// Package i18n provides language negotiation, translation catalogs, plural
// rules, and locale-aware formatting for goplusplus applications.
package i18n

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	gpp "github.com/saifsilver/goplusplus"
	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/message/catalog"
)

const (
	// ContextLanguageKey stores the canonical language tag selected by Middleware.
	ContextLanguageKey = "lang"
	// ContextLocaleKey stores the selected Locale value.
	ContextLocaleKey = "locale"
)

var (
	ErrInvalidLanguage = errors.New("i18n: invalid language tag")
	ErrMissingPlural   = errors.New("i18n: plural forms require an other form")
	ErrInvalidPlural   = errors.New("i18n: invalid plural forms for language")
)

// Direction is the natural writing direction of a locale.
type Direction string

const (
	LeftToRight Direction = "ltr"
	RightToLeft Direction = "rtl"
)

// Locale describes a resolved language and its presentation metadata.
type Locale struct {
	Tag       string
	Language  string
	Region    string
	Direction Direction
}

// PluralForms defines CLDR plural categories. Other is required. Supplying a
// category that the selected language does not define returns ErrInvalidPlural.
type PluralForms struct {
	Zero  string
	One   string
	Two   string
	Few   string
	Many  string
	Other string
}

// Bundle is a concurrency-safe translation catalog and locale registry.
// Registration is normally completed during application startup.
type Bundle struct {
	mu         sync.RWMutex
	defaultTag language.Tag
	supported  []language.Tag
	matcher    language.Matcher
	catalog    *catalog.Builder
	keys       map[string]map[string]struct{}
	profiles   map[string]DateTimeProfile
}

// NewBundle creates a bundle with English, Spanish, and French starter
// messages. Invalid default tags safely fall back to English.
func NewBundle(defaultLang string) *Bundle {
	defaultTag, err := language.Parse(defaultLang)
	if err != nil || defaultTag == language.Und {
		defaultTag = language.English
	}

	b := &Bundle{
		defaultTag: defaultTag,
		catalog:    catalog.NewBuilder(catalog.Fallback(defaultTag)),
		keys:       make(map[string]map[string]struct{}),
		profiles:   builtinDateTimeProfiles(),
	}
	b.addLanguageLocked(defaultTag, nil)
	_ = b.addMessagesLocked("en", map[string]string{
		"welcome": "Welcome to goplusplus Enterprise Platform!",
	})
	_ = b.addMessagesLocked("es", map[string]string{
		"welcome": "¡Bienvenido a la plataforma empresarial goplusplus!",
	})
	_ = b.addMessagesLocked("fr", map[string]string{
		"welcome": "Bienvenue sur la plateforme goplusplus Enterprise !",
	})
	return b
}

// AddMessages adds or replaces translated messages for a BCP 47 language tag.
func (b *Bundle) AddMessages(tag string, messages map[string]string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.addMessagesLocked(tag, messages)
}

func (b *Bundle) addMessagesLocked(rawTag string, messages map[string]string) error {
	tag, err := parseTag(rawTag)
	if err != nil {
		return err
	}
	b.addLanguageLocked(tag, messages)
	for key, translation := range messages {
		b.catalog.SetString(tag, key, translation)
		b.markKey(tag, key)
	}
	return nil
}

// AddPlural adds a translated message selected using the locale's CLDR plural
// rules. The count must be the first argument passed to Translate.
func (b *Bundle) AddPlural(rawTag, key string, forms PluralForms) error {
	if forms.Other == "" {
		return ErrMissingPlural
	}
	tag, err := parseTag(rawTag)
	if err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.addLanguageLocked(tag, nil)
	if err := b.catalog.Set(tag, key, plural.Selectf(1, "%d", pluralCases(forms)...)); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidPlural, err)
	}
	b.markKey(tag, key)
	return nil
}

func pluralCases(forms PluralForms) []any {
	cases := make([]any, 0, 12)
	appendForm := func(form plural.Form, value string) {
		if value != "" {
			cases = append(cases, form, value)
		}
	}
	appendForm(plural.Zero, forms.Zero)
	appendForm(plural.One, forms.One)
	appendForm(plural.Two, forms.Two)
	appendForm(plural.Few, forms.Few)
	appendForm(plural.Many, forms.Many)
	appendForm(plural.Other, forms.Other)
	return cases
}

func (b *Bundle) addLanguageLocked(tag language.Tag, _ map[string]string) {
	canonical := tag.String()
	for _, existing := range b.supported {
		if existing.String() == canonical {
			return
		}
	}
	b.supported = append(b.supported, tag)
	b.matcher = language.NewMatcher(b.supported)
	if b.keys[canonical] == nil {
		b.keys[canonical] = make(map[string]struct{})
	}
}

func (b *Bundle) markKey(tag language.Tag, key string) {
	b.keys[tag.String()][key] = struct{}{}
}

// Resolve selects the best supported locale from ordered BCP 47 preferences.
func (b *Bundle) Resolve(preferred ...string) Locale {
	tags := make([]language.Tag, 0, len(preferred))
	for _, raw := range preferred {
		if tag, err := language.Parse(strings.TrimSpace(raw)); err == nil {
			tags = append(tags, tag)
		}
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.matchLocked(tags)
}

// ResolveHeader applies Accept-Language quality weights and selects the best
// supported locale. Malformed or empty headers resolve to the default locale.
func (b *Bundle) ResolveHeader(header string) Locale {
	tags, _, err := language.ParseAcceptLanguage(header)
	if err != nil {
		tags = nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.matchLocked(tags)
}

// SupportedLocales returns a copy of the bundle's configured locales.
func (b *Bundle) SupportedLocales() []Locale {
	b.mu.RLock()
	defer b.mu.RUnlock()
	locales := make([]Locale, len(b.supported))
	for i, tag := range b.supported {
		locales[i] = localeFromTag(tag)
	}
	return locales
}

func (b *Bundle) matchLocked(tags []language.Tag) Locale {
	tag := b.defaultTag
	if len(tags) > 0 {
		_, index, _ := b.matcher.Match(tags...)
		tag = b.supported[index]
	}
	return localeFromTag(tag)
}

// Translate retrieves and interpolates a localized message. Missing messages
// fall back to the default language, then to the key itself.
func (b *Bundle) Translate(lang, key string, args ...any) string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	resolved := b.resolveLocked(lang)
	if !b.hasKey(resolved, key) {
		resolved = b.defaultTag
	}
	return message.NewPrinter(resolved, message.Catalog(b.catalog)).Sprintf(key, args...)
}

func (b *Bundle) resolveLocked(raw string) language.Tag {
	tag, err := language.Parse(raw)
	if err != nil {
		return b.defaultTag
	}
	_, index, _ := b.matcher.Match(tag)
	return b.supported[index]
}

func (b *Bundle) hasKey(tag language.Tag, key string) bool {
	_, ok := b.keys[tag.String()][key]
	return ok
}

// Middleware negotiates ?lang first, then Accept-Language. It stores both the
// canonical tag and Locale in context and emits cache-safe response metadata.
func (b *Bundle) Middleware() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		var locale Locale
		if requested := c.Query("lang"); requested != "" {
			locale = b.Resolve(requested)
		} else {
			locale = b.ResolveHeader(c.GetHeader("Accept-Language"))
		}
		c.Set(ContextLanguageKey, locale.Tag)
		c.Set(ContextLocaleKey, locale)
		c.SetHeader("Content-Language", locale.Tag)
		addVary(c, "Accept-Language")
		return c.Next()
	}
}

func addVary(c *gpp.Context, value string) {
	for _, existing := range c.Writer.Header().Values("Vary") {
		for _, item := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(item), value) {
				return
			}
		}
	}
	c.Writer.Header().Add("Vary", value)
}

func parseTag(raw string) (language.Tag, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsRune(raw, '_') {
		return language.Und, ErrInvalidLanguage
	}
	tag, err := language.Parse(raw)
	if err != nil || tag == language.Und {
		return language.Und, ErrInvalidLanguage
	}
	return tag, nil
}

func localeFromTag(tag language.Tag) Locale {
	base, _ := tag.Base()
	region, _ := tag.Region()
	return Locale{
		Tag:       tag.String(),
		Language:  base.String(),
		Region:    region.String(),
		Direction: directionFor(base.String()),
	}
}

func directionFor(base string) Direction {
	switch base {
	case "ar", "dv", "fa", "he", "ku", "ps", "sd", "ug", "ur", "yi":
		return RightToLeft
	default:
		return LeftToRight
	}
}
