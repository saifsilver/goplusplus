package i18n

import (
	"errors"
	"testing"
	"time"
)

func TestLocalizedDateStyles(t *testing.T) {
	bundle := NewBundle("en")
	value := time.Date(2026, time.August, 11, 15, 4, 5, 0, time.UTC)
	tests := []struct {
		locale string
		style  Style
		want   string
	}{
		{"en-US", Short, "08/11/2026"},
		{"en-GB", Medium, "11 Aug 2026"},
		{"es", Long, "11 agosto 2026"},
		{"fr", Full, "mardi, 11 août 2026"},
		{"fr", "", "11 août 2026"},
	}
	for _, tt := range tests {
		got, err := bundle.FormatDate(value, tt.locale, "UTC", tt.style)
		if err != nil || got != tt.want {
			t.Fatalf("FormatDate(%s,%s) = %q, %v; want %q", tt.locale, tt.style, got, err, tt.want)
		}
	}
	if _, err := bundle.FormatDate(value, "en", "UTC", Style("huge")); !errors.Is(err, ErrInvalidDateTimeStyle) {
		t.Fatalf("date style error = %v", err)
	}
	if _, err := bundle.FormatDate(value, "en", "Mars/Olympus", Short); err == nil {
		t.Fatal("expected invalid zone error")
	}
}

func TestLocalizedTimeAndDateTime(t *testing.T) {
	bundle := NewBundle("en")
	value := time.Date(2026, time.January, 2, 23, 4, 5, 0, time.UTC)

	got, err := bundle.FormatTime(value, "en-US", "America/New_York", Short)
	if err != nil || got != "6:04 PM" {
		t.Fatalf("US short time = %q, %v", got, err)
	}
	got, err = bundle.FormatTime(value, "fr", "", Medium)
	if err != nil || got != "23:04:05" {
		t.Fatalf("French medium time = %q, %v", got, err)
	}
	got, err = bundle.FormatTime(value, "en-GB", "UTC", Long)
	if err != nil || got != "23:04:05 UTC" {
		t.Fatalf("British long time = %q, %v", got, err)
	}
	got, err = bundle.FormatTime(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "en", "UTC", Full)
	if err != nil || got != "12:00:00 AM UTC" {
		t.Fatalf("midnight full time = %q, %v", got, err)
	}
	if _, err = bundle.FormatTime(value, "en", "UTC", Style("huge")); !errors.Is(err, ErrInvalidDateTimeStyle) {
		t.Fatalf("time style error = %v", err)
	}
	if _, err = bundle.FormatTime(value, "en", "Mars/Olympus", Short); err == nil {
		t.Fatal("expected invalid time zone error")
	}

	got, err = bundle.FormatDateTime(value, "fr", "UTC", DateTimeOptions{DateStyle: Short, TimeStyle: Short})
	if err != nil || got != "02/01/2026 à 23:04" {
		t.Fatalf("French date-time = %q, %v", got, err)
	}
	if _, err = bundle.FormatDateTime(value, "en", "UTC", DateTimeOptions{DateStyle: "bad"}); !errors.Is(err, ErrInvalidDateTimeStyle) {
		t.Fatalf("combined date error = %v", err)
	}
	if _, err = bundle.FormatDateTime(value, "en", "UTC", DateTimeOptions{TimeStyle: "bad"}); !errors.Is(err, ErrInvalidDateTimeStyle) {
		t.Fatalf("combined time error = %v", err)
	}
}

func TestCustomDateTimeProfiles(t *testing.T) {
	bundle := NewBundle("en")
	if err := bundle.RegisterDateTimeProfile("bad_tag", DateTimeProfile{}); !errors.Is(err, ErrInvalidLanguage) {
		t.Fatalf("profile language error = %v", err)
	}
	invalid := builtinDateTimeProfiles()["en"]
	invalid.DateTimeJoiner = ""
	if err := bundle.RegisterDateTimeProfile("en", invalid); !errors.Is(err, ErrInvalidDateTimeProfile) {
		t.Fatalf("empty joiner error = %v", err)
	}
	invalid = builtinDateTimeProfiles()["en"]
	invalid.ShortDateOrder = "DDY"
	if err := bundle.RegisterDateTimeProfile("en", invalid); !errors.Is(err, ErrInvalidDateTimeProfile) {
		t.Fatalf("duplicate order error = %v", err)
	}
	invalid = builtinDateTimeProfiles()["en"]
	invalid.ShortDateOrder = "DM"
	if err := bundle.RegisterDateTimeProfile("en", invalid); !errors.Is(err, ErrInvalidDateTimeProfile) {
		t.Fatalf("incomplete order error = %v", err)
	}
	invalid = builtinDateTimeProfiles()["en"]
	invalid.Months[0] = ""
	if err := bundle.RegisterDateTimeProfile("en", invalid); !errors.Is(err, ErrInvalidDateTimeProfile) {
		t.Fatalf("month error = %v", err)
	}
	invalid = builtinDateTimeProfiles()["en"]
	invalid.Weekdays[0] = ""
	if err := bundle.RegisterDateTimeProfile("en", invalid); !errors.Is(err, ErrInvalidDateTimeProfile) {
		t.Fatalf("weekday error = %v", err)
	}

	custom := builtinDateTimeProfiles()["en"]
	custom.Months[0] = "Firstmonth"
	if err := bundle.RegisterDateTimeProfile("en-AU", custom); err != nil {
		t.Fatal(err)
	}
	value := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got, err := bundle.FormatDate(value, "en-AU", "UTC", Long)
	if err != nil || got != "Firstmonth 2, 2026" {
		t.Fatalf("custom profile = %q, %v", got, err)
	}

	if got, err = bundle.FormatDate(value, "bad_tag", "UTC", Long); err != nil || got != "January 2, 2026" {
		t.Fatalf("invalid locale fallback = %q, %v", got, err)
	}
	if got, err = bundle.FormatDate(value, "zu", "UTC", Long); err != nil || got != "January 2, 2026" {
		t.Fatalf("unknown profile fallback = %q, %v", got, err)
	}
}
