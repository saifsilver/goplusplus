# `goplusplus` Framework Guide for AI Coding Agents (Codex, Claude, Cursor, Copilot)

This document provides system instructions, architectural rules, and API specifications for AI coding agents to instantly generate production-ready Go applications using `goplusplus` (`github.com/saifsilver/goplusplus`).

---

## 🚀 Core Rules & Conventions

1. **Import Path**: Always import `github.com/saifsilver/goplusplus` as package `gpp`:
   ```go
   import (
       gpp "github.com/saifsilver/goplusplus"
       "github.com/saifsilver/goplusplus/middleware"
   )
   ```
2. **Boilerplate Minimization**: Use high-level `Context` methods (`c.JSON()`, `c.BindAndValidate()`, `c.Param()`, `gpp.H`) to keep handlers under 10 lines of code.
3. **Error Handling**: Return standard RFC 7807 Problem Details errors (`gpp.ErrNotFound()`, `gpp.ErrBadRequest()`, `gpp.ErrUnauthorized()`, `gpp.ErrInternal()`) directly from handlers.

---

## 🧱 1. Master Application Template

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
	app.Use(
		middleware.Observability(),
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
		middleware.CORS(),
	)

	// API Grouping
	v1 := app.Group("/api/v1")
	v1.GET("/users/:id", func(c *gpp.Context) error {
		id := c.Param("id")
		if id == "0" {
			return gpp.ErrNotFound("User with ID '0' not found")
		}
		return c.JSON(http.StatusOK, gpp.H{
			"id":     id,
			"name":   "Alex Dev",
			"status": "active",
		})
	})

	// Triple-Auto Protocol Endpoints
	app.GET("/metrics", middleware.Prometheus())
	app.GET("/swagger", app.AutoSwaggerUI())
	app.GET("/graphql", app.AutoGraphQLPlayground("/graphql"))
	app.POST("/graphql", app.AutoGraphQLHandler())

	fmt.Println("🚀 Server running on http://localhost:8080")
	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
```

---

## 🏛️ 2. Modular Monolith & Microservices Architecture

Define business domains as `gpp.Module` implementations:

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

## 🗄️ 3. Database Access Layer (`dbcore`)

`dbcore` handles PgBouncer transaction mode, primary/replica routing, and slow-query logging:

```go
import "github.com/saifsilver/goplusplus/dbcore"

db, _ := dbcore.NewClient(ctx, dbcore.Config{
    RWDSN: "postgres://app_rw:...@pgbouncer:6432/app_rw",
    RODSN: "postgres://app_ro:...@pgbouncer:6432/app_ro",
    SlowQuery: dbcore.SlowQueryConfig{Threshold: 250 * time.Millisecond},
})

// Attach query name for observability
reqCtx := dbcore.WithQueryName(c.Request.Context(), "user.search")

// Read Query (Auto-routes to Replica RO)
err := db.Query(reqCtx, "SELECT id, name FROM users WHERE id=$1", fn, "42")

// Transaction Write (Auto-routes to Primary RW)
err = db.InTx(reqCtx, func(tx *sql.Tx) error {
    // Write logic
    return nil
})
```

---

## 🔐 4. Auth, RBAC, ABAC & MFA

```go
import "github.com/saifsilver/goplusplus/auth"

adminGroup := app.Group("/api/admin")
adminGroup.Use(
    auth.Authenticate("jwt_secret"),
    auth.RequireRoles("admin"),
    auth.RequirePolicy(func(u *auth.UserClaims) bool {
        return u.Attributes["department"] == "finance"
    }),
    auth.RequireMFA("totp_secret"),
)
```

---

## ⚡ 5. API Quick Reference Cheat Sheet

| Feature | Code Snippet |
| :--- | :--- |
| **JSON Response** | `c.JSON(200, gpp.H{"key": "val"})` |
| **Bind Body** | `c.BindJSON(&struct)` |
| **Bind & Validate** | `c.BindAndValidate(&struct)` |
| **Path Parameter** | `c.Param("id")` |
| **Query Parameter** | `c.Query("q")` or `c.QueryDefault("page", "1")` |
| **Get Typed Context Key** | `userID := c.GetInt64("user_id")` or `id := c.GetInt("id")` |
| **Get Context Any Value** | `val := c.GetAny("key")` or `val := c.Value("key")` |
| **Get Generic Context Value** | `val, ok := gpp.GetAs[User](c, "user")` |
| **Request ID** | `c.RequestID()` or `app.Use(middleware.RequestID())` |
| **Idempotency** | `app.Use(middleware.Idempotency())` |
| **Singleflight Deduplication** | `app.Use(middleware.Singleflight())` |
| **Auto DB Migrations** | `dbcore.AutoMigrate(ctx, db, migrations...)` |
| **DB Seeder & Faker** | `seed.Run(ctx, db, seed.Plan{Table: "users", Count: 50, Factory: ...})` |
| **Binary CLI Flag Handler** | `app.HandleCLI(gpp.CLIOptions{Client: db, Migrations: ..., SeedPlans: ...})` |
| **CLI App Skeleton Generator** | `gpp new <app_name>` |
| **CLI Code Generators** | `gpp gen <module|middleware|migration|handler> <name>` |
| **Offset Pagination** | `c.Paginate(200, items, page, limit, total)` |
| **Cursor Pagination** | `c.PaginateCursor(200, items, nextCursor, hasMore, limit)` |
| **Ephemeral Session Pagination** | `dbcore.MaterializePagination(ctx, db, query, 10*time.Minute)` |
| **Bounded Memory Cache** | `cache.NewBoundedMemoryStore(10000)` (bounded space capacity) |
| **Auto SQL Query Cache** | `ctx := dbcore.WithCache(c.Request.Context(), 30*time.Second)` |
| **404 Error** | `return gpp.ErrNotFound("Item not found")` |
| **400 Error** | `return gpp.ErrBadRequest("Invalid field")` |
| **401 Error** | `return gpp.ErrUnauthorized("Token expired")` |
| **403 Error** | `return gpp.ErrForbidden("Access denied")` |
| **Prometheus** | `app.GET("/metrics", middleware.Prometheus())` |
| **Auto-Swagger** | `app.GET("/swagger", app.AutoSwaggerUI())` |
| **Auto-GraphQL** | `app.GET("/graphql", app.AutoGraphQLPlayground("/graphql"))` |
| **K8s Health** | `app.GET("/healthz/readiness", healthChecker.Readiness())` |
| **Embedded React SPA** | `app.StaticFS("/", distFS)` (with automatic `index.html` fallback) |
