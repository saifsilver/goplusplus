package gpp

import (
	"errors"
	"fmt"
	"net/http"
)

// ProblemDetails implements the RFC 7807 standard specification for HTTP API Problem Details.
type ProblemDetails struct {
	Type     string           `json:"type"`
	Title    string           `json:"title"`
	Status   int              `json:"status"`
	Detail   string           `json:"detail"`
	Instance string           `json:"instance,omitempty"`
	TraceID  string           `json:"trace_id,omitempty"`
	Errors   []FieldViolation `json:"errors,omitempty"`
}

// FieldViolation is a machine-readable request validation failure. Values are
// intentionally excluded to avoid exposing passwords, tokens, or personal data.
type FieldViolation struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Error implements standard Go error interface.
func (p *ProblemDetails) Error() string {
	return fmt.Sprintf("HTTP %d [%s]: %s", p.Status, p.Title, p.Detail)
}

// ErrNotFound creates an RFC 7807 404 Not Found problem details error.
func ErrNotFound(detail string) *ProblemDetails {
	return &ProblemDetails{
		Type:   "https://goplusplus.dev/errors/not-found",
		Title:  "Resource Not Found",
		Status: http.StatusNotFound,
		Detail: detail,
	}
}

// ErrBadRequest creates an RFC 7807 400 Bad Request problem details error.
func ErrBadRequest(detail string) *ProblemDetails {
	return &ProblemDetails{
		Type:   "https://goplusplus.dev/errors/bad-request",
		Title:  "Bad Request",
		Status: http.StatusBadRequest,
		Detail: detail,
	}
}

// ErrValidation creates a structured RFC 7807 validation response.
func ErrValidation(violations []FieldViolation) *ProblemDetails {
	return &ProblemDetails{
		Type:   "https://goplusplus.dev/errors/validation",
		Title:  "Request validation failed",
		Status: http.StatusBadRequest,
		Detail: "One or more fields are invalid",
		Errors: append([]FieldViolation(nil), violations...),
	}
}

// ErrUnauthorized creates an RFC 7807 401 Unauthorized problem details error.
func ErrUnauthorized(detail string) *ProblemDetails {
	return &ProblemDetails{
		Type:   "https://goplusplus.dev/errors/unauthorized",
		Title:  "Unauthorized Access",
		Status: http.StatusUnauthorized,
		Detail: detail,
	}
}

// ErrForbidden creates an RFC 7807 403 Forbidden problem details error.
func ErrForbidden(detail string) *ProblemDetails {
	return &ProblemDetails{
		Type:   "https://goplusplus.dev/errors/forbidden",
		Title:  "Access Forbidden",
		Status: http.StatusForbidden,
		Detail: detail,
	}
}

// ErrConflict creates an RFC 7807 409 Conflict problem details error.
func ErrConflict(detail string) *ProblemDetails {
	return &ProblemDetails{
		Type:   "https://goplusplus.dev/errors/conflict",
		Title:  "Resource Conflict",
		Status: http.StatusConflict,
		Detail: detail,
	}
}

// ErrRequestTimeout creates an RFC 7807 504 Gateway Timeout error.
func ErrRequestTimeout(detail string) *ProblemDetails {
	return &ProblemDetails{
		Type: "https://goplusplus.dev/errors/request-timeout", Title: "Request Timeout",
		Status: http.StatusGatewayTimeout, Detail: detail,
	}
}

// ErrInternal creates an RFC 7807 500 Internal Server Error problem details error.
func ErrInternal(detail string) *ProblemDetails {
	return &ProblemDetails{
		Type:   "https://goplusplus.dev/errors/internal-error",
		Title:  "Internal Server Error",
		Status: http.StatusInternalServerError,
		Detail: detail,
	}
}

// InternalFailure is a causal server-side failure. Only PublicDetail and the
// stable category are eligible for client rendering; Cause and Attributes are
// logged internally.
type InternalFailure struct {
	Operation    string
	Category     string
	Cause        error
	Attributes   map[string]any
	PublicDetail string
	Status       int
}

// Error returns a sanitized internal failure summary.
func (e *InternalFailure) Error() string {
	return fmt.Sprintf("internal failure in %s [%s]", e.Operation, e.Category)
}

// Unwrap returns the causal internal error.
func (e *InternalFailure) Unwrap() error { return e.Cause }

// InternalErrorOption configures a causal internal failure.
type InternalErrorOption func(*InternalFailure)

// WithErrorCategory sets a stable internal error category.
func WithErrorCategory(category string) InternalErrorOption {
	return func(failure *InternalFailure) {
		if category != "" {
			failure.Category = category
		}
	}
}

// WithPublicDetail opts into a caller-authored, non-sensitive 5xx detail.
func WithPublicDetail(detail string) InternalErrorOption {
	return func(failure *InternalFailure) { failure.PublicDetail = detail }
}

// WithSafeAttributes copies explicitly non-sensitive structured log attributes.
func WithSafeAttributes(attributes map[string]any) InternalErrorOption {
	return func(failure *InternalFailure) {
		failure.Attributes = make(map[string]any, len(attributes))
		for key, value := range attributes {
			failure.Attributes[key] = value
		}
	}
}

// WithInternalStatus sets a 5xx response status for the internal failure.
func WithInternalStatus(status int) InternalErrorOption {
	return func(failure *InternalFailure) {
		if status >= http.StatusInternalServerError && status <= 599 {
			failure.Status = status
		}
	}
}

// NewInternalError preserves cause identity while keeping the response safe.
func NewInternalError(operation string, cause error, options ...InternalErrorOption) *InternalFailure {
	if cause == nil {
		cause = errors.New("unspecified internal failure")
	}
	failure := &InternalFailure{
		Operation: operation,
		Category:  "internal_error",
		Cause:     cause,
		Status:    http.StatusInternalServerError,
	}
	for _, option := range options {
		if option != nil {
			option(failure)
		}
	}
	return failure
}

// IsInternalFailure reports whether err represents a server-side failure.
func IsInternalFailure(err error) bool {
	var failure *InternalFailure
	if errors.As(err, &failure) {
		return true
	}
	var problem *ProblemDetails
	return errors.As(err, &problem) && problem.Status >= http.StatusInternalServerError
}
