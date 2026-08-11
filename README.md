# `goplusplus` (`gpp`)

[![Go Reference](https://pkg.go.dev/badge/github.com/saifsilver/goplusplus.svg)](https://pkg.go.dev/github.com/saifsilver/goplusplus)
[![Go Version](https://img.shields.io/badge/go-1.21%2B-00ADD8.svg)](https://go.dev/)
[![Version](https://img.shields.io/github/v/tag/saifsilver/goplusplus?color=blue&label=version)](https://github.com/saifsilver/goplusplus/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

`goplusplus` (`gpp`) is an **ultra-fast, secure, and hyper-scalable Go framework** engineered for building modern REST APIs, modular monoliths, and distributed microservices. Designed around a **90% Business Logic, 10% Infrastructure Code** philosophy, `goplusplus` reduces standard Go handler boilerplate by over 90% while keeping the learning curve under **4 hours**.

---

## 🎯 Developer Philosophy: 90% Business Logic, 10% Code

With `goplusplus`, developers focus **exclusively on domain business logic and infrastructure configuration**. The framework efficiently handles routing, data binding, validation, security, observability, multi-protocol generation, database pooling, and fault tolerance out-of-the-box:

| Category | What Developer Writes | What `goplusplus` Handles Automatically |
| :--- | :--- | :--- |
| **Routing** | `app.GET("/users/:id", handler)` | Zero-allocation Radix tree matching, path parameter parsing (`c.Param`), wildcard matching, and context recycling (`sync.Pool`). |
| **Data Binding** | `c.BindAndValidate(&struct)` | Decodes JSON body, applies built-in validation tags, and returns RFC 7807 problem details automatically on error. |
| **Security** | `app.Use(middleware.Security())` | OWASP Security Headers (HSTS, CSP, XSS), CORS preflight, Token Bucket Rate Limiting, Panic Recovery, and Request Execution Timeouts. |
| **Protocol Docs** | `app.AutoSwaggerUI()` | Dynamic OpenAPI 3.0 spec generation, Swagger UI dashboard (`/swagger`), GraphQL Playground (`/graphql`), and gRPC HTTP/2 multiplexing. |
| **Database** | `db.Query(ctx, ...)` | PgBouncer transaction mode, primary/replica routing (`RW`/`RO`), Read-Your-Own-Writes consistency, and **Slow Query Advisor** (SQL fingerprinting & suggestions). |
| **Observability** | `middleware.Observability()` | Prometheus `/metrics` counters, OpenTelemetry `X-Trace-ID` distributed tracing, and `slog` structured logs. |
| **Resilience** | `cb.Execute(...)` | Hystrix Circuit Breakers, Adaptive Concurrency Limiters (Little's Law), and Saga Distributed Transaction Coordinators with auto-compensation. |
| **Frontend SPA** | `app.StaticEmbed("/", webFS)` | Serves embedded React/Vite/Next.js assets with automatic client-side SPA `index.html` fallback routing in 1 line. |

---

## 📦 Installation

```bash
go get github.com/saifsilver/goplusplus@v1.11.0
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
Decode JSON request payloads and validate struct constraints in **1 atomic step**. The validator is implemented inside `goplusplus` using only the Go standard library—there is no external validation dependency. It supports type-aware bounds, comparisons, formats, cross-field and conditional rules, nested structs, and collection/map traversal:

```go
type CreateUserRequest struct {
    Name     string   `json:"name" validate:"required,min=2,max=100"`
    Email    string   `json:"email" validate:"required,email"`
    Age      int      `json:"age" validate:"gte=18,lte=120"`
    Role     string   `json:"role" validate:"oneof=admin editor viewer"`
    Tags     []string `json:"tags" validate:"max=10,dive,required"`
    Website  string   `json:"website" validate:"omitempty,url"`
}

app.POST("/users", func(c *gpp.Context) error {
    var req CreateUserRequest
    if err := c.BindAndValidate(&req); err != nil {
        return err // Automatically returns RFC 7807 Problem Details JSON on error!
    }
    return c.JSON(201, gpp.H{"status": "created", "user": req})
})
```

Invalid input returns stable, machine-readable RFC 7807 JSON using the request's JSON field names:

```json
{
  "type": "https://goplusplus.dev/errors/validation",
  "title": "Request validation failed",
  "status": 400,
  "detail": "One or more fields are invalid",
  "instance": "/users",
  "errors": [
    {"field": "name", "rule": "min", "message": "must contain at least 2 characters"}
  ]
}
```

Strings count Unicode code points. Slices, arrays, and maps use collection length; numeric fields use numeric value. `required` rejects zero values and empty collections. `omitempty` skips only the rules that follow it when the field is empty. Validation metadata is cached per struct type, traversal is bounded, and cycles are detected.

JSON binding requires `application/json`, limits bodies to 1 MiB, rejects unknown fields and multiple documents, and never exposes decoder internals. Compatibility can be enabled explicitly:

```go
app.JSONBinding.AllowUnknownFields = true
app.JSONBinding.AllowNonJSONContentType = true
app.JSONBinding.MaxBodyBytes = 2 << 20
```

Built-in validation rules:

- Presence: `required`, `omitempty`, `omitnil`, `required_if`, `required_unless`, `required_with`, `required_with_all`, `required_without`, `required_without_all`, plus the matching `excluded_*` rules.
- Bounds and comparisons: `min`, `max`, `len`, `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, and their `*field` variants.
- Collections and choices: `oneof`, `not_oneof`, `unique`, `dive`, and `keys`/`endkeys` for maps.
- Text: `alpha`, `alphanum`, `numeric`, `number`, `lowercase`, `uppercase`, `ascii`, `printascii`, `boolean`, `contains*`, `excludes*`, `startswith`, and `endswith`.
- Formats: `email`, `url`, `http_url`, `ip`, `ipv4`, `ipv6`, `cidr`, `cidrv4`, `cidrv6`, `hostname`, `hostname_port`, `uuid`, `uuid3`, `uuid4`, `uuid5`, `json`, `base64`, `base64url`, `hexadecimal`, `hexcolor`, and `datetime`.

Unknown rules and malformed parameters fail closed as internal configuration errors; they are never silently skipped.

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

### 4. 🔐 Type-Safe Context Storage & Retrieval
Store request-scoped variables and safely retrieve them with automatic type coercion or generic inference:

```go
app.Use(func(c *gpp.Context) error {
    c.Set("user_id", int64(1001))
    c.Set("role", "admin")
    return c.Next()
})

app.GET("/profile", func(c *gpp.Context) error {
    // 1. Direct Typed Getters (Safe conversion across int/int64/float64/string)
    userID := c.GetInt64("user_id") // 1001
    role   := c.GetString("role")   // "admin"

    // 2. Single-Value any Getter (Prevents Go syntax multi-value assertion errors)
    val := c.GetAny("user_id").(int64)

    // 3. Generic Getter Functions (Go 1.18+)
    userID, ok := gpp.GetAs[int64](c, "user_id")
    tenantID   := gpp.GetOrDefault[string](c, "tenant_id", "default_tenant")

    return c.JSON(200, gpp.H{"user_id": userID, "role": role})
})
```

---

### 5. ⚛️ Embedded React / Vite / Next.js SPA Serving
Serve embedded React/Vite/Next.js frontend assets directly out of a single Go binary with **Automatic Client-Side SPA Fallback Routing**:

```go
// Embed built static assets directly into the binary
//go:embed dist/*
var distFS embed.FS

app.StaticFS("/", distFS) // Serves assets and falls back to index.html for SPA routes!
```

---

### 6. 🌐 Triple-Auto Generators (Swagger, GraphQL, gRPC)
Auto-generate interactive Swagger OpenAPI documentation, GraphQL Playgrounds, and gRPC Web endpoints with zero manual schema setup:

```go
app.GET("/swagger", app.AutoSwaggerUI())
app.GET("/graphql", app.AutoGraphQLPlayground("/graphql"))
app.POST("/graphql", app.AutoGraphQLHandler())
app.POST("/grpc/*", app.AutoGRPCHandler())
```

---

### 7. 🏛️ Modular Monolith & Microservices Architecture
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

## ⚡ Persistent Background Task Tracker & Auto-Retry Engine (`queue`)

Dispatch tracked background tasks with automatic retries on failure and full status visibility:

```go
// 1. Dispatch tracked background task (returns task ID)
app.POST("/api/v1/tasks/send-email", func(c *gpp.Context) error {
    taskID := c.AsyncTask("send_welcome_email", func(c *gpp.Context) error {
        return sendEmailSMTP() // Automatically retries up to 3 times on failure!
    })
    return c.JSON(http.StatusAccepted, gpp.H{"task_id": taskID})
})

// 2. Query task status anytime by ID
app.GET("/api/v1/tasks/:id", func(c *gpp.Context) error {
    taskInfo, _ := c.GetTaskStatus(c.Param("id"))
    // Returns: task.Status ("PENDING"|"RUNNING"|"COMPLETED"|"FAILED"|"RETRYING"), Retries, LastError
    return c.JSON(http.StatusOK, gpp.H{"task": taskInfo})
})
```

---

## 📡 Real-Time WebSockets & Server-Sent Events (SSE) Suite (`realtime`)

`goplusplus` includes zero-dependency support for real-time WebSocket messaging and Server-Sent Events (SSE):

```go
// 1. Server-Sent Events (SSE) Live Stream
app.GET("/api/v1/sse", func(c *gpp.Context) error {
    eventChan := make(chan any)
    go func() {
        eventChan <- realtime.SSEEvent{Event: "notice", Data: "Live Update!"}
    }()
    return realtime.StreamSSE(c, eventChan)
})

// 2. WebSocket Real-Time Upgrade
app.GET("/api/v1/ws", func(c *gpp.Context) error {
    conn, err := realtime.Upgrade(c)
    if err != nil { return err }
    defer conn.Close()

    msg, _ := conn.ReadMessage()
    return conn.WriteMessage("Echo: " + msg)
})
```

---

## 🔮 Probabilistic Data Structures & Cache Defense (`bloom`)

High-performance Bloom Filters, HyperLogLog DAU counters, and Count-Min Sketch frequency estimators:

```go
// 1. Bloom Filter (Zero-False Negative Cache Penetration Defense)
bloomFilter := bloom.NewFilter(10000, 0.01)
bloomFilter.Add("usr_101")

if !bloomFilter.MayContain(id) {
    return gpp.ErrNotFound("User 100% does not exist in DB") // Blocks malicious DB attack!
}

// 2. HyperLogLog (Constant Memory Unique Visitor Counter)
hll := bloom.NewHyperLogLog()
hll.Add("user_ip_1")
daus := hll.EstimateCardinality()

// 3. Count-Min Sketch (Top-K Trending Frequency Estimator)
cms := bloom.NewCountMinSketch()
cms.Add("trending_topic", 1)
```

---

## 🔒 Universal Security & Authentication Engine

`goplusplus` includes multi-platform security for Web and Mobile applications:

```go
// 1. Password storage (Argon2id PHC hash with random per-password salt)
passwordHash, err := auth.HashPasswordWithConfig(
    password, passwordPepper, auth.DefaultPasswordConfig(),
)
if err != nil { return err }

// 2. JWT policy with explicit issuer, audience, key ID, rotation set, and TTL
tokens, err := auth.NewTokenManager(auth.TokenConfig{
    Issuer: "https://api.example.com",
    Audience: "example-api",
    ActiveKeyID: "2026-08",
    Keys: map[string][]byte{"2026-08": signingKey}, // at least 32 random bytes
    MaxTTL: 24 * time.Hour,
})
if err != nil { return err }
jwtToken, err := tokens.IssueUser(userClaims, 15*time.Minute)

// 3. Server-side HTTP-only SameSite cookie sessions
sessionMgr, err := auth.NewSessionManager(auth.SessionConfig{
    TTL: 8 * time.Hour, SameSite: http.SameSiteLaxMode,
})
if sessionMgr.CreateSession(c, userClaims) == "" { return errors.New("session creation failed") }

// 4. Accept a verified cookie session or signed JWT
app.Use(auth.UniversalAuthWithManager(tokens, sessionMgr))
```

The former placeholder PASETO API is disabled and fails closed. `GenerateToken` and `GenerateJWT` now require exactly one explicit positive TTL. Use `VerifyLegacyPassword` or `VerifyPasswordWithMigration` only while migrating pre-v1.11 HMAC password records.

---

## ⚙️ Configuration & Secret Vault Suite (`config`)

Zero-dependency `.env` file loader, type-safe getters, secret masker, and struct tag unmarshaler:

```go
// 1. Load .env file automatically
_ = config.Load(".env")

// 2. Unmarshal into struct via `env:"KEY"` tags
type AppConfig struct {
    Port   string `env:"PORT" default:":8080"`
    DBURL  string `env:"DATABASE_URL"`
    Secret string `env:"JWT_SECRET"`
}

var cfg AppConfig
_ = config.Unmarshal(&cfg)

// 3. Type-Safe Getters & Secret Masker
port   := config.GetString("PORT", ":8080")
conns  := config.GetInt("MAX_CONNS", 100)
secret := config.MaskSecret(cfg.Secret) // Redacts secret: "sup...91823"
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

// 3. Distributed Redis & L1 Memory Cache (Single-Flight Stampede Protection)
redisCache := cache.NewRedisClient("redis://localhost:6379/0")
val, _     := redisCache.GetOrSet(ctx, "user:101", 10*time.Minute, func() (any, error) {
    return fetchUserFromDB(101)
})

// 4. Elasticsearch & OpenSearch
esClient := search.NewElasticsearchClient(search.ESConfig{})
_ = esClient.IndexDocument(ctx, "products", "prod_1", payload)

// 5. Kafka & RabbitMQ Messaging
kafkaWorker := queue.NewKafkaWorker([]string{"localhost:9092"}, "events")
rabbitBus   := pubsub.NewRabbitMQBus("amqp://localhost:5672")
```

## 🔎 Dynamic Attributes, Filters & Faceted Search

Search resources declare every queryable dynamic attribute once. The same typed request contract powers Go, REST, and GraphQL, so clients cannot inject database columns, SQL fragments, or backend-specific query DSL.

```go
schema, err := search.NewSchema(
	search.AttributeDefinition{
		Key: "title", Type: search.AttributeString, Searchable: true,
	},
	search.AttributeDefinition{
		Key: "brand", Type: search.AttributeEnum,
		Filterable: true, Facetable: true,
		EnumValues: []string{"Nike", "Adidas"},
	},
	search.AttributeDefinition{
		Key: "price", Type: search.AttributeDecimal,
		Filterable: true, Sortable: true,
	},
	search.AttributeDefinition{
		Key: "tenant_id", Type: search.AttributeString, Filterable: true,
	},
)
if err != nil {
	return err
}

backend, err := search.NewDatabaseBackend(db, search.DatabaseConfig{})
if err != nil {
	return err
}
if err := backend.Setup(ctx); err != nil {
	return err
}

products, err := search.NewResource("products", schema, backend)
if err != nil {
	return err
}
```

For tenant- or authorization-scoped resources, add mandatory filters with `search.WithScope`. Scope filters always constrain hits and facet counts, including disjunctive facets:

```go
products, err := search.NewResource("products", schema, backend,
	search.WithScope(func(ctx context.Context) ([]search.Filter, error) {
		return []search.Filter{
			{Field: "tenant_id", Operator: search.OperatorEqual, Value: tenantIDFrom(ctx)},
		}, nil
	}),
)
```

Index and search dynamic documents:

```go
_ = products.Index(ctx, search.Document{
	ID: "prod_1",
	Attributes: map[string]any{
		"title": "Road running shoe",
		"brand": "Nike",
		"price": 120.00,
	},
})

result, err := products.Search(ctx, search.SearchRequest{
	Query: "running",
	Filters: []search.Filter{
		{Field: "price", Operator: search.OperatorBetween, Value: []float64{50, 150}},
	},
	Facets: []search.FacetRequest{
		{Field: "brand", Mode: search.FacetDisjunctive},
	},
})
```

Bind a REST endpoint and the schema-validated GraphQL field:

```go
registry, _ := search.NewRegistry(products)

gpp.BindSearchResource(v1, "/products/search", products)
_ = gpp.BindSearchGraphQL(app, "productSearch", registry)

app.POST("/graphql", app.AutoGraphQLHandler())
```

```graphql
query ProductSearch($request: SearchRequestInput!) {
  productSearch(resource: "products", request: $request) {
    total
    nextCursor
    items { id score attributes }
    facets { field buckets { value count } }
  }
}
```

Choose the backend explicitly per resource:

- **Database (default):** use for exact transactional results, moderate query volume, simple full-text search, and minimal operational overhead. The built-in backend targets PostgreSQL JSONB with GIN indexes.
- **Elasticsearch/OpenSearch:** use when relevance ranking, typo tolerance, stemming, synonyms, autocomplete, high-volume aggregations, or many multi-value facets justify a separate eventually-consistent index.
- **Hybrid:** use Elasticsearch for ranking/facets and hydrate authorized records from the database. Tenant and authorization filters must be applied before both hits and facet counts are calculated.

Facets are disjunctive by default: a filter on `brand` is excluded while computing the `brand` buckets, preserving alternative brand counts. Set `Mode: search.FacetConjunctive` when selected filters should narrow their own facet.

---

## ✅ Pre-Push Quality Gate

Install the repository-managed hook once per clone:

```bash
make install-hooks
```

Every `git push` then runs the same gate as CI:

```bash
make verify
```

The gate checks:

- `gofmt` formatting without modifying files
- `go vet ./...`
- `go test ./...` with atomic coverage collection
- a repository-wide coverage floor (initially 55%; override with `COVERAGE_MIN=60 make coverage`)
- `go mod verify`
- the pinned official Go `govulncheck` vulnerability scanner

The tracked hook is a developer feedback mechanism; GitHub Actions remains authoritative because local hooks can be bypassed with `--no-verify`.

### Release tagging

Create the next patch release from a clean worktree. The command updates
`gpp.Version`, creates a release commit, and adds an annotated Git tag:

```bash
make tag
```

Choose a specific semantic version when publishing a minor or major release:

```bash
make tag VERSION=v1.12.0
```

The tag remains local until explicitly published with `git push origin <tag>`.

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
- **Internationalization (`i18n`)**: BCP 47/`Accept-Language` negotiation, runtime catalogs, CLDR plural rules, interpolation, locale-aware numbers and percentages, exact ISO 4217 money, IANA time zones, localized date/time styles, and RTL/LTR metadata.
- **Multi-Tenancy (`tenant`)**: Tenant extraction from headers/subdomains.
- **Security (`auth`)**: JWT authentication, RBAC, ABAC, and TOTP 2FA MFA.
- **Notifications (`notify`)**: Email SMTP & SMS dispatch.

```go
bundle := i18n.NewBundle("en")
_ = bundle.AddMessages("de", map[string]string{
    "welcome %s": "Willkommen, %s",
})
_ = bundle.AddPlural("de", "results", i18n.PluralForms{
    One:   "ein Ergebnis",
    Other: "%d Ergebnisse",
})
app.Use(bundle.Middleware()) // ?lang=de or weighted Accept-Language

// Store money as minor units, never float64: USD 1,234.56 = 123456 cents.
price, err := i18n.FormatMoney(i18n.Money{
    MinorUnits: 123456,
    Currency:   "USD",
}, "fr-FR")

createdAt, err := bundle.FormatDateTime(
    time.Now(),
    "en-GB",
    "Europe/London",
    i18n.DateTimeOptions{DateStyle: i18n.Long, TimeStyle: i18n.Short},
)
```

The middleware stores the canonical language under `i18n.ContextLanguageKey`, the full `i18n.Locale` under `i18n.ContextLocaleKey`, and emits `Content-Language` plus `Vary: Accept-Language`. English, Spanish, and French starter catalogs/date profiles are included; applications register their own messages and may add regional date-time profiles with `RegisterDateTimeProfile`.

---

### 14. 🛠️ Production Suite: Migrations, Seeder/Faker, Idempotency, Singleflight & Pagination

- **SQL Migrations (`dbcore.AutoMigrate`, `dbcore.MigrateEmbed`)**: Auto-executes transactional migrations tracked in `gpp_migrations`.
- **DB Seeder & Faker (`seed.Run`, `seed.Faker`)**: Seed fake data (`f.Name()`, `f.Email()`, `f.UUID()`, `f.Phone()`) in 1 batch operation.
- **Idempotency Key (`middleware.Idempotency()`)**: Caches write responses for `Idempotency-Key` headers to prevent duplicate side-effects.
- **Singleflight Deduplication (`middleware.Singleflight()`)**: Deduplicates concurrent identical `GET` requests to prevent thundering herd.
- **High-Performance Pagination (`c.Paginate`, `c.PaginateCursor`)**: Standardized page-based and $O(1)$ cursor-based pagination.

```go
// 1. Auto-Run SQL Migrations from embed.FS
//go:embed migrations/*.sql
var migrationFiles embed.FS
_ = dbcore.MigrateEmbed(ctx, db, migrationFiles, "migrations")

// 2. Batch Seed Fake Users
_ = seed.Run(ctx, db, seed.Plan{
    Table: "users",
    Count: 50,
    Factory: func(f *seed.Faker) map[string]any {
        return map[string]any{"name": f.Name(), "email": f.Email(), "role": f.Select("admin", "user")}
    },
})

// 3. High-Performance Cursor Pagination Handler
app.GET("/api/v1/feed", func(c *gpp.Context) error {
    cursor, limit := c.GetCursorAndLimit(20)
    result, err := userRepo.PaginateCursor(c.Request.Context(), "id", cursor, limit)
    if err != nil {
        return err
    }
    return c.PaginateCursor(200, result.Items, result.NextCursor, result.HasMore, result.Limit)
})
```

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
