package gpp

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

type validationRule struct {
	name  string
	param string
}

type validationViolation struct {
	field string
	rule  string
	param string
	kind  reflect.Kind
}

func (e *validationViolation) Error() string {
	return fmt.Sprintf("field %s failed %s", e.field, e.rule)
}

type validationConfigError struct {
	message string
}

func (e *validationConfigError) Error() string { return e.message }

type validationVisit struct {
	typeOf  reflect.Type
	pointer uintptr
}

func validateStruct(value any) error {
	if isNilValidationTarget(value) {
		return ErrBadRequest("Validation target cannot be nil")
	}
	if !isStruct(value) {
		return nil
	}

	err := runLocalValidation(reflect.ValueOf(value))
	var violation *validationViolation
	if errors.As(err, &violation) {
		return ErrBadRequest(formatValidationError(violation))
	}
	if err != nil {
		return ErrInternal("Invalid validation configuration")
	}
	return nil
}

func runLocalValidation(value reflect.Value) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = &validationConfigError{message: fmt.Sprint(recovered)}
		}
	}()

	value = dereferenceValue(value)
	return validateStructValue(value, "", make(map[validationVisit]bool))
}

func validateStructValue(value reflect.Value, path string, visited map[validationVisit]bool) error {
	if value.Type() == reflect.TypeFor[time.Time]() {
		return nil
	}

	typeOf := value.Type()
	for index := 0; index < value.NumField(); index++ {
		fieldType := typeOf.Field(index)
		if fieldType.PkgPath != "" {
			continue
		}
		if fieldType.Tag.Get("validate") == "-" {
			continue
		}

		fieldPath := joinValidationPath(path, fieldType.Name)
		rules, err := parseValidationRules(fieldType.Tag.Get("validate"))
		if err != nil {
			return err
		}
		if err := validateField(value.Field(index), value, fieldPath, rules, visited); err != nil {
			return err
		}
	}
	return nil
}

func validateField(
	value reflect.Value,
	owner reflect.Value,
	path string,
	rules []validationRule,
	visited map[validationVisit]bool,
) error {
	if hasValidationRule(rules, "omitempty") && isEmptyValidationValue(value) {
		return nil
	}
	if hasValidationRule(rules, "omitnil") && isNilValue(value) {
		return nil
	}

	diveIndex := validationRuleIndex(rules, "dive")
	limit := len(rules)
	if diveIndex >= 0 {
		limit = diveIndex
	}
	for _, rule := range rules[:limit] {
		if rule.name == "omitempty" || rule.name == "omitnil" {
			continue
		}
		valid, err := applyValidationRule(value, owner, rule)
		if err != nil {
			return err
		}
		if !valid {
			return newValidationViolation(path, value, rule)
		}
	}

	if diveIndex >= 0 {
		return validateDive(value, owner, path, rules[diveIndex+1:], visited)
	}
	return validateNestedValue(value, path, visited)
}

func validateDive(
	value reflect.Value,
	owner reflect.Value,
	path string,
	rules []validationRule,
	visited map[validationVisit]bool,
) error {
	value = dereferenceValue(value)
	if !value.IsValid() {
		return nil
	}

	switch value.Kind() {
	case reflect.Array, reflect.Slice:
		for index := 0; index < value.Len(); index++ {
			itemPath := fmt.Sprintf("%s[%d]", path, index)
			if err := validateField(value.Index(index), owner, itemPath, rules, visited); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		return validateMapDive(value, owner, path, rules, visited)
	default:
		return &validationConfigError{message: "dive requires an array, slice, or map"}
	}
}

func validateMapDive(
	value reflect.Value,
	owner reflect.Value,
	path string,
	rules []validationRule,
	visited map[validationVisit]bool,
) error {
	keyRules, valueRules, err := splitMapValidationRules(rules)
	if err != nil {
		return err
	}

	keys := value.MapKeys()
	sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j]) })
	for _, key := range keys {
		itemPath := fmt.Sprintf("%s[%v]", path, key.Interface())
		if len(keyRules) > 0 {
			if err := validateField(key, owner, itemPath+".key", keyRules, visited); err != nil {
				return err
			}
		}
		if err := validateField(value.MapIndex(key), owner, itemPath, valueRules, visited); err != nil {
			return err
		}
	}
	return nil
}

func splitMapValidationRules(rules []validationRule) ([]validationRule, []validationRule, error) {
	keysIndex := validationRuleIndex(rules, "keys")
	endKeysIndex := validationRuleIndex(rules, "endkeys")
	if keysIndex < 0 && endKeysIndex < 0 {
		return nil, rules, nil
	}
	if keysIndex != 0 || endKeysIndex <= keysIndex {
		return nil, nil, &validationConfigError{message: "keys must immediately follow dive and end with endkeys"}
	}
	return rules[keysIndex+1 : endKeysIndex], rules[endKeysIndex+1:], nil
}

func validateNestedValue(value reflect.Value, path string, visited map[validationVisit]bool) error {
	for value.IsValid() && (value.Kind() == reflect.Interface || value.Kind() == reflect.Ptr) {
		if value.IsNil() {
			return nil
		}
		if value.Kind() == reflect.Ptr {
			visit := validationVisit{typeOf: value.Type(), pointer: value.Pointer()}
			if visited[visit] {
				return nil
			}
			visited[visit] = true
			defer delete(visited, visit)
		}
		value = value.Elem()
	}
	if value.IsValid() && value.Kind() == reflect.Struct {
		return validateStructValue(value, path, visited)
	}
	return nil
}

func parseValidationRules(tag string) ([]validationRule, error) {
	if tag == "" || tag == "-" {
		return nil, nil
	}

	parts := strings.Split(tag, ",")
	rules := make([]validationRule, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, &validationConfigError{message: "validation tag contains an empty rule"}
		}
		name, parameter, _ := strings.Cut(part, "=")
		if !isSupportedValidationRule(name) {
			return nil, &validationConfigError{message: "unsupported validation rule: " + name}
		}
		rules = append(rules, validationRule{name: name, param: parameter})
	}
	return rules, nil
}

func isNilValidationTarget(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return isNilValue(reflected)
}

func isStruct(value any) bool {
	reflected := dereferenceValue(reflect.ValueOf(value))
	return reflected.IsValid() && reflected.Kind() == reflect.Struct
}

func dereferenceValue(value reflect.Value) reflect.Value {
	for value.IsValid() && (value.Kind() == reflect.Interface || value.Kind() == reflect.Ptr) {
		if value.IsNil() {
			return reflect.Value{}
		}
		value = value.Elem()
	}
	return value
}

func isNilValue(value reflect.Value) bool {
	if !value.IsValid() {
		return true
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return true
		}
		return isNilValue(value.Elem())
	}
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func joinValidationPath(parent, field string) string {
	if parent == "" {
		return field
	}
	return parent + "." + field
}

func hasValidationRule(rules []validationRule, name string) bool {
	return validationRuleIndex(rules, name) >= 0
}

func validationRuleIndex(rules []validationRule, name string) int {
	for index, rule := range rules {
		if rule.name == name {
			return index
		}
	}
	return -1
}
