package gpp

import (
	"fmt"
	"net/http"
)

// ProblemDetails implements the RFC 7807 standard specification for HTTP API Problem Details.
type ProblemDetails struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance,omitempty"`
	TraceID  string `json:"trace_id,omitempty"`
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

// ErrInternal creates an RFC 7807 500 Internal Server Error problem details error.
func ErrInternal(detail string) *ProblemDetails {
	return &ProblemDetails{
		Type:   "https://goplusplus.dev/errors/internal-error",
		Title:  "Internal Server Error",
		Status: http.StatusInternalServerError,
		Detail: detail,
	}
}
