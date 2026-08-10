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

// MaskSecret redacts sensitive credentials (e.g. passwords, AWS keys) for logging safety.
func MaskSecret(secret string) string {
	if len(secret) <= 6 {
		return "******"
	}
	return secret[:3] + "..." + secret[len(secret)-3:]
}

// Unmarshal maps environment variables into a struct pointer using `env:"KEY"` struct tags.
func Unmarshal(v any) error {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("config: Unmarshal target must be a pointer to a struct")
	}

	structVal := val.Elem()
	structTyp := structVal.Type()

	for i := 0; i < structVal.NumField(); i++ {
		field := structVal.Field(i)
		fieldType := structTyp.Field(i)

		envKey := fieldType.Tag.Get("env")
		if envKey == "" {
			continue
		}

		defaultVal := fieldType.Tag.Get("default")
		envVal := GetString(envKey, defaultVal)

		if envVal != "" && field.CanSet() {
			switch field.Kind() {
			case reflect.String:
				field.SetString(envVal)
			case reflect.Int, reflect.Int64:
				if intVal, err := strconv.ParseInt(envVal, 10, 64); err == nil {
					field.SetInt(intVal)
				}
			case reflect.Bool:
				if boolVal, err := strconv.ParseBool(envVal); err == nil {
					field.SetBool(boolVal)
				}
			}
		}
	}
	return nil
}
