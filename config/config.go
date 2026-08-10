package config

import (
	"bufio"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	mu   sync.RWMutex
	vars = make(map[string]string)
)

// Load parses a .env file and loads key-value pairs into the environment store.
func Load(filepath ...string) error {
	path := ".env"
	if len(filepath) > 0 && filepath[0] != "" {
		path = filepath[0]
	}

	file, err := os.Open(path)
	if err != nil {
		return nil // Soft fallback if .env file is omitted
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	mu.Lock()
	defer mu.Unlock()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			vars[k] = v
			_ = os.Setenv(k, v)
		}
	}
	return nil
}

// GetString retrieves a string config value from environment or loaded .env file.
func GetString(key string, defaultValue ...string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	mu.RLock()
	val, ok := vars[key]
	mu.RUnlock()
	if ok && val != "" {
		return val
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

// GetInt retrieves an integer config value.
func GetInt(key string, defaultValue ...int) int {
	str := GetString(key)
	if str == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	val, err := strconv.Atoi(str)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	return val
}

// GetBool retrieves a boolean config value.
func GetBool(key string, defaultValue ...bool) bool {
	str := GetString(key)
	if str == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
	}
	val, err := strconv.ParseBool(str)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
	}
	return val
}

// GetDuration retrieves a time.Duration config value (e.g. "10s", "5m").
func GetDuration(key string, defaultValue ...time.Duration) time.Duration {
	str := GetString(key)
	if str == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	val, err := time.ParseDuration(str)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	return val
}

// Get is a shorthand alias for GetString.
func Get(key string, defaultValue ...string) string {
	return GetString(key, defaultValue...)
}

// Env is a shorthand alias for GetString.
func Env(key string, defaultValue ...string) string {
	return GetString(key, defaultValue...)
}

// MaskSecret redacts sensitive credentials (e.g. passwords, AWS keys) for logging safety.
func MaskSecret(secret string) string {
	if len(secret) <= 6 {
		return "******"
	}
	return secret[:3] + "..." + secret[len(secret)-3:]
}

// LoadAndUnmarshal parses a .env file and unmarshals environment variables into the target struct pointer.
func LoadAndUnmarshal(target any, filepath ...string) error {
	_ = Load(filepath...)
	return Unmarshal(target)
}

// Bind is a convenient alias for LoadAndUnmarshal.
func Bind(target any, filepath ...string) error {
	return LoadAndUnmarshal(target, filepath...)
}

// LoadStruct parses a .env file and populates a new struct pointer of type T.
func LoadStruct[T any](filepath ...string) (*T, error) {
	var cfg T
	if err := LoadAndUnmarshal(&cfg, filepath...); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// MustLoad parses a .env file and populates a new struct of type T, panicking on unmarshal failure.
func MustLoad[T any](filepath ...string) *T {
	cfg, err := LoadStruct[T](filepath...)
	if err != nil {
		panic(err)
	}
	return cfg
}

// Unmarshal maps environment variables into a struct pointer using `env:"KEY"` tags or automatic SNAKE_CASE field names.
func Unmarshal(v any) error {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("config: Unmarshal target must be a pointer to a struct")
	}

	structVal := val.Elem()
	structTyp := structVal.Type()
	durationTyp := reflect.TypeOf(time.Duration(0))

	for i := 0; i < structVal.NumField(); i++ {
		field := structVal.Field(i)
		fieldType := structTyp.Field(i)

		if !field.CanSet() {
			continue
		}

		envKey := fieldType.Tag.Get("env")
		defaultVal := fieldType.Tag.Get("default")

		var envVal string
		if envKey != "" {
			envVal = GetString(envKey, defaultVal)
		} else {
			// Auto snake_case field mapping (e.g. JWTSecret -> JWT_SECRET, DBPath -> DB_PATH)
			snakeKey := toSnakeCase(fieldType.Name)
			envVal = GetString(snakeKey)
			if envVal == "" {
				upperKey := strings.ToUpper(fieldType.Name)
				if upperKey != snakeKey {
					envVal = GetString(upperKey)
				}
			}
			if envVal == "" && defaultVal != "" {
				envVal = defaultVal
			}
		}

		if envVal == "" {
			continue
		}

		if field.Type() == durationTyp {
			if durVal, err := time.ParseDuration(envVal); err == nil {
				field.Set(reflect.ValueOf(durVal))
			}
			continue
		}

		switch field.Kind() {
		case reflect.String:
			field.SetString(envVal)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if intVal, err := strconv.ParseInt(envVal, 10, 64); err == nil {
				field.SetInt(intVal)
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if uintVal, err := strconv.ParseUint(envVal, 10, 64); err == nil {
				field.SetUint(uintVal)
			}
		case reflect.Float32, reflect.Float64:
			if floatVal, err := strconv.ParseFloat(envVal, 64); err == nil {
				field.SetFloat(floatVal)
			}
		case reflect.Bool:
			if boolVal, err := strconv.ParseBool(envVal); err == nil {
				field.SetBool(boolVal)
			}
		}
	}
	return nil
}

func toSnakeCase(s string) string {
	var builder strings.Builder
	runes := []rune(s)
	n := len(runes)
	for i := 0; i < n; i++ {
		r := runes[i]
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := runes[i-1]
			nextIsLower := i+1 < n && runes[i+1] >= 'a' && runes[i+1] <= 'z'
			prevIsLower := prev >= 'a' && prev <= 'z'
			prevIsDigit := prev >= '0' && prev <= '9'
			if prevIsLower || prevIsDigit || (nextIsLower && prev >= 'A' && prev <= 'Z') {
				builder.WriteRune('_')
			}
		}
		builder.WriteRune(r)
	}
	return strings.ToUpper(builder.String())
}

