package gpp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/middleware"
)

func TestInternalFailurePreservesCauseAndRedactsResponse(t *testing.T) {
	cause := errors.New("SELECT secret FROM users at /private/app.db")
	failure := gpp.NewInternalError("users.load", cause, gpp.WithErrorCategory("database"))
	if !errors.Is(failure, cause) {
		t.Fatal("internal failure did not preserve cause")
	}
	app := gpp.New()
	app.Use(middleware.RequestID())
	app.GET("/users", func(*gpp.Context) error { return failure })
	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/users", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "SELECT") || strings.Contains(recorder.Body.String(), "/private") {
		t.Fatalf("response leaked cause: %s", recorder.Body.String())
	}
	var problem gpp.ProblemDetails
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.TraceID == "" || problem.TraceID != recorder.Header().Get("X-Request-ID") {
		t.Fatalf("request ID not propagated: %#v", problem)
	}
}

func TestInternalFailureLogsOnceWithRecommendedMiddleware(t *testing.T) {
	handler := &countingLogHandler{}
	previous := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(previous) })
	app := gpp.New()
	app.Use(middleware.RequestID(), middleware.Logger(), middleware.Recovery())
	app.GET("/failure", func(*gpp.Context) error {
		return gpp.NewInternalError("orders.load", errors.New("database offline"))
	})
	app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/failure", nil))
	if got := handler.errorCount(); got != 1 {
		t.Fatalf("internal failure error logs = %d, want 1", got)
	}
}

func TestErrorHandlerDoesNotOverwriteStartedResponse(t *testing.T) {
	app := gpp.New()
	app.GET("/partial", func(c *gpp.Context) error {
		_, _ = c.Writer.Write([]byte("partial"))
		return errors.New("late failure")
	})
	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/partial", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "partial" {
		t.Fatalf("started response was overwritten: %d %q", recorder.Code, recorder.Body.String())
	}
}

type countingLogHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (handler *countingLogHandler) Enabled(context.Context, slog.Level) bool { return true }
func (handler *countingLogHandler) Handle(_ context.Context, record slog.Record) error {
	handler.mu.Lock()
	handler.records = append(handler.records, record.Clone())
	handler.mu.Unlock()
	return nil
}
func (handler *countingLogHandler) WithAttrs([]slog.Attr) slog.Handler { return handler }
func (handler *countingLogHandler) WithGroup(string) slog.Handler      { return handler }
func (handler *countingLogHandler) errorCount() int {
	handler.mu.Lock()
	defer handler.mu.Unlock()
	count := 0
	for _, record := range handler.records {
		if record.Level >= slog.LevelError {
			count++
		}
	}
	return count
}

func TestBindNormalizeAndValidateOrdering(t *testing.T) {
	type request struct {
		Title string `json:"title" validate:"required"`
	}
	app := gpp.New()
	app.POST("/items", func(c *gpp.Context) error {
		var input request
		err := c.BindNormalizeAndValidate(&input, func(context.Context) error {
			input.Title = strings.TrimSpace(input.Title)
			return nil
		})
		if err != nil {
			return err
		}
		return c.OK(input)
	})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/items", bytes.NewBufferString(`{"title":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestStrictParametersAndPagination(t *testing.T) {
	app := gpp.New()
	app.GET("/items/:id", func(c *gpp.Context) error {
		id, err := c.ParamPositiveInt64("id")
		if err != nil {
			return err
		}
		pagination, err := (gpp.PaginationPolicy{DefaultLimit: 10, MaximumLimit: 50, Strict: true}).Parse(c)
		if err != nil {
			return err
		}
		return c.OK(gpp.H{"id": id, "pagination": pagination})
	})
	for _, target := range []string{"/items/nope", "/items/0", "/items/-1", "/items/9223372036854775808", "/items/1?limit=51"} {
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d", target, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/items/42?page=3&limit=10", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"offset":20`) {
		t.Fatalf("valid pagination failed: %d %s", recorder.Code, recorder.Body.String())
	}
}
