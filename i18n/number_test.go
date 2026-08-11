package i18n

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestFormatNumberAndPercent(t *testing.T) {
	got, err := FormatNumber(12345.6789, "en-US", NumberOptions{MinFractionDigits: 2, MaxFractionDigits: 2})
	if err != nil || got != "12,345.68" {
		t.Fatalf("English number = %q, %v", got, err)
	}
	got, err = FormatNumber(12345.6, "de-DE", NumberOptions{DisableGrouping: true})
	if err != nil || got != "12345,6" {
		t.Fatalf("German ungrouped number = %q, %v", got, err)
	}
	got, err = FormatNumber(12345.6, "en", NumberOptions{Integer: true})
	if err != nil || got != "12,346" {
		t.Fatalf("integer number = %q, %v", got, err)
	}
	got, err = FormatPercent(0.125, "fr", NumberOptions{MinFractionDigits: 1, MaxFractionDigits: 1})
	if err != nil || got != "12,5 %" {
		t.Fatalf("French percent = %q, %v", got, err)
	}
	if _, err = FormatNumber(1, "bad_tag", NumberOptions{}); !errors.Is(err, ErrInvalidLanguage) {
		t.Fatalf("number locale error = %v", err)
	}
	if _, err = FormatPercent(1, "bad_tag", NumberOptions{}); !errors.Is(err, ErrInvalidLanguage) {
		t.Fatalf("percent locale error = %v", err)
	}
	if _, err = FormatPercent(1, "en", NumberOptions{MinFractionDigits: -1}); !errors.Is(err, ErrInvalidNumberOptions) {
		t.Fatalf("percent options error = %v", err)
	}
	if _, err = FormatPercent(1, "en", NumberOptions{Integer: true, MaxFractionDigits: 2}); !errors.Is(err, ErrInvalidNumberOptions) {
		t.Fatalf("integer options error = %v", err)
	}
	for _, value := range []any{nil, "12"} {
		if _, err = FormatNumber(value, "en", NumberOptions{}); !errors.Is(err, ErrUnsupportedNumber) {
			t.Fatalf("unsupported value %v error = %v", value, err)
		}
	}
	if _, err = FormatPercent(nil, "en", NumberOptions{}); !errors.Is(err, ErrUnsupportedNumber) {
		t.Fatalf("unsupported percent error = %v", err)
	}
	type customInt int16
	for _, value := range []any{customInt(12), uint8(12), float32(12.5)} {
		if _, err = FormatNumber(value, "en", NumberOptions{}); err != nil {
			t.Fatalf("supported value %T error = %v", value, err)
		}
	}
	invalid := []NumberOptions{
		{MinFractionDigits: -1},
		{MaxFractionDigits: -1},
		{MaxFractionDigits: 19},
		{MinFractionDigits: 3, MaxFractionDigits: 2},
	}
	for _, options := range invalid {
		if _, err = FormatNumber(1, "en", options); !errors.Is(err, ErrInvalidNumberOptions) {
			t.Fatalf("options %+v error = %v", options, err)
		}
	}
}

func TestFormatMoneyUsesExactMinorUnits(t *testing.T) {
	tests := []struct {
		name   string
		money  Money
		locale string
		want   string
	}{
		{name: "US dollars", money: Money{123456, "usd"}, locale: "en-US", want: "$1,234.56"},
		{name: "French euros", money: Money{123456, "EUR"}, locale: "fr-FR", want: "1 234,56 €"},
		{name: "yen", money: Money{1234, "JPY"}, locale: "ja-JP", want: "￥1,234"},
		{name: "three decimals", money: Money{1234, "KWD"}, locale: "en", want: "KWD1.234"},
		{name: "negative minimum", money: Money{math.MinInt64, "USD"}, locale: "en", want: "-$92,233,720,368,547,758.08"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatMoney(tt.money, tt.locale)
			if err != nil || got != tt.want {
				t.Fatalf("FormatMoney = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
	if _, err := FormatMoney(Money{1, "NOPE"}, "en"); !errors.Is(err, ErrInvalidCurrency) {
		t.Fatalf("currency error = %v", err)
	}
	if _, err := FormatMoney(Money{1, "USD"}, "bad_tag"); !errors.Is(err, ErrInvalidLanguage) {
		t.Fatalf("locale error = %v", err)
	}
}

func TestCompatibilityCurrencyFormatter(t *testing.T) {
	for _, test := range []struct {
		amount   float64
		currency string
		want     string
	}{
		{100.50, "USD", "$100.50"},
		{100.50, "EUR", "€100.50"},
		{100.50, "GBP", "£100.50"},
		{100.50, "JPY", "¥101"},
	} {
		if got := FormatCurrency(test.amount, test.currency); got != test.want {
			t.Fatalf("FormatCurrency(%s) = %q; want %q", test.currency, got, test.want)
		}
	}
	if got := FormatCurrency(12.5, "wat"); !strings.HasPrefix(got, "WAT ") {
		t.Fatalf("unknown currency = %q", got)
	}
}
