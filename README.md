# `go++` (`gpp`) — Ultra-Fast, Secure & Scalable Go Framework

[![Go Version](https://img.shields.io/badge/Go-1.26%2B-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/Build-Passing-brightgreen.svg)]()

`go++` is a zero-dependency, ultra-fast, secure, and scalable HTTP framework for building modern REST APIs and microservices in Go. Designed to eliminate boilerplate, `go++` reduces code by over **90%** compared to standard Go handlers while providing a learning curve of **< 4 hours**.

---

## ⚡ Highlights

- **⚡ Blazing Fast**: Custom Radix Tree router with zero-alloc parameter lookup and `sync.Pool` context pooling.
- **🛡️ Built-in Security**: Pre-configured Security Headers (HSTS, CSP, XSS, Frame Options), Token Bucket Rate Limiting, CORS, and Panic Recovery.
- **🧱 10% Code (Ultra-Low Boilerplate)**: High-level `Context` primitives (`c.JSON()`, `c.BindJSON()`, `c.Param()`, `gpp.H`) turn verbose handlers into one-liners.
- **📈 Scalable**: Built natively on Go's `net/http` server primitives with goroutine concurrency and graceful shutdown support.
- **⏱️ < 4 Hours Learning Curve**: Idiomatic Go API without complex reflection or hidden black magic.

---

## 📦 How to Add `go++` to Any Project

### Method 1: Standard Remote Import (GitHub)

Publish your `go++` repository to GitHub (`github.com/your-username/go++`), then in any project directory run:

```bash
go get github.com/your-username/go++
```

In your application `main.go`:
```go
import (
    "github.com/your-username/go++"
    "github.com/your-username/go++/middleware"
)
```

---

### Method 2: Local Filesystem Link (`replace` directive in `go.mod`)

If `go++` lives locally on your disk (e.g., `/path/to/go++`) and you want to use it in a separate app project without publishing yet:

1. Open your app's `go.mod`:
```go
module my-app

go 1.26

require github.com/saifsulaiman/go++ v0.0.0

replace github.com/saifsulaiman/go++ => /Users/saifsulaiman/Documents/websites/go++
```

2. Run `go mod tidy` in your app folder. Go will link directly to your local `go++` source files!

---

### Method 3: Go Workspaces (`go.work`)

For multi-repo or monorepo development, initialize a Go workspace:

```bash
go work init ./path/to/go++ ./path/to/my-app
```

Now Go automatically links `go++` to `my-app` locally!

---

## 🚀 Quickstart (Under 15 Lines of Code!)

```go
package main

import (
	"net/http"
	"go++"
	"go++/middleware"
)

func main() {
	app := gpp.New()
	app.Use(middleware.Logger(), middleware.Recovery())

	app.GET("/api/v1/users/:id", func(c *gpp.Context) error {
		return c.JSON(http.StatusOK, gpp.H{
			"user_id": c.Param("id"),
			"status":  "active",
		})
	})

	app.Listen(":8080")
}
```

---

## 🏛️ Modular Monolith & Microservices Architecture

`go++` provides native support for **Modular Monoliths**. Each domain feature (e.g. `User`, `Order`, `Payment`) implements the simple `gpp.Module` interface:

```go
type Module interface {
    Name() string
    Register(group *gpp.RouterGroup)
}
```

### 1. Monolith Mode (Single Binary, Port 8080)
Run all modules together inside a single binary during early stages or low-overhead deployments:

```go
app := gpp.New()

// Register all modules into one app
app.RegisterModule("/api/v1/users", userModule.New())
app.RegisterModule("/api/v1/orders", orderModule.New())

app.Listen(":8080")
```

### 2. Microservice Extraction (Zero Refactoring!)
When a module requires independent scaling, extract it into its own microservice binary—**with 0 changes to your business domain code**:

```go
// Standalone Order Microservice (Port 8082)
app := gpp.New()
app.RegisterModule("/orders", orderModule.New())

app.Listen(":8082")
```

---

## 🌐 Multi-Language (`i18n`) & Currency Formatting

`goplusplus` includes built-in localization and currency formatting:

```go
app.Use(i18nBundle.Middleware())

app.GET("/", func(c *gpp.Context) error {
    lang := c.GetString("lang")
    return c.JSON(200, gpp.H{
        "message":   i18nBundle.Translate(lang, "welcome"),
        "price_usd": i18n.FormatCurrency(199.99, "USD"),
        "price_eur": i18n.FormatCurrency(199.99, "EUR"),
    })
})
```

---

## 🏢 Multi-Tenancy Engine (`tenant`)

Automatically extracts tenant isolation IDs from subdomains (`tenant.app.com`), headers (`X-Tenant-ID`), or JWT claims:

```go
app.Use(tenant.Middleware())

app.GET("/data", func(c *gpp.Context) error {
    tenantID := tenant.GetTenantID(c)
    return c.JSON(200, gpp.H{"tenant_id": tenantID})
})
```

---

## 🔐 Identity, RBAC, ABAC & MFA (TOTP) Security Suite (`auth`)

```go
adminGroup := app.Group("/api/admin")
adminGroup.Use(
    auth.Authenticate("secret_key"),
    auth.RequireRoles("admin"),
    auth.RequirePolicy(func(u *auth.UserClaims) bool {
        return u.Attributes["department"] == "finance"
    }),
    auth.RequireMFA("totp_secret"),
)
```

---

## 📬 Email & SMS Notification Suite (`notify`)

```go
_ = notify.SendEmail(ctx, notify.EmailMessage{To: "user@example.com", Subject: "Welcome", Body: "Hello!"})
_ = notify.SendSMS(ctx, notify.SMSMessage{ToPhone: "+15550192831", Text: "Your OTP is 123456"})
```

---

## 📊 Observability (Prometheus Metrics & Tracing)

`goplusplus` includes built-in request metrics and a Prometheus output endpoint:

```go
app.Use(middleware.Observability())
app.GET("/metrics", middleware.Prometheus())
```

Accessing `http://localhost:8080/metrics` returns standard Prometheus counters (`http_requests_total`, `http_active_requests`) for scraping.

---

## 🚨 Zero-Boilerplate RFC 7807 Exception Handling

`goplusplus` includes a built-in RFC 7807 Problem Details exception handling suite:

```go
app.GET("/users/:id", func(c *gpp.Context) error {
    id := c.Param("id")
    if id == "" {
        return gpp.ErrBadRequest("User ID cannot be empty")
    }
    user, err := db.Find(id)
    if err != nil {
        return gpp.ErrNotFound("User with ID '" + id + "' not found")
    }
    return c.JSON(200, user)
})
```

Responses automatically format as RFC 7807 Problem Details JSON:
```json
{
  "type": "https://goplusplus.dev/errors/not-found",
  "title": "Resource Not Found",
  "status": 404,
  "detail": "User with ID '99' not found",
  "instance": "/users/99"
}
```

---

## 📄 Swagger / OpenAPI Interactive Documentation UI

`go++` includes built-in Swagger UI rendering for your API specs:

```go
app.GET("/swagger", middleware.SwaggerUI(openAPISpecJSON))
```

Accessing `http://localhost:8080/swagger` in your browser opens an interactive Swagger UI dashboard allowing full API exploration and testing.

---

## 📊 GraphQL Engine & Interactive Playground

`go++` supports GraphQL queries and mutations alongside an interactive Playground web UI:

```go
// 1. GraphQL Playground Web UI
app.GET("/graphql", middleware.GraphQLPlayground("/graphql"))

// 2. GraphQL Query Execution Engine
app.POST("/graphql", middleware.GraphQLHandler(func(query string, vars map[string]any) (any, error) {
    return gpp.H{"user": gpp.H{"id": "42", "name": "Alex"}}, nil
}))
```

Accessing `http://localhost:8080/graphql` in your browser launches the GraphQL Playground IDE.

---

## ⚡ gRPC Protocol Multiplexing

`go++` supports gRPC HTTP/2 multiplexing directly alongside standard REST endpoints:

```go
app.Use(middleware.GRPCMultiplex(grpcServerHandler))
```

---

## 🌐 HTTP Gateway -> gRPC & Dual-Mode Architecture

`go++` operates seamlessly as an **HTTP API Gateway** converting external HTTP/REST traffic into internal gRPC calls (or direct in-memory calls in Monolith mode):

```
Clients (Web/Mobile) ──[HTTP/REST]──> go++ Gateway ──┬──[In-Memory (0ms)]──> Monolith Modules
                                                    └──[gRPC / HTTP2]────> Microservices
```

### Gateway Implementation Example:

```go
// Gateway handler delegating to UserServiceClient (In-Memory or gRPC)
func (gw *GatewayServer) handleGetUser(c *gpp.Context) error {
    id := c.Param("id")
    user, err := gw.userClient.GetUser(c.Request.Context(), id)
    if err != nil {
        return gpp.NewHTTPError(http.StatusNotFound, err.Error())
    }
    return c.JSON(http.StatusOK, gpp.H{"data": user})
}
```

---

## 🔒 Security Middleware Suite

`go++` comes bundled with essential production security features in `go++/middleware`:

### 1. OWASP Security Headers
Automatically sets `X-Frame-Options`, `X-Content-Type-Options`, `X-XSS-Protection`, `Strict-Transport-Security` (HSTS), and `Content-Security-Policy`.

```go
app.Use(middleware.Security())
```

### 2. Thread-Safe Rate Limiting
In-memory token bucket rate limiter indexed by client IP to block brute-force and DoS attacks.

```go
app.Use(middleware.RateLimit(middleware.RateLimiterConfig{
    Rate:     20, // 20 requests per second
    Capacity: 50, // Burst up to 50 requests
}))
```

### 3. Cross-Origin Resource Sharing (CORS)
Configurable CORS middleware supporting preflight `OPTIONS` requests.

```go
app.Use(middleware.CORS())
```

### 4. Panic Recovery & Request Timeout
Traps unexpected runtime panics without crashing the process, and enforces strict execution timeouts.

```go
app.Use(middleware.Recovery(), middleware.Timeout(5 * time.Second))
```

---

## 📚 4-Hour Quick Reference Guide

### 1. Route Handlers & Method Chaining
```go
app.GET("/path", handler)
app.POST("/path", handler)
app.PUT("/path", handler)
app.DELETE("/path", handler)
app.PATCH("/path", handler)
app.OPTIONS("/path", handler)
```

### 2. Route Groups
```go
v1 := app.Group("/api/v1")
v1.Use(authMiddleware)

v1.GET("/profile", getProfileHandler)
v1.POST("/settings", updateSettingsHandler)
```

### 3. Context Utilities (`*gpp.Context`)

| Method | Description |
| :--- | :--- |
| `c.Param("id")` | Get URL path parameter (`/users/:id`) |
| `c.Query("q")` | Get URL query string parameter |
| `c.QueryDefault("page", "1")` | Get query parameter with fallback default |
| `c.BindJSON(&struct)` | Parse incoming JSON body into struct |
| `c.JSON(200, data)` | Serialize response struct or `gpp.H` to JSON |
| `c.String(200, "text")` | Send plain text response |
| `c.Set("key", val)` | Store request-scoped variable in context |
| `c.Get("key")` | Retrieve stored request-scoped variable |
| `c.Next()` | Execute next handler in middleware chain |
| `c.Abort()` | Stop execution of remaining handlers |

---

## 📂 Project Structure

```
go++/
├── gpp.go               # Core Engine & Server Lifecycle
├── context.go           # High-Performance Request/Response Context
├── router.go            # Route Group & Method Registration
├── tree.go              # Radix Tree Routing Node Engine
├── response.go          # Response Primitives (gpp.H, HTTPError)
├── middleware/          # Built-in Security & System Middleware
│   ├── logger.go        # Structured slog Logger
│   ├── recovery.go      # Panic Recovery
│   ├── security.go      # OWASP Security Headers
│   ├── cors.go          # CORS Preflight & Header Manager
│   ├── ratelimit.go     # Token Bucket Rate Limiter
│   └── timeout.go       # Request Execution Timeout
├── examples/
│   ├── basic/           # Simple REST API Example
│   └── secure_api/      # Production Secure Microservice
└── README.md
```

---

## 📄 License

Distributed under the MIT License. See `LICENSE` for more information.
