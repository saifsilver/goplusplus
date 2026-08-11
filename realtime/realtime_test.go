package realtime_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/realtime"
)

func TestStreamSSE(t *testing.T) {
	app := gpp.New()

	app.GET("/events", func(c *gpp.Context) error {
		ch := make(chan any, 2)
		ch <- realtime.SSEEvent{Event: "ping", Data: "hello", ID: "1"}
		ch <- "simple message"
		close(ch)

		return realtime.StreamSSE(c, ch)
	})

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	w := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)

	app.ServeHTTP(w, req)

	if w.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got '%s'", w.Header().Get("Content-Type"))
	}
}

func TestWebSocketUpgradeMissingKey(t *testing.T) {
	app := gpp.New()

	app.GET("/ws", func(c *gpp.Context) error {
		_, err := realtime.Upgrade(c)
		if err != nil {
			return err
		}
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request when Sec-WebSocket-Key is missing, got %d", w.Code)
	}
}
