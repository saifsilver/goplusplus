package gpp

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

func validateRequiredCondition(value, owner reflect.Value, rule validationRule) (bool, error) {
	triggered, err := validationConditionTriggered(owner, rule)
	if err != nil {
		return false, err
	}
	return !triggered || !isEmptyValidationValue(value), nil
}

func validateExcludedCondition(value, owner reflect.Value, rule validationRule) (bool, error) {
	triggered, err := validationConditionTriggered(owner, rule)
	if err != nil {
		return false, err
	}
	return !triggered || isEmptyValidationValue(value), nil
}

func validationConditionTriggered(owner reflect.Value, rule validationRule) (bool, error) {
	parameters, err := splitValidationParameters(rule.param)
	if err != nil || len(parameters) == 0 {
		return false, &validationConfigError{message: rule.name + " requires field parameters"}
	}

	suffix := strings.TrimPrefix(strings.TrimPrefix(rule.name, "required_"), "excluded_")
	switch suffix {
	case "if", "unless":
		if len(parameters)%2 != 0 {
			return false, &validationConfigError{message: rule.name + " requires field/value pairs"}
		}
		matches := true
		for index := 0; index < len(parameters); index += 2 {
			field, found := fieldByValidationPath(owner, parameters[index])
			if !found {
				return false, &validationConfigError{message: "unknown conditional field: " + parameters[index]}
			}
			if fmt.Sprint(valueInterface(dereferenceValue(field))) != parameters[index+1] {
				matches = false
			}
		}
		if suffix == "unless" {
			return !matches, nil
		}
		return matches, nil
	case "with", "with_all", "without", "without_all":
		present := 0
		for _, fieldName := range parameters {
			field, found := fieldByValidationPath(owner, fieldName)
			if !found {
				return false, &validationConfigError{message: "unknown conditional field: " + fieldName}
			}
			if !isEmptyValidationValue(field) {
				present++
			}
		}
		switch suffix {
		case "with":
			return present > 0, nil
		case "with_all":
			return present == len(parameters), nil
		case "without":
			return present < len(parameters), nil
		default:
			return present == 0, nil
		}
	default:
		return false, &validationConfigError{message: "invalid conditional rule: " + rule.name}
	}
}

func fieldByValidationPath(owner reflect.Value, path string) (reflect.Value, bool) {
	value := dereferenceValue(owner)
	for _, segment := range strings.Split(path, ".") {
		if !value.IsValid() || value.Kind() != reflect.Struct {
			return reflect.Value{}, false
		}
		value = value.FieldByName(segment)
		if !value.IsValid() {
			return reflect.Value{}, false
		}
		if segment != path {
			value = dereferenceValue(value)
		}
	}
	return value, true
}

func compareReflectedValues(left, right reflect.Value) (int, bool) {
	left = dereferenceValue(left)
	right = dereferenceValue(right)
	if !left.IsValid() || !right.IsValid() {
		if !left.IsValid() && !right.IsValid() {
			return 0, true
		}
		if !left.IsValid() {
			return -1, true
		}
		return 1, true
	}

	if left.Type() == reflect.TypeFor[time.Time]() && right.Type() == left.Type() {
		leftTime := left.Interface().(time.Time)
		rightTime := right.Interface().(time.Time)
		if leftTime.Before(rightTime) {
			return -1, true
		}
		if leftTime.After(rightTime) {
			return 1, true
		}
		return 0, true
	}
	if isSignedInteger(left.Kind()) && isSignedInteger(right.Kind()) {
		return compareInt64(left.Int(), right.Int()), true
	}
	if isUnsignedInteger(left.Kind()) && isUnsignedInteger(right.Kind()) {
		return compareUint64(left.Uint(), right.Uint()), true
	}
	if isFloat(left.Kind()) && isFloat(right.Kind()) {
		return compareFloat64(left.Float(), right.Float()), true
	}
	if left.Kind() != right.Kind() {
		return 0, false
	}

	switch left.Kind() {
	case reflect.String:
		return strings.Compare(left.String(), right.String()), true
	case reflect.Bool:
		return compareBool(left.Bool(), right.Bool()), true
	case reflect.Array, reflect.Map, reflect.Slice:
		return compareInt64(int64(left.Len()), int64(right.Len())), true
	default:
		if left.Type().Comparable() && right.Type() == left.Type() {
			if left.Interface() == right.Interface() {
				return 0, true
			}
		}
		return 0, false
	}
}

func splitValidationParameters(parameter string) ([]string, error) {
	var parameters []string
	var current strings.Builder
	var quote rune
	escaped := false

	flush := func() {
		if current.Len() > 0 {
			parameters = append(parameters, current.String())
			current.Reset()
		}
	}
	for _, character := range parameter {
		if escaped {
			current.WriteRune(character)
			escaped = false
			continue
		}
		if character == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if character == quote {
				quote = 0
			} else {
				current.WriteRune(character)
			}
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
			continue
		}
		if character == ' ' || character == '\t' {
			flush()
			continue
		}
		current.WriteRune(character)
	}
	if escaped || quote != 0 {
		return nil, &validationConfigError{message: "unterminated validation parameter"}
	}
	flush()
	return parameters, nil
}

func isSignedInteger(kind reflect.Kind) bool {
	return kind >= reflect.Int && kind <= reflect.Int64
}

func isUnsignedInteger(kind reflect.Kind) bool {
	return kind >= reflect.Uint && kind <= reflect.Uintptr
}

func isFloat(kind reflect.Kind) bool {
	return kind == reflect.Float32 || kind == reflect.Float64
}
