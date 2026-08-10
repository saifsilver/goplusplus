package gpp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Version is the current release version of the goplusplus framework.
const Version = "v1.0.0"

// Module defines the contract for self-contained domain modules in a modular monolith.
type Module interface {
	Name() string
	Register(group *RouterGroup)
}

// Engine is the central core of the go++ framework, serving as router, HTTP server wrapper, and handler.
type Engine struct {
	RouterGroup
	trees            map[string]*node
	pool             sync.Pool
	openapi          *OpenAPIGenerator
	NotFoundHandler  HandlerFunc
	ErrorHandler     func(c *Context, err error)
	Server           *http.Server
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	IdleTimeout      time.Duration
	MaxHeaderBytes   int
	ShutdownTimeout  time.Duration
}

// New creates a fresh instance of the go++ engine with high-performance default configurations.
func New() *Engine {
	engine := &Engine{
		RouterGroup: RouterGroup{
			prefix: "/",
		},
		trees:           make(map[string]*node),
		openapi:         newOpenAPIGenerator(),
		ReadTimeout:     15 * time.Second,
		WriteTimeout:    15 * time.Second,
		IdleTimeout:     60 * time.Second,
		MaxHeaderBytes:  1 << 20, // 1 MB
		ShutdownTimeout: 10 * time.Second,
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
	if c.IsAborted() {
		return
	}
	var probErr *ProblemDetails
	if errors.As(err, &probErr) {
		if probErr.Instance == "" {
			probErr.Instance = c.Request.URL.Path
		}
		_ = c.JSON(probErr.Status, probErr)
		return
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		_ = c.JSON(httpErr.Code, H{
			"code":    httpErr.Code,
			"message": httpErr.Message,
			"details": httpErr.Details,
		})
		return
	}
	_ = c.JSON(http.StatusInternalServerError, ProblemDetails{
		Type:     "https://goplusplus.dev/errors/internal-error",
		Title:    "Internal Server Error",
		Status:   http.StatusInternalServerError,
		Detail:   err.Error(),
		Instance: c.Request.URL.Path,
	})
}
