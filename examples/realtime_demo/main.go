package main

import (
	"fmt"
	"net/http"
	"time"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/middleware"
	"github.com/saifsilver/goplusplus/realtime"
)

func main() {
	app := gpp.New()

	app.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
	)

	// 1. Server-Sent Events (SSE) Streaming Endpoint
	app.GET("/api/v1/sse", func(c *gpp.Context) error {
		eventChan := make(chan any)

		go func() {
			defer close(eventChan)
			for i := 1; i <= 5; i++ {
				time.Sleep(500 * time.Millisecond)
				eventChan <- realtime.SSEEvent{
					Event: "notification",
					ID:    fmt.Sprintf("evt_%d", i),
					Data:  gpp.H{"message": fmt.Sprintf("Live Notification #%d", i), "time": time.Now().Format(time.RFC3339)},
				}
			}
		}()

		return realtime.StreamSSE(c, eventChan)
	})

	// 2. WebSocket Real-Time Connection Endpoint
	app.GET("/api/v1/ws", func(c *gpp.Context) error {
		conn, err := realtime.Upgrade(c)
		if err != nil {
			return err
		}
		defer conn.Close()

		_ = conn.WriteMessage("Connected to goplusplus WebSocket Real-Time Server!")

		for {
			msg, err := conn.ReadMessage()
			if err != nil {
				break
			}
			_ = conn.WriteMessage("Echo: " + msg)
		}
		return nil
	})

	fmt.Println("🚀 Starting goplusplus Real-Time Streaming Server on http://localhost:8080")
	fmt.Println("   • SSE Event Stream: GET http://localhost:8080/api/v1/sse")
	fmt.Println("   • WebSocket Engine: ws://localhost:8080/api/v1/ws")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
