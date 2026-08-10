package gpp

import (
	"fmt"
)

// H is a convenience alias for map[string]any, enabling ultra-concise JSON responses.
type H map[string]any

// Map is an alias for map[string]any.
type Map = map[string]any

// HTTPError represents a standardized structured framework error.
type HTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Error implements the standard Go error interface.
func (e *HTTPError) Error() string {
	if e.Details != nil {
		return fmt.Sprintf("HTTP %d: %s (%v)", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("HTTP %d: %s", e.Code, e.Message)
}

// NewHTTPError constructs a new HTTPError with status code, message, and optional details.
func NewHTTPError(code int, message string, details ...any) *HTTPError {
	err := &HTTPError{
		Code:    code,
		Message: message,
	}
	if len(details) > 0 {
		err.Details = details[0]
	}
	return err
}
