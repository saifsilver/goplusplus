package search

import (
	"context"
	"fmt"
	"regexp"
	"sync"
)

type Backend interface {
	Name() string
	Index(ctx context.Context, resource string, schema Schema, document Document) error
	Delete(ctx context.Context, resource, documentID string) error
	Search(ctx context.Context, resource string, schema Schema, request SearchRequest) (SearchResult, error)
}

type Resource struct {
	name    string
	schema  Schema
	backend Backend
	scope   ScopeFunc
}

type ScopeFunc func(ctx context.Context) ([]Filter, error)

type ResourceOption func(*Resource) error

type ExecutionError struct {
	cause error
}

func (e *ExecutionError) Error() string {
	return "search: execution failed"
}

func (e *ExecutionError) Unwrap() error {
	return e.cause
}

func newExecutionError(err error) error {
	if err == nil {
		return nil
	}
	return &ExecutionError{cause: err}
}

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

func (r *Resource) Name() string {
	return r.name
}

func (r *Resource) Schema() Schema {
	return r.schema
}

func (r *Resource) Index(ctx context.Context, document Document) error {
	if err := r.schema.ValidateDocument(document); err != nil {
		return err
	}
	return r.backend.Index(ctx, r.name, r.schema, document)
}

func (r *Resource) Delete(ctx context.Context, documentID string) error {
	if documentID == "" {
		return fmt.Errorf("search: document ID is required")
	}
	return r.backend.Delete(ctx, r.name, documentID)
}

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

type Registry struct {
	mu        sync.RWMutex
	resources map[string]*Resource
}

func NewRegistry(resources ...*Resource) (*Registry, error) {
	registry := &Registry{resources: make(map[string]*Resource, len(resources))}
	for _, resource := range resources {
		if err := registry.Register(resource); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

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

func (r *Registry) Resource(name string) (*Resource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	resource, exists := r.resources[name]
	return resource, exists
}

func (r *Registry) Search(ctx context.Context, name string, request SearchRequest) (SearchResult, error) {
	resource, exists := r.Resource(name)
	if !exists {
		return SearchResult{}, fmt.Errorf("search: resource %q is not registered", name)
	}
	return resource.Search(ctx, request)
}
