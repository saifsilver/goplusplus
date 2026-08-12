package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LoaderOptions selects explicit environment values and an optional dotenv file.
type LoaderOptions struct {
	Environment    map[string]string
	DotEnvPath     string
	OptionalDotEnv bool
}

// Loader is an immutable, process-environment-independent configuration source.
type Loader struct {
	values map[string]string
}

// LoadValue returns a populated value only after normalization and validation
// succeed, avoiding exposure of partially loaded configuration.
func LoadValue[T any](loader *Loader, normalize func(*T) error, validate func(T) error) (T, error) {
	var value T
	var zero T
	if loader == nil {
		return value, errors.New("config: loader is nil")
	}
	if err := unmarshalLookup(&value, func(key string) string { return loader.values[key] }); err != nil {
		return value, err
	}
	if normalize != nil {
		if err := normalize(&value); err != nil {
			return zero, fmt.Errorf("config: normalization failed: %w", err)
		}
	}
	if validate != nil {
		if err := validate(value); err != nil {
			return zero, fmt.Errorf("config: validation failed: %w", err)
		}
	}
	return value, nil
}

// NewLoader constructs an immutable loader; environment values override dotenv values.
func NewLoader(options LoaderOptions) (*Loader, error) {
	fileValues := map[string]string{}
	if options.DotEnvPath != "" {
		parsed, err := readDotEnv(options.DotEnvPath)
		if err != nil {
			if !(options.OptionalDotEnv && errors.Is(err, os.ErrNotExist)) {
				return nil, err
			}
		} else {
			fileValues = parsed
		}
	}
	environment := options.Environment
	if environment == nil {
		environment = currentEnvironment()
	}
	values := make(map[string]string, len(fileValues)+len(environment))
	for key, value := range fileValues {
		values[key] = value
	}
	for key, value := range environment {
		values[key] = value
	}
	return &Loader{values: values}, nil
}

// Load populates target, then runs optional normalization and validation callbacks.
func (loader *Loader) Load(target any, normalize func() error, validate func() error) error {
	if loader == nil {
		return errors.New("config: loader is nil")
	}
	if err := unmarshalLookup(target, func(key string) string { return loader.values[key] }); err != nil {
		return err
	}
	if normalize != nil {
		if err := normalize(); err != nil {
			return fmt.Errorf("config: normalization failed: %w", err)
		}
	}
	if validate != nil {
		if err := validate(); err != nil {
			return fmt.Errorf("config: validation failed: %w", err)
		}
	}
	return nil
}

func currentEnvironment() map[string]string {
	values := make(map[string]string)
	for _, item := range os.Environ() {
		parts := strings.SplitN(item, "=", 2)
		values[parts[0]] = parts[1]
	}
	return values
}

var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func readDotEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: open .env file: %w", err)
	}
	defer file.Close()
	return parseDotEnv(file)
}

func parseDotEnv(reader io.Reader) (map[string]string, error) {
	values := make(map[string]string)
	scanner := bufio.NewScanner(reader)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 || !envKeyPattern.MatchString(strings.TrimSpace(parts[0])) {
			return nil, fmt.Errorf("config: malformed .env line %d", lineNumber)
		}
		values[strings.TrimSpace(parts[0])] = strings.Trim(strings.TrimSpace(parts[1]), `"'`)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("config: read .env file: %w", err)
	}
	return values, nil
}

func unmarshalLookup(target any, lookup func(string) string) error {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Ptr || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return errors.New("config: target must be a non-nil pointer to a struct")
	}
	return populateStruct(value.Elem(), lookup)
}

func populateStruct(value reflect.Value, lookup func(string) string) error {
	typeOf := value.Type()
	durationType := reflect.TypeOf(time.Duration(0))
	for index := 0; index < value.NumField(); index++ {
		field, definition := value.Field(index), typeOf.Field(index)
		if !field.CanSet() {
			continue
		}
		key, fallback, required, err := parseConfigField(definition)
		if err != nil {
			return err
		}
		raw := lookup(key)
		if raw == "" {
			raw = fallback
		}
		if raw == "" {
			if required {
				return fmt.Errorf("config: required field %q is missing", definition.Name)
			}
			continue
		}
		if field.Type() == durationType {
			parsed, err := time.ParseDuration(raw)
			if err != nil {
				return fmt.Errorf("config: field %q must be a duration", definition.Name)
			}
			field.SetInt(int64(parsed))
			continue
		}
		if err := setConfigValue(field, raw); err != nil {
			return fmt.Errorf("config: field %q has an invalid %s value", definition.Name, field.Kind())
		}
	}
	return nil
}

func parseConfigField(field reflect.StructField) (string, string, bool, error) {
	tag := field.Tag.Get("env")
	if tag == "" {
		tag = field.Tag.Get("config")
	}
	key, fallback, required := "", field.Tag.Get("default"), field.Tag.Get("required") == "true" || field.Tag.Get("required") == "1"
	if tag != "" {
		parts := strings.Split(tag, ",")
		key = strings.TrimSpace(parts[0])
		for _, option := range parts[1:] {
			option = strings.TrimSpace(option)
			switch {
			case option == "required":
				required = true
			case strings.HasPrefix(option, "default="):
				fallback = strings.TrimPrefix(option, "default=")
			case option != "":
				return "", "", false, fmt.Errorf("config: field %q has unknown option %q", field.Name, option)
			}
		}
	}
	if key == "" {
		key = toSnakeCase(field.Name)
	}
	if !envKeyPattern.MatchString(key) {
		return "", "", false, fmt.Errorf("config: field %q has invalid environment key", field.Name)
	}
	return key, fallback, required, nil
}

func setConfigValue(field reflect.Value, raw string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(raw)
	case reflect.Bool:
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		field.SetBool(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := strconv.ParseInt(raw, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetInt(value)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value, err := strconv.ParseUint(raw, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetUint(value)
	case reflect.Float32, reflect.Float64:
		value, err := strconv.ParseFloat(raw, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetFloat(value)
	default:
		return errors.New("unsupported type")
	}
	return nil
}

// Required rejects an empty or whitespace-only configuration value.
func Required(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("field %q is required", field)
	}
	return nil
}

// StringLength validates a configuration string using Unicode code-point length.
func StringLength(field, value string, minimum, maximum int) error {
	if minimum < 0 || maximum < minimum {
		return fmt.Errorf("field %q has invalid length bounds", field)
	}
	length := len([]rune(value))
	if length < minimum || length > maximum {
		return fmt.Errorf("field %q length must be between %d and %d", field, minimum, maximum)
	}
	return nil
}

// IntRange validates an integer against inclusive bounds.
func IntRange(field string, value, minimum, maximum int64) error {
	if maximum < minimum || value < minimum || value > maximum {
		return fmt.Errorf("field %q must be between %d and %d", field, minimum, maximum)
	}
	return nil
}

// OneOf validates that value exactly matches an allowed string.
func OneOf(field, value string, allowed ...string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("field %q is not an allowed value", field)
}

// Port validates an integer TCP or UDP port number.
func Port(field string, value int) error { return IntRange(field, int64(value), 1, 65535) }
