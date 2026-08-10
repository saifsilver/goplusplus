package realtime

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/saifsilver/goplusplus"
)

// SSEEvent represents a single Server-Sent Event frame.
type SSEEvent struct {
	Event string `json:"event,omitempty"`
	Data  any    `json:"data"`
	ID    string `json:"id,omitempty"`
}

// StreamSSE streams events from a channel continuously using HTTP text/event-stream headers.
func StreamSSE(c *gpp.Context, eventChan <-chan any) error {
	c.SetHeader("Content-Type", "text/event-stream")
	c.SetHeader("Cache-Control", "no-cache")
	c.SetHeader("Connection", "keep-alive")
	c.SetHeader("Access-Control-Allow-Origin", "*")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if ok {
		flusher.Flush()
	}

	for {
		select {
		case <-c.Request.Context().Done():
			return nil
		case item, open := <-eventChan:
			if !open {
				return nil
			}

			var payload string
			if evt, ok := item.(SSEEvent); ok {
				dataBytes, _ := json.Marshal(evt.Data)
				if evt.Event != "" {
					payload += fmt.Sprintf("event: %s\n", evt.Event)
				}
				if evt.ID != "" {
					payload += fmt.Sprintf("id: %s\n", evt.ID)
				}
				payload += fmt.Sprintf("data: %s\n\n", string(dataBytes))
			} else {
				dataBytes, _ := json.Marshal(item)
				payload = fmt.Sprintf("data: %s\n\n", string(dataBytes))
			}

			_, _ = c.Writer.Write([]byte(payload))
			if ok && flusher != nil {
				flusher.Flush()
			}
		}
	}
}
