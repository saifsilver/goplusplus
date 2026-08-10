package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/di"
	"github.com/saifsilver/goplusplus/middleware"
	"github.com/saifsilver/goplusplus/resilience"
	"github.com/saifsilver/goplusplus/saga"
)

func main() {
	// 1. Uber FX-style Dependency Injection Container
	container := di.New()

	container.OnStart(func() error {
		fmt.Println("⚡ DI Container Lifecycle: All services initialized cleanly")
		return nil
	})
	_ = container.Start()
	defer func() { _ = container.Stop() }()

	// 2. Resilience Suite: Circuit Breaker & Adaptive Limiter
	cb := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
		FailureThreshold: 3,
		ResetTimeout:     5 * time.Second,
	})
	limiter := resilience.NewAdaptiveLimiter(500)

	// 3. goplusplus App Engine
	app := gpp.New()

	app.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
		limiter.Middleware(),
	)

	// 4. Saga Transaction Endpoint
	app.POST("/api/v1/checkout", func(c *gpp.Context) error {
		// Protected execution by Circuit Breaker
		err := cb.Execute(func() error {
			// Multi-microservice Saga Distributed Transaction with Auto-Compensation
			sagaCoord := saga.NewCoordinator()

			// Step 1: Reserve Inventory
			sagaCoord.AddStep("reserve_inventory",
				func(ctx context.Context) error {
					fmt.Println("  1. Reserved inventory items")
					return nil
				},
				func(ctx context.Context) error {
					fmt.Println("  1. [REVERSE COMPENSATE] Released reserved inventory items")
					return nil
				},
			)

			// Step 2: Charge Payment
			sagaCoord.AddStep("charge_payment",
				func(ctx context.Context) error {
					fmt.Println("  2. Payment charged successfully")
					return nil
				},
				func(ctx context.Context) error {
					fmt.Println("  2. [REVERSE COMPENSATE] Refunded payment charge")
					return nil
				},
			)

			return sagaCoord.Execute(c.Request.Context())
		})

		if err != nil {
			return gpp.ErrInternal("Checkout failed: " + err.Error())
		}

		return c.JSON(http.StatusOK, gpp.H{
			"status":  "success",
			"message": "Checkout completed via Saga Transaction!",
		})
	})

	fmt.Println("🚀 Starting goplusplus Hyper-Scale Enterprise Server on http://localhost:8080")
	fmt.Println("   • Saga Transaction Endpoint: POST http://localhost:8080/api/v1/checkout")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
