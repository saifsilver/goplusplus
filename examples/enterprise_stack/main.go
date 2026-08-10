package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/cache"
	"github.com/saifsilver/goplusplus/dbcore"
	"github.com/saifsilver/goplusplus/middleware"
	"github.com/saifsilver/goplusplus/pubsub"
	"github.com/saifsilver/goplusplus/queue"
	"github.com/saifsilver/goplusplus/search"
)

func main() {
	ctx := context.Background()

	// 1. Initialize dbcore Database Layer (PgBouncer + Replica Routing + Slow Query Advisor)
	db, err := dbcore.NewClient(ctx, dbcore.Config{
		RWDSN: "postgres://app_rw:secret@pgbouncer:6432/app_rw?sslmode=require",
		RODSN: "postgres://app_ro:secret@pgbouncer:6432/app_ro?sslmode=require",
		PgBouncerTransactionMode: true,
		SlowQuery: dbcore.SlowQueryConfig{
			Threshold:    100 * time.Millisecond,
			MaxSQLLength: 1500,
		},
	})
	if err != nil {
		panic(err)
	}

	// 2. Initialize Cache, Queue, PubSub, and Search engines
	cacheClient := cache.NewClient()
	jobQueue := queue.New()
	eventBus := pubsub.New()
	searchEngine := search.New()

	// Subscribe event listener
	eventBus.Subscribe("lead.created", func(ctx context.Context, payload any) {
		fmt.Printf("📢 Event Received: lead.created -> %v\n", payload)
	})

	// 3. Initialize goplusplus App Engine with Observability
	app := gpp.New()

	app.Use(
		middleware.Observability(), // Prometheus & Latency tracking
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
		middleware.CORS(),
	)

	// 4. REST Endpoints
	v1 := app.Group("/api/v1")

	v1.GET("/leads", func(c *gpp.Context) error {
		reqCtx := dbcore.WithQueryName(c.Request.Context(), "lead.search")

		// Query database (triggers slow-query logging & suggestions if >100ms)
		err := db.Query(reqCtx, "SELECT * FROM leads ORDER BY updated_at DESC", nil)
		if err != nil {
			return gpp.ErrInternal("Database query failed: " + err.Error())
		}

		// Dispatch background queue job & publish event
		_ = jobQueue.Enqueue(reqCtx, "email.send_welcome", gpp.H{"lead_id": "lead_99"})
		_ = eventBus.Publish(reqCtx, "lead.created", gpp.H{"lead_id": "lead_99"})

		// Search docs
		results, _ := searchEngine.Search(reqCtx, "leads", "sales", 10)

		return c.JSON(http.StatusOK, gpp.H{
			"status":  "success",
			"cache":   cacheClient != nil,
			"search":  results,
			"message": "Lead search query processed with dbcore slow query advisor!",
		})
	})

	v1.GET("/leads/:id", func(c *gpp.Context) error {
		id := c.Param("id")
		if id == "0" {
			return gpp.ErrNotFound("Lead with ID '0' does not exist")
		}
		return c.JSON(http.StatusOK, gpp.H{
			"id":     id,
			"status": "active",
		})
	})

	// 5. Metrics & Triple-Auto Endpoints: Prometheus Metrics, Auto-Swagger UI, Auto-GraphQL Playground
	app.GET("/metrics", middleware.Prometheus())
	app.GET("/swagger", app.AutoSwaggerUI())
	app.GET("/graphql", app.AutoGraphQLPlayground("/graphql"))
	app.POST("/graphql", app.AutoGraphQLHandler())

	fmt.Println("🚀 Starting goplusplus Enterprise Stack Server on http://localhost:8080")
	fmt.Println("   • REST Endpoint:       http://localhost:8080/api/v1/leads")
	fmt.Println("   • Prometheus Metrics:  http://localhost:8080/metrics")
	fmt.Println("   • Auto-Swagger UI:     http://localhost:8080/swagger")
	fmt.Println("   • Auto-GraphQL IDE:    http://localhost:8080/graphql")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
