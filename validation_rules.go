package gpp

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var supportedValidationRules = map[string]struct{}{
	"omitempty": {}, "omitnil": {}, "required": {},
	"required_if": {}, "required_unless": {}, "required_with": {}, "required_with_all": {},
	"required_without": {}, "required_without_all": {},
	"excluded_if": {}, "excluded_unless": {}, "excluded_with": {}, "excluded_with_all": {},
	"excluded_without": {}, "excluded_without_all": {},
	"eq": {}, "ne": {}, "gt": {}, "gte": {}, "lt": {}, "lte": {}, "min": {}, "max": {}, "len": {},
	"eqfield": {}, "nefield": {}, "gtfield": {}, "gtefield": {}, "ltfield": {}, "ltefield": {},
	"oneof": {}, "not_oneof": {}, "unique": {},
	"alpha": {}, "alphanum": {}, "numeric": {}, "number": {}, "lowercase": {}, "uppercase": {},
	"ascii": {}, "printascii": {}, "boolean": {},
	"contains": {}, "containsany": {}, "containsrune": {}, "excludes": {}, "excludesall": {},
	"excludesrune": {}, "startswith": {}, "endswith": {},
	"email": {}, "url": {}, "http_url": {}, "ip": {}, "ipv4": {}, "ipv6": {},
	"cidr": {}, "cidrv4": {}, "cidrv6": {}, "hostname": {}, "hostname_port": {},
	"uuid": {}, "uuid3": {}, "uuid4": {}, "uuid5": {}, "json": {}, "base64": {},
	"base64url": {}, "hexadecimal": {}, "hexcolor": {}, "datetime": {},
	"dive": {}, "keys": {}, "endkeys": {},
}

func isSupportedValidationRule(name string) bool {
	_, supported := supportedValidationRules[name]
	return supported
}

func applyValidationRule(value, owner reflect.Value, rule validationRule) (bool, error) {
	switch rule.name {
	case "required":
		return !isEmptyValidationValue(value), nil
	case "required_if", "required_unless", "required_with", "required_with_all", "required_without", "required_without_all":
		return validateRequiredCondition(value, owner, rule)
	case "excluded_if", "excluded_unless", "excluded_with", "excluded_with_all", "excluded_without", "excluded_without_all":
		return validateExcludedCondition(value, owner, rule)
	case "eq", "ne", "gt", "gte", "lt", "lte", "min", "max", "len":
		return validateConstantComparison(value, rule)
	case "eqfield", "nefield", "gtfield", "gtefield", "ltfield", "ltefield":
		return validateFieldComparison(value, owner, rule)
	case "oneof", "not_oneof":
		return validateOneOf(value, rule)
	case "unique":
		return validateUnique(value)
	case "alpha", "alphanum", "numeric", "number", "lowercase", "uppercase", "ascii", "printascii", "boolean":
		return validateCharacterRule(value, rule)
	case "contains", "containsany", "containsrune", "excludes", "excludesall", "excludesrune", "startswith", "endswith":
		return validateStringRelation(value, rule)
	case "email", "url", "http_url", "ip", "ipv4", "ipv6", "cidr", "cidrv4", "cidrv6", "hostname", "hostname_port",
		"uuid", "uuid3", "uuid4", "uuid5", "json", "base64", "base64url", "hexadecimal", "hexcolor", "datetime":
		return validateFormatRule(value, rule)
	case "omitempty", "omitnil":
		return true, nil
	case "dive", "keys", "endkeys":
		return false, &validationConfigError{message: rule.name + " is in an invalid position"}
	default:
		return false, &validationConfigError{message: "unsupported validation rule: " + rule.name}
	}
}

func validateConstantComparison(value reflect.Value, rule validationRule) (bool, error) {
	if rule.param == "" {
		return false, &validationConfigError{message: rule.name + " requires a parameter"}
	}
	value = dereferenceValue(value)
	if !value.IsValid() {
		return false, nil
	}

	comparison, err := compareValueToParameter(value, rule)
	if err != nil {
		return false, err
	}
	switch rule.name {
	case "eq", "len":
		return comparison == 0, nil
	case "ne":
		return comparison != 0, nil
	case "gt":
		return comparison > 0, nil
	case "gte", "min":
		return comparison >= 0, nil
	case "lt":
		return comparison < 0, nil
	case "lte", "max":
		return comparison <= 0, nil
	default:
		return false, &validationConfigError{message: "invalid comparison rule"}
	}
}

func compareValueToParameter(value reflect.Value, rule validationRule) (int, error) {
	parameter := rule.param
	switch value.Kind() {
	case reflect.String:
		if rule.name == "eq" || rule.name == "ne" {
			return strings.Compare(value.String(), parameter), nil
		}
		length, err := strconv.ParseInt(parameter, 10, 64)
		if err != nil {
			return 0, invalidRuleParameter(parameter)
		}
		return compareInt64(int64(utf8.RuneCountInString(value.String())), length), nil
	case reflect.Array, reflect.Map, reflect.Slice:
		length, err := strconv.ParseInt(parameter, 10, 64)
		if err != nil {
			return 0, invalidRuleParameter(parameter)
		}
		return compareInt64(int64(value.Len()), length), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		expected, err := strconv.ParseInt(parameter, 10, value.Type().Bits())
		if err != nil {
			return 0, invalidRuleParameter(parameter)
		}
		return compareInt64(value.Int(), expected), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		expected, err := strconv.ParseUint(parameter, 10, value.Type().Bits())
		if err != nil {
			return 0, invalidRuleParameter(parameter)
		}
		return compareUint64(value.Uint(), expected), nil
	case reflect.Float32, reflect.Float64:
		expected, err := strconv.ParseFloat(parameter, value.Type().Bits())
		if err != nil {
			return 0, invalidRuleParameter(parameter)
		}
		return compareFloat64(value.Float(), expected), nil
	case reflect.Bool:
		expected, err := strconv.ParseBool(parameter)
		if err != nil {
			return 0, invalidRuleParameter(parameter)
		}
		return compareBool(value.Bool(), expected), nil
	default:
		return 0, &validationConfigError{message: "comparison is unsupported for " + value.Kind().String()}
	}
}

func validateFieldComparison(value, owner reflect.Value, rule validationRule) (bool, error) {
	if rule.param == "" {
		return false, &validationConfigError{message: rule.name + " requires a field name"}
	}
	other, found := fieldByValidationPath(owner, rule.param)
	if !found {
		return false, &validationConfigError{message: "unknown comparison field: " + rule.param}
	}

	comparison, comparable := compareReflectedValues(value, other)
	if !comparable {
		return false, &validationConfigError{message: "fields are not comparable"}
	}
	switch rule.name {
	case "eqfield":
		return comparison == 0, nil
	case "nefield":
		return comparison != 0, nil
	case "gtfield":
		return comparison > 0, nil
	case "gtefield":
		return comparison >= 0, nil
	case "ltfield":
		return comparison < 0, nil
	case "ltefield":
		return comparison <= 0, nil
	default:
		return false, &validationConfigError{message: "invalid field comparison"}
	}
}

func validateOneOf(value reflect.Value, rule validationRule) (bool, error) {
	parameters, err := splitValidationParameters(rule.param)
	if err != nil || len(parameters) == 0 {
		return false, &validationConfigError{message: rule.name + " requires one or more values"}
	}
	actual := fmt.Sprint(valueInterface(value))
	found := false
	for _, parameter := range parameters {
		if actual == parameter {
			found = true
			break
		}
	}
	if rule.name == "not_oneof" {
		return !found, nil
	}
	return found, nil
}

func validateUnique(value reflect.Value) (bool, error) {
	value = dereferenceValue(value)
	if !value.IsValid() {
		return true, nil
	}
	if value.Kind() != reflect.Array && value.Kind() != reflect.Slice {
		return false, &validationConfigError{message: "unique requires an array or slice"}
	}
	if value.Len() > maxValidationCollection {
		return false, nil
	}
	seen := make(map[any]struct{}, value.Len())
	for index := 0; index < value.Len(); index++ {
		item := dereferenceValue(value.Index(index))
		if !item.IsValid() {
			if _, exists := seen[nil]; exists {
				return false, nil
			}
			seen[nil] = struct{}{}
			continue
		}
		if !item.Comparable() {
			return false, &validationConfigError{message: "unique requires comparable collection elements"}
		}
		key := item.Interface()
		if _, exists := seen[key]; exists {
			return false, nil
		}
		seen[key] = struct{}{}
	}
	return true, nil
}

func validateCharacterRule(value reflect.Value, rule validationRule) (bool, error) {
	text, err := validationString(value, rule.name)
	if err != nil {
		return false, err
	}
	switch rule.name {
	case "alpha":
		return text != "" && allRunes(text, unicode.IsLetter), nil
	case "alphanum":
		return text != "" && allRunes(text, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }), nil
	case "numeric":
		return text != "" && allRunes(text, unicode.IsDigit), nil
	case "number":
		return numberPattern.MatchString(text), nil
	case "lowercase":
		return text == strings.ToLower(text), nil
	case "uppercase":
		return text == strings.ToUpper(text), nil
	case "ascii":
		return allRunes(text, func(r rune) bool { return r <= unicode.MaxASCII }), nil
	case "printascii":
		return allRunes(text, func(r rune) bool { return r >= 0x20 && r <= 0x7e }), nil
	case "boolean":
		_, err := strconv.ParseBool(text)
		return err == nil, nil
	default:
		return false, &validationConfigError{message: "invalid character rule"}
	}
}

func validateStringRelation(value reflect.Value, rule validationRule) (bool, error) {
	text, err := validationString(value, rule.name)
	if err != nil {
		return false, err
	}
	if rule.param == "" {
		return false, &validationConfigError{message: rule.name + " requires a parameter"}
	}
	switch rule.name {
	case "contains":
		return strings.Contains(text, rule.param), nil
	case "containsany":
		return strings.ContainsAny(text, rule.param), nil
	case "containsrune":
		runes := []rune(rule.param)
		if len(runes) != 1 {
			return false, &validationConfigError{message: "containsrune requires exactly one rune"}
		}
		return strings.ContainsRune(text, runes[0]), nil
	case "excludes":
		return !strings.Contains(text, rule.param), nil
	case "excludesall":
		return !strings.ContainsAny(text, rule.param), nil
	case "excludesrune":
		runes := []rune(rule.param)
		if len(runes) != 1 {
			return false, &validationConfigError{message: "excludesrune requires exactly one rune"}
		}
		return !strings.ContainsRune(text, runes[0]), nil
	case "startswith":
		return strings.HasPrefix(text, rule.param), nil
	case "endswith":
		return strings.HasSuffix(text, rule.param), nil
	default:
		return false, &validationConfigError{message: "invalid string rule"}
	}
}

func isEmptyValidationValue(value reflect.Value) bool {
	if !value.IsValid() {
		return true
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return true
		}
		return isEmptyValidationValue(value.Elem())
	}
	switch value.Kind() {
	case reflect.String, reflect.Map, reflect.Slice:
		return value.Len() == 0
	case reflect.Array:
		return value.Len() == 0 || value.IsZero()
	}
	return value.IsZero()
}

func validationString(value reflect.Value, rule string) (string, error) {
	value = dereferenceValue(value)
	if !value.IsValid() {
		return "", nil
	}
	if value.Kind() != reflect.String {
		return "", &validationConfigError{message: rule + " requires a string"}
	}
	return value.String(), nil
}

func valueInterface(value reflect.Value) any {
	if !value.IsValid() {
		return nil
	}
	if value.Kind() == reflect.Interface && !value.IsNil() {
		return valueInterface(value.Elem())
	}
	return value.Interface()
}

func allRunes(value string, predicate func(rune) bool) bool {
	for _, character := range value {
		if !predicate(character) {
			return false
		}
	}
	return true
}

func compareInt64(left, right int64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareUint64(left, right uint64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareFloat64(left, right float64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareBool(left, right bool) int {
	if left == right {
		return 0
	}
	if !left {
		return -1
	}
	return 1
}

func invalidRuleParameter(parameter string) error {
	return &validationConfigError{message: "invalid validation parameter: " + parameter}
}
