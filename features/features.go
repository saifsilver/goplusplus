package features

import (
	"context"
	"sync"
)

type Manager struct {
	mu    sync.RWMutex
	flags map[string]bool
}

// NewManager creates a new feature flag manager.
func NewManager() *Manager {
	return &Manager{
		flags: map[string]bool{
			"new_checkout_v2":    true,
			"ai_recommendations": false,
		},
	}
}

// IsEnabled evaluates if a feature flag is enabled.
func (m *Manager) IsEnabled(ctx context.Context, featureKey string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.flags[featureKey]
}
