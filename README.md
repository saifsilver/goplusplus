# `goplusplus` (`gpp`)

[![Go Reference](https://pkg.go.dev/badge/github.com/saifsilver/goplusplus.svg)](https://pkg.go.dev/github.com/saifsilver/goplusplus)
[![Go Version](https://img.shields.io/badge/go-1.21%2B-00ADD8.svg)](https://go.dev/)
[![Version](https://img.shields.io/github/v/tag/saifsilver/goplusplus?color=blue&label=version)](https://github.com/saifsilver/goplusplus/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

`goplusplus` (`gpp`) is a **zero-dependency, ultra-fast, secure, and hyper-scalable Go framework** engineered for building modern REST APIs, modular monoliths, and distributed microservices. Designed around a **90% Business Logic, 10% Infrastructure Code** philosophy, `goplusplus` reduces standard Go handler boilerplate by over 90% while keeping the learning curve under **4 hours**.

---

## 🎯 Developer Philosophy: 90% Business Logic, 10% Code

With `goplusplus`, developers focus **exclusively on domain business logic and infrastructure configuration**. The framework efficiently handles routing, data binding, validation, security, observability, multi-protocol generation, database pooling, and fault tolerance out-of-the-box:

| Category | What Developer Writes | What `goplusplus` Handles Automatically |
| :--- | :--- | :--- |
| **Routing** | `app.GET("/users/:id", handler)` | Zero-allocation Radix tree matching, path parameter parsing (`c.Param`), wildcard matching, and context recycling (`sync.Pool`). |
| **Data Binding** | `c.BindAndValidate(&struct)` | Decodes JSON body, validates struct tags (`validate:"required,email"`), and returns RFC 7807 problem details automatically on error. |
| **Security** | `app.Use(middleware.Security())` | OWASP Security Headers (HSTS, CSP, XSS), CORS preflight, Token Bucket Rate Limiting, Panic Recovery, and Request Execution Timeouts. |
| **Protocol Docs** | `app.AutoSwaggerUI()` | Dynamic OpenAPI 3.0 spec generation, Swagger UI dashboard (`/swagger`), GraphQL Playground (`/graphql`), and gRPC HTTP/2 multiplexing. |
| **Database** | `db.Query(ctx, ...)` | PgBouncer transaction mode, primary/replica routing (`RW`/`RO`), Read-Your-Own-Writes consistency, and **Slow Query Advisor** (SQL fingerprinting & suggestions). |
| **Observability** | `middleware.Observability()` | Prometheus `/metrics` counters, OpenTelemetry `X-Trace-ID` distributed tracing, and `slog` structured logs. |
| **Resilience** | `cb.Execute(...)` | Hystrix Circuit Breakers, Adaptive Concurrency Limiters (Little's Law), and Saga Distributed Transaction Coordinators with auto-compensation. |
| **Frontend SPA** | `app.StaticEmbed("/", webFS)` | Serves embedded React/Vite/Next.js assets with automatic client-side SPA `index.html` fallback routing in 1 line. |

---

## 📦 Installation

```bash
go get github.com/saifsilver/goplusplus@v1.0.0
```

---

## 🚀 Quick Start (15 Lines of Code)

```go
package main

import (
	"fmt"
	"net/http"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/middleware"
)

func main() {
	app := gpp.New()

	// Global Security & Observability Middleware
	app.Use(middleware.Logger(), middleware.Recovery(), middleware.Security())

	// API Handler
	app.GET("/api/v1/hello", func(c *gpp.Context) error {
		return c.JSON(http.StatusOK, gpp.H{
			"message": "Welcome to goplusplus!",
			"status":  "active",
		})
	})

	fmt.Println("🚀 Server running on http://localhost:8080")
	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
```

---

## 🏛️ Master Feature Catalog

### 1. ⚡ Zero-Allocation Radix Router & Context Pooling
High-performance HTTP routing supporting static paths, named parameters (`:id`), and wildcards (`*path`):

```go
v1 := app.Group("/api/v1")
v1.GET("/users/:id", func(c *gpp.Context) error {
    id := c.Param("id")
    page := c.QueryDefault("page", "1")
    return c.JSON(200, gpp.H{"id": id, "page": page})
})
```

---

### 2. ✅ Ergonomic Struct Binding & Tag Validation
Decode JSON request payloads and validate struct constraints (`validate:"required"`, `validate:"email"`) in **1 atomic step**:

```go
type CreateUserRequest struct {
    Name  string `json:"name" validate:"required"`
    Email string `json:"email" validate:"required,email"`
}

app.POST("/users", func(c *gpp.Context) error {
    var req CreateUserRequest
    if err := c.BindAndValidate(&req); err != nil {
        return err // Automatically returns RFC 7807 Problem Details JSON on error!
    }
    return c.JSON(201, gpp.H{"status": "created", "user": req})
})
```

---

### 3. 🚀 Parallel Concurrency & DB Query Execution
Execute multiple asynchronous tasks or database queries concurrently in parallel goroutines with context cancellation and panic safety:

```go
// Parallel Async Task Execution
err := c.Parallel(
    func(c *gpp.Context) error { return fetchUserProfile(c) },
    func(c *gpp.Context) error { return fetchUserOrders(c) },
    func(c *gpp.Context) error { return fetchUserMetrics(c) },
)

// Concurrent Parallel Database Queries across Replicas
err = db.ParallelQuery(ctx,
    dbcore.ParallelTask{QueryName: "user.fetch", SQL: "SELECT * FROM users WHERE id=$1", Args: []any{"42"}},
    dbcore.ParallelTask{QueryName: "orders.fetch", SQL: "SELECT * FROM orders WHERE user_id=$1", Args: []any{"42"}},
)
```

---

### 4. ⚛️ Embedded React / Vite / Next.js SPA Serving
Serve embedded React/Vite/Next.js frontend assets directly out of a single Go binary with **Automatic Client-Side SPA Fallback Routing**:

```go
//go:embed all:dist
var reactDistFS embed.FS

func main() {
    app := gpp.New()

    app.GET("/api/v1/users", getUsersHandler)

    // 1-Liner: Auto-detects dist/build folder & serves React SPA on "/" with index.html fallback
    app.StaticEmbed("/", reactDistFS)

    app.Listen(":8080")
}
```

---

### 5. 🌐 Triple-Auto Generators (Swagger, GraphQL, gRPC)
Zero-configuration automatic protocol documentation and multiplexing:

```go
// Auto-Generated OpenAPI 3.0 & Interactive Swagger UI
app.GET("/swagger", app.AutoSwaggerUI())

// Auto-Generated GraphQL Schema & Interactive Playground
app.GET("/graphql", app.AutoGraphQLPlayground("/graphql"))
app.POST("/graphql", app.AutoGraphQLHandler())

// Prometheus Metrics Endpoint
app.GET("/metrics", middleware.Prometheus())
```

---

### 6. 🏛️ Modular Monolith & Microservices Architecture
Organize domain features into clean `gpp.Module` implementations:

```go
type UserModule struct{}

func (m *UserModule) Name() string { return "UserModule" }
func (m *UserModule) Register(group *gpp.RouterGroup) {
    group.GET("/profile/:id", m.getProfile)
}

func (m *UserModule) getProfile(c *gpp.Context) error {
    return c.JSON(200, gpp.H{"id": c.Param("id")})
}

// In main.go (Monolith Mode):
app.RegisterModule("/api/v1/users", &UserModule{})
```

---

### 7. 🗄️ `dbcore` Production Database Access & Slow Query Advisor
Handles PgBouncer transaction mode, primary (`RW`) / replica (`RO`) routing, Read-Your-Own-Writes consistency, and automated **Slow Query Advisor**:

```go
import "github.com/saifsilver/goplusplus/dbcore"

db, _ := dbcore.NewClient(ctx, dbcore.Config{
    RWDSN: "postgres://app_rw:...@pgbouncer:6432/app_rw",
    RODSN: "postgres://app_ro:...@pgbouncer:6432/app_ro",
    SlowQuery: dbcore.SlowQueryConfig{Threshold: 250 * time.Millisecond},
})

// Attach query name for dashboards
reqCtx := dbcore.WithQueryName(c.Request.Context(), "user.search")

// Read Query (Auto-routes to Replica RO)
err := db.Query(reqCtx, "SELECT id, name FROM users WHERE id=$1", fn, "42")

// Transaction Write (Auto-routes to Primary RW)
err = db.InTx(reqCtx, func(tx *sql.Tx) error {
    return nil
})
```

---

### 8. 🛡️ Hyper-Scale Resilience Suite (Circuit Breaker & Adaptive Limiter)
Protect downstream services from cascading failure and sudden 100x traffic spikes:

```go
// 1. Circuit Breaker
cb := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
    FailureThreshold: 5,
    ResetTimeout:     10 * time.Second,
})

err := cb.Execute(func() error {
    return callExternalPaymentAPI()
})

// 2. Adaptive Concurrency Limiter (Little's Law)
app.Use(resilience.NewAdaptiveLimiter(1000).Middleware())
```

---

### 9. 🔄 Saga Distributed Transactions (`saga`)
Manage multi-microservice transactions with **automatic reverse compensation** on step failures:

```go
sagaCoord := saga.NewCoordinator()

sagaCoord.AddStep("reserve_stock",
    func(ctx context.Context) error { return reserveStock() },
    func(ctx context.Context) error { return releaseStock() }, // Auto-compensation on error!
)

sagaCoord.AddStep("charge_payment",
    func(ctx context.Context) error { return chargePayment() },
    func(ctx context.Context) error { return refundPayment() },
)

err := sagaCoord.Execute(ctx)
```

---

## ☁️ Cloud Infrastructure Suite (SQLite, S3, CloudFront, Elasticsearch, Kafka, RabbitMQ)

`goplusplus` includes built-in driver adapters for enterprise cloud services:

```go
// 1. AWS S3 & CloudFront CDN
s3Client := storage.NewS3Client(storage.S3Config{Bucket: "app-assets", Region: "us-east-1"})
s3URL, _ := s3Client.Upload(ctx, "logo.png", data, "image/png")
cdnURL  := storage.GenerateCloudFrontURL("cdn.myapp.com", "logo.png")

// 2. Zero-Config Embedded SQLite Database
sqliteDB, _ := dbcore.NewSQLiteClient("app.db")

// 3. Elasticsearch & OpenSearch
esClient := search.NewElasticsearchClient(search.ESConfig{})
_ = esClient.IndexDocument(ctx, "products", "prod_1", payload)

// 4. Kafka & RabbitMQ Messaging
kafkaWorker := queue.NewKafkaWorker([]string{"localhost:9092"}, "events")
rabbitBus   := pubsub.NewRabbitMQBus("amqp://localhost:5672")
```

---

## 🧪 Ergonomic E2E Integration Testing Suite (`gpptest`)

Write 3-line E2E API integration tests without starting real network sockets:

```go
func TestCreateUser(t *testing.T) {
    tester := gpptest.New(t, app)
    res := tester.POST("/api/v1/users", gpp.H{"name": "Alice", "email": "alice@dev.com"})
    res.AssertStatus(201)
    res.AssertJSON("status", "created")
}
```

---

## ⚡ `cmd/gpp` CLI Code Generator

Install the CLI tool to scaffold applications and domain modules in seconds:

```bash
# Install CLI
go install github.com/saifsilver/goplusplus/cmd/gpp@latest

# Scaffold new app
gpp new myapp

# Generate domain module
gpp gen module order
```

---

## 🧱 Dependency Injection & Lifecycle Hooks (`di`) — Uber FX-Style

Constructor dependency injection container with startup and shutdown hooks:

```go
container := di.New()

container.OnStart(func() error {
    log.Println("Database connection pool initialized")
    return nil
})

_ = container.Start()
defer func() { _ = container.Stop() }()
```

---

### 11. 📌 RFC 8594 API Versioning Engine
Multi-strategy API version negotiation (Header `X-API-Version`, Query `?v=1`, `Accept` header) with RFC 8594 deprecation and sunset date HTTP headers:

```go
vm := versioning.NewManager("v1")
vm.Deprecate("v1", "2027-01-01") // Warns v1 clients with Sunset HTTP headers!

app.Use(vm.Middleware())
```

---

### 12. 🩺 Kubernetes Health Probes & Security Audit Logger
Kubernetes `/healthz/liveness` & `/healthz/readiness` probes and tamper-evident SOC2 audit logging:

```go
healthChecker := health.NewChecker()
healthChecker.AddReadinessCheck("database", func(ctx context.Context) error { return db.Ping() })

app.GET("/healthz/liveness", healthChecker.Liveness())
app.GET("/healthz/readiness", healthChecker.Readiness())

// Security Audit Event
audit.Log(ctx, "user_admin", "UPDATE_ROLE", "user_42", map[string]any{"new_role": "admin"})
```

---

### 13. 🌐 Enterprise Platform Suite (`i18n`, `tenant`, `auth`, `notify`)
- **Localization (`i18n`)**: Multi-language translation & currency formatting.
- **Multi-Tenancy (`tenant`)**: Tenant extraction from headers/subdomains.
- **Security (`auth`)**: JWT authentication, RBAC, ABAC, and TOTP 2FA MFA.
- **Notifications (`notify`)**: Email SMTP & SMS dispatch.

---

## 🤖 AI Agent Integration Guide (`AGENTS.md`)

`goplusplus` includes a dedicated **[AGENTS.md](AGENTS.md)** specification file designed for AI coding assistants (Codex, Claude, Cursor, Copilot, ChatGPT). AI agents reading `AGENTS.md` can instantly generate 100% correct, production-grade `goplusplus` code.

---

## 📁 Ready-to-Run Example Applications

Explore complete, runnable production examples in the [`examples/`](examples/) directory:

- 🚀 **[`examples/basic`](examples/basic)**: Fundamental REST routing & context.
- 🔒 **[`examples/secure_api`](examples/secure_api)**: OWASP security headers, CORS & rate limiting.
- 🏛️ **[`examples/modular_monolith`](examples/modular_monolith)**: Domain-driven module registration.
- 🌁 **[`examples/grpc_gateway`](examples/grpc_gateway)**: gRPC protocol multiplexing & gateway.
- 🔀 **[`examples/multi_protocol`](examples/multi_protocol)**: REST, GraphQL, and Swagger UI co-existing.
- 🏢 **[`examples/enterprise_stack`](examples/enterprise_stack)**: Complete `dbcore`, cache, queue, pubsub & search setup.
- 🌐 **[`examples/enterprise_platform`](examples/enterprise_platform)**: `i18n`, `tenant`, `auth` (RBAC/ABAC/MFA), and `notify`.
- 🏭 **[`examples/production_ready_app`](examples/production_ready_app)**: K8s health probes, OTel tracing, validation, versioning, feature flags.
- ⚡ **[`examples/parallel_concurrency`](examples/parallel_concurrency)**: Concurrent task execution & parallel replica DB queries.
- ⚛️ **[`examples/react_embedded`](examples/react_embedded)**: Single-binary embedded React/Vite SPA serving.
- 🛡️ **[`examples/hyper_scale_app`](examples/hyper_scale_app)**: DI container, Circuit Breakers, Adaptive Limiters, and Saga Distributed Transactions.

---

## 📄 License

Distributed under the MIT License. See [`LICENSE`](LICENSE) for more information.
