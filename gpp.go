package gpp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/saifsilver/goplusplus/dbcore"
	"github.com/saifsilver/goplusplus/dbcore/seed"
)

// Version is the current release version of the goplusplus framework.
const Version = "v1.11.4"

// CLIOptions defines configuration for application binary CLI flags.
type CLIOptions struct {
	Client     *dbcore.Client
	Migrations []dbcore.Migration
	SeedPlans  []seed.Plan
	Args       []string // Optional override args for testing (defaults to os.Args if empty)
}

// Module defines the contract for self-contained domain modules in a modular monolith.
type Module interface {
	Name() string
	Register(group *RouterGroup)
}

// Engine is the central core of the go++ framework, serving as router, HTTP server wrapper, and handler.
type Engine struct {
	RouterGroup
	trees           map[string]*node
	pool            sync.Pool
	openapi         *OpenAPIGenerator
	graphqlMu       sync.RWMutex
	graphqlFields   graphql.Fields
	graphqlSchema   *graphql.Schema
	staticMu        sync.RWMutex
	staticRoutes    []staticRoute
	NotFoundHandler HandlerFunc
	ErrorHandler    func(c *Context, err error)
	Server          *http.Server
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	MaxHeaderBytes  int
	ShutdownTimeout time.Duration
	JSONBinding     JSONBindingConfig
}

// New creates a fresh instance of the go++ engine with high-performance default configurations.
func New() *Engine {
	engine := &Engine{
		RouterGroup: RouterGroup{
			prefix: "/",
		},
		trees:           make(map[string]*node),
		openapi:         newOpenAPIGenerator(),
		graphqlFields:   make(graphql.Fields),
		ReadTimeout:     15 * time.Second,
		WriteTimeout:    15 * time.Second,
		IdleTimeout:     60 * time.Second,
		MaxHeaderBytes:  1 << 20, // 1 MB
		ShutdownTimeout: 10 * time.Second,
		JSONBinding:     defaultJSONBindingConfig(),
	}
	engine.RouterGroup.engine = engine
	engine.pool.New = func() any {
		ctx := newContext()
		ctx.engine = engine
		return ctx
	}
	engine.NotFoundHandler = defaultNotFoundHandler
	engine.ErrorHandler = defaultErrorHandler
	return engine
}

// addRoute registers a route in the radix tree for a specific HTTP method.
func (engine *Engine) addRoute(method, path string, handlers HandlersChain) {
	if path == "" || path[0] != '/' {
		panic(fmt.Sprintf("gpp: path '%s' must begin with '/'", path))
	}
	if method == "" {
		panic("gpp: HTTP method cannot be empty")
	}
	if len(handlers) == 0 {
		panic("gpp: handler chain cannot be empty")
	}

	engine.openapi.RegisterRoute(method, path)

	root := engine.trees[method]
	if root == nil {
		root = new(node)
		engine.trees[method] = root
	}
	root.addRoute(path, handlers)
}

// RegisterModule mounts a self-contained domain module under a specific URL path prefix.
func (engine *Engine) RegisterModule(prefix string, module Module) {
	group := engine.Group(prefix)
	module.Register(group)
}

// RegisterModules mounts multiple domain modules onto the root engine.
func (engine *Engine) RegisterModules(modules ...Module) {
	for _, m := range modules {
		engine.RegisterModule("", m)
	}
}

// HandleCLI inspects binary CLI flags (e.g. ./myapp migrate, ./myapp seed, ./myapp migrate:fresh).
// If a CLI flag is detected, it executes the target database task and returns true.
func (engine *Engine) HandleCLI(opts CLIOptions) bool {
	args := os.Args
	if len(opts.Args) > 0 {
		args = opts.Args
	}

	if len(args) < 2 {
		return false
	}

	arg := strings.ToLower(args[1])
	ctx := context.Background()

	switch arg {
	case "migrate", "-migrate", "--migrate":
		slog.Info("gpp: Executing binary CLI database migrations...")
		if err := dbcore.AutoMigrate(ctx, opts.Client, opts.Migrations...); err != nil {
			slog.Error("gpp: Binary CLI Migration error", slog.String("error", err.Error()))
			return true
		}
		slog.Info("gpp: Binary CLI Migrations completed successfully!")
		return true

	case "seed", "-seed", "--seed":
		slog.Info("gpp: Executing binary CLI database seeders...")
		if err := seed.Run(ctx, opts.Client, opts.SeedPlans...); err != nil {
			slog.Error("gpp: Binary CLI Seeding error", slog.String("error", err.Error()))
			return true
		}
		slog.Info("gpp: Binary CLI Database Seeding completed successfully!")
		return true

	case "migrate:fresh", "-migrate:fresh", "-migrate-fresh":
		slog.Info("gpp: Executing binary CLI Fresh Migration & Seeding...")
		if err := dbcore.AutoMigrate(ctx, opts.Client, opts.Migrations...); err != nil {
			slog.Error("gpp: Binary CLI Migration error", slog.String("error", err.Error()))
			return true
		}
		if err := seed.Run(ctx, opts.Client, opts.SeedPlans...); err != nil {
			slog.Error("gpp: Binary CLI Seeding error", slog.String("error", err.Error()))
			return true
		}
		slog.Info("gpp: Binary CLI Fresh Migration & Seeding completed successfully!")
		return true
	}

	return false
}

// ServeHTTP implements the standard net/http.Handler interface for zero-alloc request processing.
func (engine *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	c := engine.pool.Get().(*Context)
	c.reset(w, req)

	root := engine.trees[req.Method]
	if root != nil {
		handlers := root.getValue(req.URL.Path, &c.Params)
		if handlers != nil {
			c.handlers = handlers
			if err := c.Next(); err != nil {
				engine.ErrorHandler(c, err)
			}
			engine.pool.Put(c)
			return
		}
	}
	if handlers := engine.matchStaticRoute(req.Method, req.URL.Path); handlers != nil {
		c.handlers = handlers
		if err := c.Next(); err != nil {
			engine.ErrorHandler(c, err)
		}
		engine.pool.Put(c)
		return
	}

	// OPTIONS preflight fallback for CORS global middleware
	if req.Method == http.MethodOptions {
		for _, methodRoot := range engine.trees {
			if methodRoot != nil {
				handlers := methodRoot.getValue(req.URL.Path, &c.Params)
				if handlers != nil {
					c.handlers = append(engine.RouterGroup.middlewares, func(c *Context) error {
						c.Status(http.StatusNoContent)
						return nil
					})
					if err := c.Next(); err != nil {
						engine.ErrorHandler(c, err)
					}
					engine.pool.Put(c)
					return
				}
			}
		}
	}

	// Route not found
	c.handlers = HandlersChain{engine.NotFoundHandler}
	if err := c.Next(); err != nil {
		engine.ErrorHandler(c, err)
	}
	engine.pool.Put(c)
}

// Listen starts an HTTP server listening on the provided address with graceful shutdown support.
func (engine *Engine) Listen(addr string) error {
	server := &http.Server{
		Addr:           addr,
		Handler:        engine,
		ReadTimeout:    engine.ReadTimeout,
		WriteTimeout:   engine.WriteTimeout,
		IdleTimeout:    engine.IdleTimeout,
		MaxHeaderBytes: engine.MaxHeaderBytes,
	}
	engine.Server = server

	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("go++ HTTP server starting", "addr", addr)
		serverErrors <- server.ListenAndServe()
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	case sig := <-shutdown:
		slog.Info("go++ server received shutdown signal", "signal", sig)
		ctx, cancel := context.WithTimeout(context.Background(), engine.ShutdownTimeout)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			server.Close()
			return fmt.Errorf("could not stop server gracefully: %w", err)
		}
		slog.Info("go++ server gracefully stopped")
		return nil
	}
}

// ListenTLS starts an HTTPS server listening on the provided address using TLS certificates.
func (engine *Engine) ListenTLS(addr, certFile, keyFile string) error {
	server := &http.Server{
		Addr:           addr,
		Handler:        engine,
		ReadTimeout:    engine.ReadTimeout,
		WriteTimeout:   engine.WriteTimeout,
		IdleTimeout:    engine.IdleTimeout,
		MaxHeaderBytes: engine.MaxHeaderBytes,
	}
	engine.Server = server

	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("go++ HTTPS server starting", "addr", addr)
		serverErrors <- server.ListenAndServeTLS(certFile, keyFile)
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server TLS error: %w", err)
		}
		return nil
	case sig := <-shutdown:
		slog.Info("go++ TLS server received shutdown signal", "signal", sig)
		ctx, cancel := context.WithTimeout(context.Background(), engine.ShutdownTimeout)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			server.Close()
			return fmt.Errorf("could not stop TLS server gracefully: %w", err)
		}
		slog.Info("go++ TLS server gracefully stopped")
		return nil
	}
}

func defaultNotFoundHandler(c *Context) error {
	return c.JSON(http.StatusNotFound, H{
		"code":    http.StatusNotFound,
		"message": "Resource Not Found",
		"path":    c.Request.URL.Path,
	})
}

func defaultErrorHandler(c *Context, err error) {
	if c.IsAborted() || c.written || c.response != nil && c.response.written {
		return
	}
	var internal *InternalFailure
	if errors.As(err, &internal) {
		logInternalRequestError(c, err)
		detail := "An internal server error occurred"
		if internal.PublicDetail != "" {
			detail = internal.PublicDetail
		}
		_ = c.Problem(ProblemDetails{
			Type: "https://goplusplus.dev/errors/" + internal.Category, Title: "Internal Server Error",
			Status: internal.Status, Detail: detail, Instance: c.Request.URL.Path, TraceID: c.RequestID(),
		})
		return
	}
	var probErr *ProblemDetails
	if errors.As(err, &probErr) {
		response := *probErr
		response.Errors = append([]FieldViolation(nil), probErr.Errors...)
		if response.Instance == "" {
			response.Instance = c.Request.URL.Path
		}
		if response.Status >= http.StatusInternalServerError {
			logInternalRequestError(c, err)
			response.Detail = "An internal server error occurred"
			response.Errors = nil
		}
		response.TraceID = c.RequestID()
		_ = c.Problem(response)
		return
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		if httpErr.Code >= http.StatusInternalServerError {
			logInternalRequestError(c, err)
			_ = c.Problem(ProblemDetails{
				Type: "https://goplusplus.dev/errors/internal-error", Title: "Internal Server Error",
				Status: httpErr.Code, Detail: "An internal server error occurred",
				Instance: c.Request.URL.Path, TraceID: c.RequestID(),
			})
			return
		}
		_ = c.JSON(httpErr.Code, H{
			"code":    httpErr.Code,
			"message": httpErr.Message,
			"details": httpErr.Details,
		})
		return
	}
	logInternalRequestError(c, err)
	_ = c.Problem(ProblemDetails{
		Type:     "https://goplusplus.dev/errors/internal-error",
		Title:    "Internal Server Error",
		Status:   http.StatusInternalServerError,
		Detail:   "An internal server error occurred",
		Instance: c.Request.URL.Path,
		TraceID:  c.RequestID(),
	})
}

func logInternalRequestError(c *Context, err error) {
	operation := "http.request"
	category := "internal_error"
	cause := err
	attributes := map[string]any(nil)
	var internal *InternalFailure
	if errors.As(err, &internal) {
		operation, category, cause, attributes = internal.Operation, internal.Category, internal.Cause, internal.Attributes
	}
	slog.Error("gpp: request failed", slog.Any("error", cause), slog.String("operation", operation),
		slog.String("category", category), slog.String("request_id", c.RequestID()),
		slog.String("path", c.Request.URL.Path), slog.Any("attributes", attributes))
}
