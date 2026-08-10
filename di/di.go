package di

import (
	"fmt"
	"reflect"
	"sync"
)

// Container manages constructor dependency injection and application lifecycle hooks (Uber FX style).
type Container struct {
	mu           sync.RWMutex
	providers    map[reflect.Type]reflect.Value
	onStartHooks []func() error
	onStopHooks  []func() error
}

// New creates a new DI container instance.
func New() *Container {
	return &Container{
		providers: make(map[reflect.Type]reflect.Value),
	}
}

// Provide registers a constructor component or instance into the container.
func (c *Container) Provide(target any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	val := reflect.ValueOf(target)
	c.providers[val.Type()] = val
}

// OnStart registers a lifecycle hook executed when the application starts.
func (c *Container) OnStart(hook func() error) {
	c.mu.Lock()
	c.onStartHooks = append(c.onStartHooks, hook)
	c.mu.Unlock()
}

// OnStop registers a lifecycle hook executed during graceful application shutdown.
func (c *Container) OnStop(hook func() error) {
	c.mu.Lock()
	c.onStopHooks = append(c.onStopHooks, hook)
	c.mu.Unlock()
}

// Start executes all registered OnStart lifecycle hooks.
func (c *Container) Start() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, hook := range c.onStartHooks {
		if err := hook(); err != nil {
			return fmt.Errorf("di: OnStart hook failed: %w", err)
		}
	}
	return nil
}

// Stop executes all registered OnStop lifecycle hooks.
func (c *Container) Stop() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, hook := range c.onStopHooks {
		if err := hook(); err != nil {
			return fmt.Errorf("di: OnStop hook failed: %w", err)
		}
	}
	return nil
}
