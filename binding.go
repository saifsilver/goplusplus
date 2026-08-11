package gpp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"
)

const (
	DefaultMaxJSONBodyBytes int64 = 1 << 20
	MaximumJSONBodyBytes    int64 = 64 << 20
)

// JSONBindingConfig controls request JSON decoding. Zero values retain secure defaults.
type JSONBindingConfig struct {
	MaxBodyBytes            int64
	AllowUnknownFields      bool
	AllowNonJSONContentType bool
}

func defaultJSONBindingConfig() JSONBindingConfig {
	return JSONBindingConfig{MaxBodyBytes: DefaultMaxJSONBodyBytes}
}

func normalizeJSONBindingConfig(config JSONBindingConfig) JSONBindingConfig {
	if config.MaxBodyBytes <= 0 {
		config.MaxBodyBytes = DefaultMaxJSONBodyBytes
	} else if config.MaxBodyBytes > MaximumJSONBodyBytes {
		config.MaxBodyBytes = MaximumJSONBodyBytes
	}
	return config
}

func (c *Context) jsonBindingConfig() JSONBindingConfig {
	if c.engine == nil {
		return defaultJSONBindingConfig()
	}
	return normalizeJSONBindingConfig(c.engine.JSONBinding)
}

func bindJSON(c *Context, target any) error {
	if !isValidBindingTarget(target) {
		return bindingProblem(http.StatusBadRequest, "invalid-target", "Invalid request target")
	}
	if c.Request == nil || c.Request.Body == nil {
		return bindingProblem(http.StatusBadRequest, "empty-body", "Request body is required")
	}
	defer c.Request.Body.Close()

	select {
	case <-c.Request.Context().Done():
		return c.Request.Context().Err()
	default:
	}

	config := c.jsonBindingConfig()
	if c.Request.ContentLength > config.MaxBodyBytes {
		return bindingProblem(http.StatusRequestEntityTooLarge, "body-too-large", "Request body exceeds the maximum allowed size")
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, config.MaxBodyBytes+1))
	if err != nil {
		if c.Request.Context().Err() != nil {
			return c.Request.Context().Err()
		}
		return bindingProblem(http.StatusBadRequest, "read-failed", "Request body could not be read")
	}
	if int64(len(body)) > config.MaxBodyBytes {
		return bindingProblem(http.StatusRequestEntityTooLarge, "body-too-large", "Request body exceeds the maximum allowed size")
	}
	if err := c.Request.Context().Err(); err != nil {
		return err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return bindingProblem(http.StatusBadRequest, "empty-body", "Request body is required")
	}
	if !config.AllowNonJSONContentType && !hasJSONContentType(c.Request.Header.Get("Content-Type")) {
		return bindingProblem(http.StatusUnsupportedMediaType, "content-type", "Content-Type must be application/json")
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	if !config.AllowUnknownFields {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(target); err != nil {
		return classifyJSONDecodeError(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return bindingProblem(http.StatusBadRequest, "trailing-data", "Request body must contain exactly one JSON document")
	}
	return nil
}

func isValidBindingTarget(target any) bool {
	if target == nil {
		return false
	}
	value := reflect.ValueOf(target)
	return value.Kind() == reflect.Ptr && !value.IsNil()
}

func hasJSONContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func classifyJSONDecodeError(err error) error {
	var syntaxError *json.SyntaxError
	if errors.As(err, &syntaxError) {
		return bindingProblem(http.StatusBadRequest, "malformed-json", "Request body contains malformed JSON")
	}
	var typeError *json.UnmarshalTypeError
	if errors.As(err, &typeError) {
		return bindingProblem(http.StatusBadRequest, "type-mismatch", "Request body contains a value with the wrong type")
	}
	if strings.HasPrefix(err.Error(), "json: unknown field ") {
		return bindingProblem(http.StatusBadRequest, "unknown-field", "Request body contains an unknown field")
	}
	return bindingProblem(http.StatusBadRequest, "malformed-json", "Request body contains invalid JSON")
}

func bindingProblem(status int, kind, detail string) *ProblemDetails {
	title := "Invalid request body"
	if status == http.StatusRequestEntityTooLarge {
		title = "Request body too large"
	} else if status == http.StatusUnsupportedMediaType {
		title = "Unsupported media type"
	}
	return &ProblemDetails{
		Type:  "https://goplusplus.dev/errors/binding/" + kind,
		Title: title, Status: status, Detail: detail,
	}
}
