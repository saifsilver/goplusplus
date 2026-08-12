package gpp

import (
	"cmp"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	maxValidationDepth      = 32
	maxValidationErrors     = 64
	maxValidationCollection = 10_000
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

// Error implements error for an internal validation-rule violation.
func (e *validationViolation) Error() string {
	return fmt.Sprintf("field %s failed %s", e.field, e.rule)
}

type validationConfigError struct {
	message string
}

// Error implements error for invalid validator configuration.
func (e *validationConfigError) Error() string { return e.message }

type validationVisit struct {
	typeOf  reflect.Type
	pointer uintptr
}

type validationFieldMetadata struct {
	index int
	name  string
	rules []validationRule
	skip  bool
}

type validationStructMetadata struct {
	fields []validationFieldMetadata
}

type validationMetadataResult struct {
	metadata *validationStructMetadata
	err      error
}

type validationState struct {
	violations    []FieldViolation
	visited       map[validationVisit]bool
	configuration error
	limitReported bool
}

var validationMetadataCache sync.Map

func validateStruct(value any) (result error) {
	defer func() {
		if recover() != nil {
			result = ErrInternal("Invalid validation configuration")
		}
	}()
	if isNilValidationTarget(value) {
		return ErrValidation([]FieldViolation{{Field: "request", Rule: "required", Message: "must not be nil"}})
	}
	if !isStruct(value) {
		return nil
	}

	state := validationState{visited: make(map[validationVisit]bool)}
	state.validateStructValue(dereferenceValue(reflect.ValueOf(value)), "", 0)
	if state.configuration != nil {
		return ErrInternal("Invalid validation configuration")
	}
	if len(state.violations) > 0 {
		return ErrValidation(state.violations)
	}
	return nil
}

func (state *validationState) validateStructValue(value reflect.Value, path string, depth int) {
	if state.done() || value.Type() == reflect.TypeFor[time.Time]() {
		return
	}
	if depth > maxValidationDepth {
		state.addLimitViolation(path, "max_depth", "validation nesting exceeds the supported limit")
		return
	}

	metadata, err := cachedValidationMetadata(value.Type())
	if err != nil {
		state.configuration = err
		return
	}
	for _, field := range metadata.fields {
		if state.done() {
			return
		}
		if field.skip {
			continue
		}
		state.validateField(value.Field(field.index), value, joinValidationPath(path, field.name), field.rules, depth)
	}
}

func (state *validationState) validateField(
	value reflect.Value,
	owner reflect.Value,
	path string,
	rules []validationRule,
	depth int,
) {
	if state.done() {
		return
	}

	diveIndex := validationRuleIndex(rules, "dive")
	limit := len(rules)
	if diveIndex >= 0 {
		limit = diveIndex
	}
	for _, rule := range rules[:limit] {
		switch rule.name {
		case "omitempty":
			if isEmptyValidationValue(value) {
				return
			}
			continue
		case "omitnil":
			if isNilValue(value) {
				return
			}
			continue
		}

		valid, err := applyValidationRule(value, owner, rule)
		if err != nil {
			state.configuration = err
			return
		}
		if !valid {
			state.addViolation(path, value, rule)
		}
	}

	if diveIndex >= 0 {
		state.validateDive(value, owner, path, rules[diveIndex+1:], depth+1)
		return
	}
	state.validateNestedValue(value, path, depth+1)
}

func (state *validationState) validateDive(
	value reflect.Value,
	owner reflect.Value,
	path string,
	rules []validationRule,
	depth int,
) {
	value = dereferenceValue(value)
	if !value.IsValid() || state.done() {
		return
	}

	switch value.Kind() {
	case reflect.Array, reflect.Slice:
		length := state.boundedCollectionLength(path, value.Len())
		for index := 0; index < length && !state.done(); index++ {
			state.validateField(value.Index(index), owner, fmt.Sprintf("%s[%d]", path, index), rules, depth)
		}
	case reflect.Map:
		state.validateMapDive(value, owner, path, rules, depth)
	default:
		state.configuration = &validationConfigError{message: "dive requires an array, slice, or map"}
	}
}

func (state *validationState) validateMapDive(
	value reflect.Value,
	owner reflect.Value,
	path string,
	rules []validationRule,
	depth int,
) {
	keyRules, valueRules, err := splitMapValidationRules(rules)
	if err != nil {
		state.configuration = err
		return
	}

	keys := boundedSortedMapKeys(value, state.boundedCollectionLength(path, value.Len()))
	for index, key := range keys {
		if state.done() {
			return
		}
		itemPath := fmt.Sprintf("%s[%d]", path, index)
		if len(keyRules) > 0 {
			state.validateField(key, owner, itemPath+".key", keyRules, depth)
		}
		state.validateField(value.MapIndex(key), owner, itemPath, valueRules, depth)
	}
}

func (state *validationState) validateNestedValue(value reflect.Value, path string, depth int) {
	for value.IsValid() && (value.Kind() == reflect.Interface || value.Kind() == reflect.Ptr) {
		if value.IsNil() {
			return
		}
		if value.Kind() == reflect.Ptr {
			visit := validationVisit{typeOf: value.Type(), pointer: value.Pointer()}
			if state.visited[visit] {
				return
			}
			state.visited[visit] = true
			defer delete(state.visited, visit)
		}
		value = value.Elem()
	}
	if !value.IsValid() || state.done() {
		return
	}

	switch value.Kind() {
	case reflect.Struct:
		state.validateStructValue(value, path, depth)
	case reflect.Array, reflect.Slice:
		length := state.boundedCollectionLength(path, value.Len())
		for index := 0; index < length && !state.done(); index++ {
			state.validateNestedValue(value.Index(index), fmt.Sprintf("%s[%d]", path, index), depth+1)
		}
	case reflect.Map:
		keys := boundedSortedMapKeys(value, state.boundedCollectionLength(path, value.Len()))
		for index, key := range keys {
			state.validateNestedValue(value.MapIndex(key), fmt.Sprintf("%s[%d]", path, index), depth+1)
		}
	}
}

func cachedValidationMetadata(typeOf reflect.Type) (*validationStructMetadata, error) {
	if cached, ok := validationMetadataCache.Load(typeOf); ok {
		result := cached.(validationMetadataResult)
		return result.metadata, result.err
	}

	metadata, err := buildValidationMetadata(typeOf)
	result := validationMetadataResult{metadata: metadata, err: err}
	actual, _ := validationMetadataCache.LoadOrStore(typeOf, result)
	stored := actual.(validationMetadataResult)
	return stored.metadata, stored.err
}

func buildValidationMetadata(typeOf reflect.Type) (*validationStructMetadata, error) {
	metadata := &validationStructMetadata{fields: make([]validationFieldMetadata, 0, typeOf.NumField())}
	for index := 0; index < typeOf.NumField(); index++ {
		field := typeOf.Field(index)
		if field.PkgPath != "" {
			continue
		}
		tag := field.Tag.Get("validate")
		rules, err := parseValidationRules(tag)
		if err != nil {
			return nil, err
		}
		metadata.fields = append(metadata.fields, validationFieldMetadata{
			index: index,
			name:  validationJSONFieldName(field),
			rules: rules,
			skip:  tag == "-",
		})
	}
	return metadata, nil
}

func validationJSONFieldName(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if name == "" || name == "-" {
		return field.Name
	}
	return name
}

func (state *validationState) addViolation(path string, value reflect.Value, rule validationRule) {
	if len(state.violations) >= maxValidationErrors {
		return
	}
	internal := newValidationViolation(path, value, rule).(*validationViolation)
	state.violations = append(state.violations, FieldViolation{
		Field: path, Rule: rule.name, Message: validationRuleMessage(internal),
	})
}

func (state *validationState) addLimitViolation(path, rule, message string) {
	if state.limitReported || len(state.violations) >= maxValidationErrors {
		return
	}
	state.limitReported = true
	state.violations = append(state.violations, FieldViolation{Field: path, Rule: rule, Message: message})
}

func (state *validationState) boundedCollectionLength(path string, length int) int {
	if length <= maxValidationCollection {
		return length
	}
	state.addLimitViolation(path, "max_items", "collection exceeds the validation traversal limit")
	return maxValidationCollection
}

func (state *validationState) done() bool {
	return state.configuration != nil || len(state.violations) >= maxValidationErrors
}

func boundedSortedMapKeys(value reflect.Value, limit int) []reflect.Value {
	keys := make([]reflect.Value, 0, limit)
	iterator := value.MapRange()
	for len(keys) < limit && iterator.Next() {
		keys = append(keys, iterator.Key())
	}
	slices.SortFunc(keys, func(a, b reflect.Value) int {
		return cmp.Compare(fmt.Sprint(a), fmt.Sprint(b))
	})
	return keys
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
	return isNilValue(reflect.ValueOf(value))
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

func validationRuleIndex(rules []validationRule, name string) int {
	for index, rule := range rules {
		if rule.name == name {
			return index
		}
	}
	return -1
}
