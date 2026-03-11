package checks

import (
	"fmt"
	"sync"
)

// Registry holds all registered checks.
type Registry struct {
	mu     sync.RWMutex
	checks map[string]Check
}

// NewRegistry creates an empty check registry.
func NewRegistry() *Registry {
	return &Registry{
		checks: make(map[string]Check),
	}
}

// Register adds a check to the registry.
// Returns an error if a check with the same name is already registered.
func (r *Registry) Register(c Check) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.checks[c.Name()]; exists {
		return fmt.Errorf("check %q already registered", c.Name())
	}
	r.checks[c.Name()] = c
	return nil
}

// Get returns a check by name.
func (r *Registry) Get(name string) (Check, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.checks[name]
	return c, ok
}

// All returns all registered checks.
func (r *Registry) All() []Check {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Check, 0, len(r.checks))
	for _, c := range r.checks {
		result = append(result, c)
	}
	return result
}

// Names returns the names of all registered checks.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.checks))
	for name := range r.checks {
		names = append(names, name)
	}
	return names
}
