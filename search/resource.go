package search

import (
	"context"
	"fmt"
	"regexp"
	"sync"
)

// Backend stores, deletes, and queries documents for named resources.
type Backend interface {
	Name() string
	Index(ctx context.Context, resource string, schema Schema, document Document) error
	Delete(ctx context.Context, resource, documentID string) error
	Search(ctx context.Context, resource string, schema Schema, request SearchRequest) (SearchResult, error)
}

// Resource binds a validated schema and optional authorization scope to a backend.
type Resource struct {
	name    string
	schema  Schema
	backend Backend
	scope   ScopeFunc
}

// ScopeFunc returns mandatory filters for the current request context.
type ScopeFunc func(ctx context.Context) ([]Filter, error)

// ResourceOption configures a Resource during construction.
type ResourceOption func(*Resource) error

// ExecutionError hides backend details while preserving the cause for errors.Is/As.
type ExecutionError struct {
	cause error
}

// Error implements error without exposing storage or scope details to callers.
func (e *ExecutionError) Error() string {
	return "search: execution failed"
}

// Unwrap returns the internal execution cause.
func (e *ExecutionError) Unwrap() error {
	return e.cause
}

func newExecutionError(err error) error {
	if err == nil {
		return nil
	}
	return &ExecutionError{cause: err}
}

// WithScope adds mandatory filters resolved from each request context.
func WithScope(scope ScopeFunc) ResourceOption {
	return func(resource *Resource) error {
		if scope == nil {
			return fmt.Errorf("search: scope function cannot be nil")
		}
		resource.scope = scope
		return nil
	}
}

var resourceNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

// NewResource validates and creates a named searchable resource.
func NewResource(name string, schema Schema, backend Backend, options ...ResourceOption) (*Resource, error) {
	if !resourceNamePattern.MatchString(name) {
		return nil, fmt.Errorf("search: invalid resource name %q", name)
	}
	if backend == nil {
		return nil, fmt.Errorf("search: backend is required for resource %q", name)
	}
	for key, definition := range schema.Attributes {
		if key != definition.Key {
			return nil, fmt.Errorf("search: schema key %q does not match attribute key %q", key, definition.Key)
		}
		if err := validateDefinition(definition); err != nil {
			return nil, err
		}
	}
	resource := &Resource{name: name, schema: schema, backend: backend}
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("search: resource option cannot be nil")
		}
		if err := option(resource); err != nil {
			return nil, err
		}
	}
	return resource, nil
}

// Name returns the stable resource name used by backends and registries.
func (r *Resource) Name() string {
	return r.name
}

// Schema returns a copy of the resource schema.
func (r *Resource) Schema() Schema {
	return r.schema
}

// Index validates and stores a document.
func (r *Resource) Index(ctx context.Context, document Document) error {
	if err := r.schema.ValidateDocument(document); err != nil {
		return err
	}
	return r.backend.Index(ctx, r.name, r.schema, document)
}

// Delete removes a document by ID.
func (r *Resource) Delete(ctx context.Context, documentID string) error {
	if documentID == "" {
		return fmt.Errorf("search: document ID is required")
	}
	return r.backend.Delete(ctx, r.name, documentID)
}

// Search normalizes a request, enforces scope filters, and queries the backend.
func (r *Resource) Search(ctx context.Context, request SearchRequest) (SearchResult, error) {
	normalized, err := r.schema.NormalizeRequest(request)
	if err != nil {
		return SearchResult{}, err
	}
	if r.scope != nil {
		filters, err := r.scope(ctx)
		if err != nil {
			return SearchResult{}, newExecutionError(fmt.Errorf("resolve resource scope: %w", err))
		}
		for index := range filters {
			filters[index].mandatory = true
		}
		normalized.Filters = append(normalized.Filters, filters...)
		normalized, err = r.schema.NormalizeRequest(normalized)
		if err != nil {
			return SearchResult{}, newExecutionError(fmt.Errorf("invalid resource scope: %w", err))
		}
	}
	result, err := r.backend.Search(ctx, r.name, r.schema, normalized)
	if err != nil {
		return SearchResult{}, newExecutionError(err)
	}
	return result, nil
}

// Registry owns a concurrency-safe collection of named resources.
type Registry struct {
	mu        sync.RWMutex
	resources map[string]*Resource
}

// NewRegistry creates a registry and rejects nil or duplicate resources.
func NewRegistry(resources ...*Resource) (*Registry, error) {
	registry := &Registry{resources: make(map[string]*Resource, len(resources))}
	for _, resource := range resources {
		if err := registry.Register(resource); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// Register adds a resource if its name is not already registered.
func (r *Registry) Register(resource *Resource) error {
	if resource == nil {
		return fmt.Errorf("search: cannot register a nil resource")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.resources[resource.Name()]; exists {
		return fmt.Errorf("search: resource %q is already registered", resource.Name())
	}
	r.resources[resource.Name()] = resource
	return nil
}

// Resource returns a resource and whether it is registered.
func (r *Registry) Resource(name string) (*Resource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	resource, exists := r.resources[name]
	return resource, exists
}

// Search dispatches a request to a registered resource.
func (r *Registry) Search(ctx context.Context, name string, request SearchRequest) (SearchResult, error) {
	resource, exists := r.Resource(name)
	if !exists {
		return SearchResult{}, fmt.Errorf("search: resource %q is not registered", name)
	}
	return resource.Search(ctx, request)
}
