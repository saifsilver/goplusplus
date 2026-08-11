package i18n

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"
)

var (
	ErrInvalidDateTimeStyle   = errors.New("i18n: invalid date or time style")
	ErrInvalidDateTimeProfile = errors.New("i18n: invalid date-time profile")
)

// Style selects the amount of detail in localized date and time output.
type Style string

const (
	Short  Style = "short"
	Medium Style = "medium"
	Long   Style = "long"
	Full   Style = "full"
)

// DateTimeOptions configures combined output. Empty styles default to Medium.
type DateTimeOptions struct {
	DateStyle Style
	TimeStyle Style
}

// DateTimeProfile localizes Gregorian month/day names, field ordering, the
// date-time separator, and 12/24-hour display for an application locale.
type DateTimeProfile struct {
	Months         [12]string
	ShortMonths    [12]string
	Weekdays       [7]string
	ShortDateOrder string
	DayFirst       bool
	TwentyFourHour bool
	DateTimeJoiner string
}

// RegisterDateTimeProfile adds or replaces a profile for a BCP 47 tag.
func (b *Bundle) RegisterDateTimeProfile(rawTag string, profile DateTimeProfile) error {
	tag, err := parseTag(rawTag)
	if err != nil {
		return err
	}
	if err := validateProfile(profile); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.profiles[tag.String()] = profile
	return nil
}

// FormatDate formats a date in the requested locale and IANA time zone.
func (b *Bundle) FormatDate(value time.Time, locale, zone string, style Style) (string, error) {
	tag, profile := b.dateProfile(locale)
	localized, err := inLocation(value, zone)
	if err != nil {
		return "", err
	}
	return formatDate(localized, tag, profile, normalizeStyle(style))
}

// FormatTime formats a time in the requested locale and IANA time zone.
func (b *Bundle) FormatTime(value time.Time, locale, zone string, style Style) (string, error) {
	tag, profile := b.dateProfile(locale)
	localized, err := inLocation(value, zone)
	if err != nil {
		return "", err
	}
	return formatTime(localized, tag, profile, normalizeStyle(style))
}

// FormatDateTime formats a date and time after conversion to an IANA zone.
func (b *Bundle) FormatDateTime(value time.Time, locale, zone string, options DateTimeOptions) (string, error) {
	date, err := b.FormatDate(value, locale, zone, options.DateStyle)
	if err != nil {
		return "", err
	}
	clock, err := b.FormatTime(value, locale, zone, options.TimeStyle)
	if err != nil {
		return "", err
	}
	_, profile := b.dateProfile(locale)
	return date + profile.DateTimeJoiner + clock, nil
}

func (b *Bundle) dateProfile(rawLocale string) (language.Tag, DateTimeProfile) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	tag, err := parseTag(rawLocale)
	if err != nil {
		tag = b.defaultTag
	}
	if profile, ok := b.profiles[tag.String()]; ok {
		return tag, profile
	}
	base, _ := tag.Base()
	if profile, ok := b.profiles[base.String()]; ok {
		return tag, profile
	}
	return tag, b.profiles["en"]
}

func inLocation(value time.Time, zone string) (time.Time, error) {
	if strings.TrimSpace(zone) == "" {
		return value, nil
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, fmt.Errorf("i18n: load time zone %q: %w", zone, err)
	}
	return value.In(location), nil
}

func normalizeStyle(style Style) Style {
	if style == "" {
		return Medium
	}
	return style
}

func formatDate(value time.Time, tag language.Tag, profile DateTimeProfile, style Style) (string, error) {
	p := message.NewPrinter(tag)
	day := p.Sprintf("%v", number.Decimal(value.Day(), number.NoSeparator()))
	year := p.Sprintf("%v", number.Decimal(value.Year(), number.NoSeparator()))
	month := int(value.Month()) - 1
	switch style {
	case Short:
		return shortDate(value, tag, profile), nil
	case Medium:
		if profile.DayFirst {
			return day + " " + profile.ShortMonths[month] + " " + year, nil
		}
		return profile.ShortMonths[month] + " " + day + ", " + year, nil
	case Long:
		if profile.DayFirst {
			return day + " " + profile.Months[month] + " " + year, nil
		}
		return profile.Months[month] + " " + day + ", " + year, nil
	case Full:
		long, _ := formatDate(value, tag, profile, Long)
		return profile.Weekdays[value.Weekday()] + ", " + long, nil
	default:
		return "", ErrInvalidDateTimeStyle
	}
}

func shortDate(value time.Time, tag language.Tag, profile DateTimeProfile) string {
	p := message.NewPrinter(tag)
	day := p.Sprintf("%v", number.Decimal(value.Day(), number.NoSeparator(), number.MinIntegerDigits(2)))
	month := p.Sprintf("%v", number.Decimal(int(value.Month()), number.NoSeparator(), number.MinIntegerDigits(2)))
	year := p.Sprintf("%v", number.Decimal(value.Year(), number.NoSeparator(), number.MinIntegerDigits(4)))
	fields := map[byte]string{'D': day, 'M': month, 'Y': year}
	parts := make([]string, 0, 3)
	for _, field := range []byte(profile.ShortDateOrder) {
		parts = append(parts, fields[field])
	}
	return strings.Join(parts, "/")
}

func formatTime(value time.Time, tag language.Tag, profile DateTimeProfile, style Style) (string, error) {
	if style != Short && style != Medium && style != Long && style != Full {
		return "", ErrInvalidDateTimeStyle
	}
	p := message.NewPrinter(tag)
	hour := value.Hour()
	suffix := ""
	if !profile.TwentyFourHour {
		suffix = " AM"
		if hour >= 12 {
			suffix = " PM"
		}
		hour %= 12
		if hour == 0 {
			hour = 12
		}
	}
	hourDigits := 2
	if !profile.TwentyFourHour {
		hourDigits = 1
	}
	hourText := p.Sprintf("%v", number.Decimal(hour, number.NoSeparator(), number.MinIntegerDigits(hourDigits)))
	minute := p.Sprintf("%v", number.Decimal(value.Minute(), number.NoSeparator(), number.MinIntegerDigits(2)))
	formatted := hourText + ":" + minute
	if style != Short {
		second := p.Sprintf("%v", number.Decimal(value.Second(), number.NoSeparator(), number.MinIntegerDigits(2)))
		formatted += ":" + second
	}
	formatted += suffix
	if style == Long || style == Full {
		zone, _ := value.Zone()
		formatted += " " + zone
	}
	return formatted, nil
}

func validateProfile(profile DateTimeProfile) error {
	if profile.ShortDateOrder == "" || profile.DateTimeJoiner == "" {
		return ErrInvalidDateTimeProfile
	}
	seen := map[byte]bool{}
	for _, field := range []byte(profile.ShortDateOrder) {
		if field != 'D' && field != 'M' && field != 'Y' || seen[field] {
			return ErrInvalidDateTimeProfile
		}
		seen[field] = true
	}
	if len(seen) != 3 {
		return ErrInvalidDateTimeProfile
	}
	for i := range profile.Months {
		if profile.Months[i] == "" || profile.ShortMonths[i] == "" {
			return ErrInvalidDateTimeProfile
		}
	}
	for _, weekday := range profile.Weekdays {
		if weekday == "" {
			return ErrInvalidDateTimeProfile
		}
	}
	return nil
}

func builtinDateTimeProfiles() map[string]DateTimeProfile {
	en := DateTimeProfile{
		Months:         [12]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
		ShortMonths:    [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
		Weekdays:       [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
		ShortDateOrder: "MDY", DateTimeJoiner: ", ",
	}
	enGB := en
	enGB.ShortDateOrder = "DMY"
	enGB.DayFirst = true
	enGB.TwentyFourHour = true

	es := DateTimeProfile{
		Months:         [12]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"},
		ShortMonths:    [12]string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sept", "oct", "nov", "dic"},
		Weekdays:       [7]string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"},
		ShortDateOrder: "DMY", DayFirst: true, TwentyFourHour: true, DateTimeJoiner: ", ",
	}
	fr := DateTimeProfile{
		Months:         [12]string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"},
		ShortMonths:    [12]string{"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."},
		Weekdays:       [7]string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"},
		ShortDateOrder: "DMY", DayFirst: true, TwentyFourHour: true, DateTimeJoiner: " à ",
	}
	return map[string]DateTimeProfile{"en": en, "en-GB": enGB, "es": es, "fr": fr}
}
