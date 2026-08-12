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

func validationRuleMessage(violation *validationViolation) string {
	switch violation.rule {
	case "required", "required_if", "required_unless", "required_with", "required_with_all", "required_without", "required_without_all":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "min":
		return formatMinimumError(violation)
	case "max":
		return formatMaximumError(violation)
	case "len":
		return formatLengthError(violation)
	case "oneof":
		return fmt.Sprintf("must be one of [%s]", violation.param)
	}

	rule := violation.rule
	if violation.param != "" {
		rule += "=" + violation.param
	}
	return fmt.Sprintf("failed validation rule '%s'", rule)
}

func formatMinimumError(violation *validationViolation) string {
	switch violation.kind {
	case reflect.String:
		return fmt.Sprintf("must contain at least %s characters", violation.param)
	case reflect.Array, reflect.Map, reflect.Slice:
		return fmt.Sprintf("must contain at least %s items", violation.param)
	default:
		return fmt.Sprintf("must be at least %s", violation.param)
	}
}

func formatMaximumError(violation *validationViolation) string {
	switch violation.kind {
	case reflect.String:
		return fmt.Sprintf("must contain at most %s characters", violation.param)
	case reflect.Array, reflect.Map, reflect.Slice:
		return fmt.Sprintf("must contain at most %s items", violation.param)
	default:
		return fmt.Sprintf("must be at most %s", violation.param)
	}
}

func formatLengthError(violation *validationViolation) string {
	switch violation.kind {
	case reflect.String:
		return fmt.Sprintf("must contain exactly %s characters", violation.param)
	case reflect.Array, reflect.Map, reflect.Slice:
		return fmt.Sprintf("must contain exactly %s items", violation.param)
	default:
		return fmt.Sprintf("must equal %s", violation.param)
	}
}
