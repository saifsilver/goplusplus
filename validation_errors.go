package gpp

import (
	"fmt"
	"reflect"
)

func newValidationViolation(path string, value reflect.Value, rule validationRule) error {
	value = dereferenceValue(value)
	kind := reflect.Invalid
	if value.IsValid() {
		kind = value.Kind()
	}
	return &validationViolation{field: path, rule: rule.name, param: rule.param, kind: kind}
}

func formatValidationError(violation *validationViolation) string {
	switch violation.rule {
	case "required", "required_if", "required_unless", "required_with", "required_with_all", "required_without", "required_without_all":
		return fmt.Sprintf("Field '%s' is required", violation.field)
	case "email":
		return fmt.Sprintf("Field '%s' must be a valid email address", violation.field)
	case "min":
		return formatMinimumError(violation)
	case "max":
		return formatMaximumError(violation)
	case "len":
		return formatLengthError(violation)
	case "oneof":
		return fmt.Sprintf("Field '%s' must be one of [%s]", violation.field, violation.param)
	}

	rule := violation.rule
	if violation.param != "" {
		rule += "=" + violation.param
	}
	return fmt.Sprintf("Field '%s' failed validation rule '%s'", violation.field, rule)
}

func formatMinimumError(violation *validationViolation) string {
	switch violation.kind {
	case reflect.String:
		return fmt.Sprintf("Field '%s' must contain at least %s characters", violation.field, violation.param)
	case reflect.Array, reflect.Map, reflect.Slice:
		return fmt.Sprintf("Field '%s' must contain at least %s items", violation.field, violation.param)
	default:
		return fmt.Sprintf("Field '%s' must be at least %s", violation.field, violation.param)
	}
}

func formatMaximumError(violation *validationViolation) string {
	switch violation.kind {
	case reflect.String:
		return fmt.Sprintf("Field '%s' must contain at most %s characters", violation.field, violation.param)
	case reflect.Array, reflect.Map, reflect.Slice:
		return fmt.Sprintf("Field '%s' must contain at most %s items", violation.field, violation.param)
	default:
		return fmt.Sprintf("Field '%s' must be at most %s", violation.field, violation.param)
	}
}

func formatLengthError(violation *validationViolation) string {
	switch violation.kind {
	case reflect.String:
		return fmt.Sprintf("Field '%s' must contain exactly %s characters", violation.field, violation.param)
	case reflect.Array, reflect.Map, reflect.Slice:
		return fmt.Sprintf("Field '%s' must contain exactly %s items", violation.field, violation.param)
	default:
		return fmt.Sprintf("Field '%s' must equal %s", violation.field, violation.param)
	}
}
