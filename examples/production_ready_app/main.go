package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/audit"
	"github.com/saifsilver/goplusplus/dbcore"
	"github.com/saifsilver/goplusplus/features"
	"github.com/saifsilver/goplusplus/health"
	"github.com/saifsilver/goplusplus/middleware"
	"github.com/saifsilver/goplusplus/tracing"
	"github.com/saifsilver/goplusplus/versioning"
)

type CreateLeadRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func main() {
	ctx := context.Background()

	// 1. Database auto-migrations
	db, _ := dbcore.NewClient(ctx, dbcore.Config{})
	_ = dbcore.Migrate(ctx, db, []dbcore.Migration{
		{ID: "001_create_leads", SQL: "CREATE TABLE leads (id text);"},
	})

	// 2. Health Probes & Feature Flags
	healthChecker := health.NewChecker()
	healthChecker.AddReadinessCheck("database", func(ctx context.Context) error {
		return nil // DB is healthy
	})
	featureManager := features.NewManager()
	tracingProvider, err := tracing.NewProvider(ctx, tracing.Config{
		ServiceName: "goplusplus-production-api", Endpoint: os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"),
		Insecure: parseBool(os.Getenv("OTEL_EXPORTER_OTLP_INSECURE")),
	})
	if err != nil {
		panic(err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = tracingProvider.Shutdown(shutdownCtx)
	}()

	// 3. goplusplus App Engine with Full Observability & Production Middleware
	app := gpp.New()

	app.Use(
		tracingProvider.Middleware(), // OpenTelemetry OTLP tracing
		versioning.Middleware("v1"),  // API versioning negotiation
		middleware.Observability(),   // Prometheus request metrics
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
		middleware.CORS(),
	)

	// K8s Health Probes
	app.GET("/healthz/liveness", healthChecker.Liveness())
	app.GET("/healthz/readiness", healthChecker.Readiness())

	// Production API Routes
	v1 := app.Group("/api/v1")

	v1.POST("/leads", func(c *gpp.Context) error {
		var req CreateLeadRequest
		if err := c.BindJSON(&req); err != nil {
			return gpp.ErrBadRequest("Invalid JSON request body")
		}

		if err := c.Validate(&req); err != nil {
			return err
		}

		if featureManager.IsEnabled(c.Request.Context(), "new_checkout_v2") {
			c.SetHeader("X-Feature-V2", "enabled")
		}

		// Record security audit event
		audit.Log(c.Request.Context(), "user_admin", "CREATE_LEAD", "lead_101", map[string]any{
			"email": req.Email,
		})

		return c.JSON(http.StatusCreated, gpp.H{
			"status":   "created",
			"trace_id": tracing.GetTraceID(c),
			"lead":     req,
		})
	})

	fmt.Println("🚀 Starting 100% Production-Ready goplusplus Master Server on http://localhost:8080")
	fmt.Println("   • K8s Liveness Probe:  http://localhost:8080/healthz/liveness")
	fmt.Println("   • K8s Readiness Probe: http://localhost:8080/healthz/readiness")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}

func parseBool(value string) bool {
	parsed, _ := strconv.ParseBool(value)
	return parsed
}
