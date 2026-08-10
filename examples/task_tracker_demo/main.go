package main

import (
	"fmt"
	"net/http"
	"sync/atomic"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/middleware"
)

var attemptCounter int32

func main() {
	app := gpp.New()

	app.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
	)

	// Dispatch tracked background task with auto-retry on failure
	app.POST("/api/v1/tasks/email", func(c *gpp.Context) error {
		taskID := c.AsyncTask("send_welcome_email", func(c *gpp.Context) error {
			current := atomic.AddInt32(&attemptCounter, 1)
			if current < 3 {
				return fmt.Errorf("Simulated transient SMTP network error (Attempt #%d)", current)
			}
			fmt.Println("  ✅ Email delivered successfully on attempt #3!")
			return nil
		})

		return c.JSON(http.StatusAccepted, gpp.H{
			"status":            "task_dispatched",
			"task_id":           taskID,
			"status_query_url":  "/api/v1/tasks/" + taskID,
			"auto_retry_policy": "Retries up to 3 times automatically on failure!",
		})
	})

	// Query task status by ID
	app.GET("/api/v1/tasks/:id", func(c *gpp.Context) error {
		taskID := c.Param("id")
		taskInfo, ok := c.GetTaskStatus(taskID)
		if !ok {
			return gpp.ErrNotFound("Task ID not found")
		}
		return c.JSON(http.StatusOK, gpp.H{
			"task": taskInfo,
		})
	})

	fmt.Println("🚀 Starting goplusplus Task Tracker Server on http://localhost:8080")
	fmt.Println("   • Dispatch Task: POST http://localhost:8080/api/v1/tasks/email")
	fmt.Println("   • Query Status:  GET  http://localhost:8080/api/v1/tasks/:id")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
