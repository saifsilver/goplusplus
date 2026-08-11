package i18n

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"unicode"

	"golang.org/x/text/currency"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"
)

var (
	ErrInvalidCurrency      = errors.New("i18n: invalid ISO 4217 currency")
	ErrInvalidNumberOptions = errors.New("i18n: invalid number formatting options")
	ErrUnsupportedNumber    = errors.New("i18n: unsupported numeric value")
)

// NumberOptions controls decimal precision and grouping. A zero-value uses up
// to three fractional digits and locale grouping.
type NumberOptions struct {
	MinFractionDigits int
	MaxFractionDigits int
	DisableGrouping   bool
	Integer           bool
}

// Money stores an exact amount in ISO currency minor units (for example,
// USD 10.25 is MinorUnits: 1025). This avoids binary floating-point money.
type Money struct {
	MinorUnits int64
	Currency   string
}

// FormatNumber formats numeric built-in Go values with locale separators and
// digits.
func FormatNumber(value any, locale string, options NumberOptions) (string, error) {
	value, err := normalizeNumber(value)
	if err != nil {
		return "", err
	}
	tag, err := parseTag(locale)
	if err != nil {
		return "", err
	}
	opts, err := numberOptions(options)
	if err != nil {
		return "", err
	}
	return message.NewPrinter(tag).Sprintf("%v", number.Decimal(value, opts...)), nil
}

// FormatPercent formats a ratio, where 0.125 represents 12.5 percent.
func FormatPercent(value any, locale string, options NumberOptions) (string, error) {
	value, err := normalizeNumber(value)
	if err != nil {
		return "", err
	}
	tag, err := parseTag(locale)
	if err != nil {
		return "", err
	}
	opts, err := numberOptions(options)
	if err != nil {
		return "", err
	}
	return message.NewPrinter(tag).Sprintf("%v", number.Percent(value, opts...)), nil
}

func numberOptions(options NumberOptions) ([]number.Option, error) {
	max := options.MaxFractionDigits
	if options.Integer {
		if options.MinFractionDigits != 0 || max != 0 {
			return nil, ErrInvalidNumberOptions
		}
	} else if max == 0 {
		max = 3
	}
	if options.MinFractionDigits < 0 || max < 0 || max > 18 || options.MinFractionDigits > max {
		return nil, ErrInvalidNumberOptions
	}
	opts := []number.Option{
		number.MinFractionDigits(options.MinFractionDigits),
		number.MaxFractionDigits(max),
	}
	if options.DisableGrouping {
		opts = append(opts, number.NoSeparator())
	}
	return opts, nil
}

func normalizeNumber(value any) (any, error) {
	if value == nil {
		return nil, ErrUnsupportedNumber
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflected.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return reflected.Uint(), nil
	case reflect.Float32, reflect.Float64:
		return reflected.Float(), nil
	default:
		return nil, ErrUnsupportedNumber
	}
}

// FormatMoney formats exact minor units using the currency's ISO precision,
// localized digits, and locale-appropriate currency placement.
func FormatMoney(money Money, locale string) (string, error) {
	tag, err := parseTag(locale)
	if err != nil {
		return "", err
	}
	unit, err := currency.ParseISO(strings.ToUpper(strings.TrimSpace(money.Currency)))
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrInvalidCurrency, money.Currency)
	}

	scale, _ := currency.Standard.Rounding(unit)
	major, fraction := splitMinorUnits(money.MinorUnits, scale)
	p := message.NewPrinter(tag)
	numberText := p.Sprintf("%v", number.Decimal(major))
	if scale > 0 {
		fractionText := p.Sprintf("%v", number.Decimal(fraction,
			number.NoSeparator(),
			number.MinIntegerDigits(scale),
			number.MaxIntegerDigits(scale),
		))
		numberText += decimalSeparator(tag) + fractionText
	}
	symbol := p.Sprintf("%v", currency.Symbol(unit))
	formatted := placeCurrency(tag, symbol, numberText)
	if money.MinorUnits < 0 {
		formatted = "-" + formatted
	}
	return formatted, nil
}

func splitMinorUnits(value int64, scale int) (uint64, uint64) {
	abs := uint64(value)
	if value < 0 {
		abs = uint64(-(value + 1)) + 1
	}
	power := uint64(1)
	for range scale {
		power *= 10
	}
	return abs / power, abs % power
}

func decimalSeparator(tag language.Tag) string {
	formatted := message.NewPrinter(tag).Sprintf("%v", number.Decimal(1.1,
		number.MinFractionDigits(1), number.MaxFractionDigits(1)))
	formatted = strings.TrimLeftFunc(formatted, func(r rune) bool {
		return unicode.IsDigit(r)
	})
	return string([]rune(formatted)[0])
}

func placeCurrency(tag language.Tag, symbol, amount string) string {
	base, _ := tag.Base()
	switch base.String() {
	case "ca", "da", "de", "es", "fi", "fr", "it", "nl", "no", "pl", "pt", "ru", "sv":
		return amount + "\u00a0" + symbol
	default:
		return symbol + amount
	}
}

// FormatCurrency preserves the original convenience API. New financial code
// should use FormatMoney so amounts remain exact and invalid currencies surface
// as errors.
func FormatCurrency(amount float64, currencyCode string) string {
	unit, err := currency.ParseISO(strings.ToUpper(strings.TrimSpace(currencyCode)))
	if err != nil {
		return strings.ToUpper(strings.TrimSpace(currencyCode)) + " " + fmt.Sprintf("%.2f", amount)
	}
	scale, _ := currency.Standard.Rounding(unit)
	factor := math.Pow10(scale)
	minor := int64(math.Round(amount * factor))
	formatted, _ := FormatMoney(Money{MinorUnits: minor, Currency: unit.String()}, "en-US")
	return formatted
}
