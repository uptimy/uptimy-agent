// Package repair provides the repair engine — recipe execution,
// guardrails, and deterministic action system for the Uptimy Agent.
package repair

import (
	"context"
	"fmt"
	"sync"
)

// Action is the interface every repair action must implement.
type Action interface {
	// Name returns a unique identifier for this action.
	Name() string
	// Execute performs the action with the given context and params.
	// Returns an error if the action fails.
	Execute(ctx context.Context, params map[string]string) error
}

// ActionRegistry is a thread-safe registry of repair actions.
type ActionRegistry struct {
	mu      sync.RWMutex
	actions map[string]Action
}

// NewActionRegistry creates an empty ActionRegistry.
func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{
		actions: make(map[string]Action),
	}
}

// Register adds an action to the registry. Returns an error if
// an action with the same name is already registered.
func (r *ActionRegistry) Register(a Action) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.actions[a.Name()]; exists {
		return fmt.Errorf("action %q already registered", a.Name())
	}
	r.actions[a.Name()] = a
	return nil
}

// Get retrieves an action by name.
func (r *ActionRegistry) Get(name string) (Action, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.actions[name]
	return a, ok
}

// Names returns the names of all registered actions.
func (r *ActionRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.actions))
	for name := range r.actions {
		names = append(names, name)
	}
	return names
}
