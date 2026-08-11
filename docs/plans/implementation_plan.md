# Implementation Plan - `goplusplus` Generic Framework Enhancements (`v1.8.0`)

This plan outlines generic, reusable framework features to reduce application boilerplate by **70%–80%** across any Go app built using `goplusplus`.

---

## Proposed Generic Framework Changes

### 1. `Context` Path Parameter Type Parsers (`context.go`)
Eliminates manual `strconv.ParseInt` / `strconv.Atoi` calls across application handlers.

- **`c.ParamInt64(key string, defaultValue ...int64) int64`**: Parses path parameters (e.g. `/todos/:id`) directly into `int64`.
- **`c.ParamInt(key string, defaultValue ...int) int`**: Parses path parameters directly into `int`.

```go
// Before:
idStr := c.Param("id")
todoID, err := strconv.ParseInt(idStr, 10, 64)

// After:
todoID := c.ParamInt64("id")
```

---

### 2. `Context` Identity & Auth Helpers (`context.go`)
Eliminates custom `getUserID(c)` helper boilerplate across application code.

- **`c.UserID() int64`**: Returns the active authenticated user ID (`c.GetInt64("user_id")`).
- **`c.RequireUserID() (int64, error)`**: Returns `userID`; if 0 or unauthenticated, automatically returns `gpp.ErrUnauthorized("Unauthorized request")`.

```go
// Before:
userID, ok := getUserID(c)
if !ok {
    return c.JSON(http.StatusUnauthorized, gpp.H{"error": "Unauthorized"})
}

// After:
userID, err := c.RequireUserID()
if err != nil { return err }
```

---

### 3. Response Shorthand Methods (`context.go`)
Provides clean response methods without typing `http.StatusOK`, `http.StatusCreated`, or `gpp.H{"error": ...}`.

- **`c.OK(data any) error`**: Shorthand for `c.JSON(http.StatusOK, data)`.
- **`c.Created(data any) error`**: Shorthand for `c.JSON(http.StatusCreated, data)`.
- **`c.NoContent() error`**: Writes HTTP `204 No Content`.

```go
// Before:
return c.JSON(http.StatusOK, dto.TodoResponse{Todo: todo})

// After:
return c.OK(dto.TodoResponse{Todo: todo})
```

---

### 4. First-Class Password & Token Utilities (`auth/auth.go`)
Provides zero-dependency password hashing & JWT token creation natively in `goplusplus/auth`.

- **`auth.HashPassword(password string) string`**
- **`auth.VerifyPassword(password, hash string) bool`**
- **`auth.GenerateToken(userID int64, secret string, ttl time.Duration) (string, error)`**

---

### 5. Generic Auto-CRUD Resource Router (`gpp.BindResource`)
Allows developers to mount full RESTful CRUD endpoints for any entity with 1 line of code.

- **`group.BindResource[T]("/todos", repository)`**: Binds `GET /`, `GET /:id`, `POST /`, `PUT /:id`, and `DELETE /:id` backed by `dbcore.Repository[T]`.

```go
// 1 line creates complete REST API for Todos!
v1.BindResource("/todos", dbcore.NewRepository[Todo](db, "todos"))
```

---

## Summary of Affected Files in `goplusplus`

### [MODIFY] [context.go](file:///Users/saifsulaiman/Documents/websites/go++/context.go)
- Add `ParamInt64`, `ParamInt`, `UserID`, `RequireUserID`, `OK`, `Created`, `NoContent`.

### [MODIFY] [gpp.go](file:///Users/saifsulaiman/Documents/websites/go++/gpp.go)
- Add `BindResource` generic CRUD binder on `Engine` and `RouterGroup`.

### [MODIFY] [auth/auth.go](file:///Users/saifsulaiman/Documents/websites/go++/auth/auth.go)
- Add `HashPassword`, `VerifyPassword`, `GenerateToken`.

### [NEW] [context_enhancements_test.go](file:///Users/saifsulaiman/Documents/websites/go++/context_enhancements_test.go)
- Unit tests verifying `ParamInt64`, `UserID`, `RequireUserID`, `OK`, `Created`, `NoContent`.

---

## Verification Plan

1. **Automated Tests**:
   - `go test -v -cover ./...` across all framework packages.
   - Verify 100% statement coverage on newly added functions.
2. **Backwards Compatibility**:
   - Ensure all existing APIs, middleware, and tests remain 100% compatible.
