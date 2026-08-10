package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/dbcore"
	"github.com/saifsilver/goplusplus/middleware"
)

type BatchUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func main() {
	ctx := context.Background()

	// 1. Initialize dbcore Database Client
	db, _ := dbcore.NewClient(ctx, dbcore.Config{})

	// 2. Initialize goplusplus App Engine
	app := gpp.New()

	app.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
	)

	// 3. Endpoint demonstrating Auto-BindAndValidate, Parallel Concurrency, and Parallel DB Actions
	app.POST("/api/v1/dashboard", func(c *gpp.Context) error {
		var req BatchUserRequest

		// 1-liner Auto-Decode JSON AND Validate Struct
		if err := c.BindAndValidate(&req); err != nil {
			return err
		}

		var profileData, orderData, metricData gpp.H

		start := time.Now()

		// Execute 3 concurrent tasks in parallel goroutines using c.Parallel()
		err := c.Parallel(
			func(c *gpp.Context) error {
				time.Sleep(20 * time.Millisecond) // Simulated task 1
				profileData = gpp.H{"user": req.Name, "email": req.Email}
				return nil
			},
			func(c *gpp.Context) error {
				time.Sleep(20 * time.Millisecond) // Simulated task 2
				orderData = gpp.H{"total_orders": 12, "active_order_id": "ord_88192"}
				return nil
			},
			func(c *gpp.Context) error {
				time.Sleep(20 * time.Millisecond) // Simulated task 3
				metricData = gpp.H{"reward_points": 4500, "membership": "VIP Platinum"}
				return nil
			},
		)

		if err != nil {
			return gpp.ErrInternal("Parallel task execution failed: " + err.Error())
		}

		// Execute 2 concurrent parallel DB queries across replicas in 1 step!
		_ = db.ParallelQuery(c.Request.Context(),
			dbcore.ParallelTask{QueryName: "user.fetch_details", SQL: "SELECT id, name FROM users WHERE email=$1", Args: []any{req.Email}},
			dbcore.ParallelTask{QueryName: "order.fetch_recent", SQL: "SELECT id, amount FROM orders WHERE status=$1", Args: []any{"COMPLETED"}},
		)

		totalLatency := time.Since(start)

		return c.JSON(http.StatusOK, gpp.H{
			"status":             "success",
			"parallel_latency":   totalLatency.String(),
			"profile":            profileData,
			"orders":             orderData,
			"metrics":            metricData,
			"parallel_execution": "3 tasks executed concurrently in ~20ms instead of 60ms sequential!",
		})
	})

	fmt.Println("🚀 Starting goplusplus Parallel Concurrency Server on http://localhost:8080")
	fmt.Println("   • Endpoint: POST http://localhost:8080/api/v1/dashboard")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
