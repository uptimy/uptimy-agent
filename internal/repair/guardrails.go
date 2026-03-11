package repair

import (
	"fmt"
	"sync"
	"time"
)

// Guardrails enforces safety constraints on repair execution:
// rate limits per service, cooldowns per action, and allowed/forbidden actions.
type Guardrails struct {
	mu sync.Mutex

	// maxRepairsPerHour per rule.
	maxRepairs map[string]int
	// repairHistory tracks timestamps of repairs per rule.
	repairHistory map[string][]time.Time

	// cooldowns tracks the minimum duration between executions of each action.
	cooldowns map[string]time.Duration
	// lastExecution tracks when each action was last executed.
	lastExecution map[string]time.Time

	// allowedActions is the set of permitted action names.
	// If empty, all non-forbidden actions are allowed.
	allowedActions map[string]bool
	// forbiddenActions is the set of never-permitted action names.
	forbiddenActions map[string]bool
}

// NewGuardrails creates a Guardrails instance with default safety settings.
func NewGuardrails() *Guardrails {
	return &Guardrails{
		maxRepairs:    make(map[string]int),
		repairHistory: make(map[string][]time.Time),
		cooldowns:     make(map[string]time.Duration),
		lastExecution: make(map[string]time.Time),
		allowedActions: map[string]bool{
			"restart_pod":         true,
			"restart_container":   true,
			"restart_service":     true,
			"start_service":       true,
			"stop_service":        true,
			"rollback_deployment": true,
			"scale_replicas":      true,
			"wait":                true,
			"healthcheck":         true,
			"clear_temp":          true,
			"rotate_logs":         true,
			"webhook":             true,
		},
		forbiddenActions: map[string]bool{
			"shell_exec":     true,
			"delete_files":   true,
			"modify_secrets": true,
		},
	}
}

// SetMaxRepairsPerHour sets the rate limit for a repair rule.
func (g *Guardrails) SetMaxRepairsPerHour(rule string, max int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.maxRepairs[rule] = max
}

// SetCooldown sets the minimum duration between executions of an action.
func (g *Guardrails) SetCooldown(action string, duration time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cooldowns[action] = duration
}

// AllowAction adds an action to the allowed set.
func (g *Guardrails) AllowAction(name string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.allowedActions[name] = true
	delete(g.forbiddenActions, name)
}

// CheckAction verifies that executing an action is permitted.
// Returns nil if allowed, or an error describing why it's blocked.
func (g *Guardrails) CheckAction(actionName string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Check forbidden list.
	if g.forbiddenActions[actionName] {
		return fmt.Errorf("action %q is forbidden", actionName)
	}

	// Check allowed list (if non-empty).
	if len(g.allowedActions) > 0 && !g.allowedActions[actionName] {
		return fmt.Errorf("action %q is not in the allowed list", actionName)
	}

	// Check cooldown.
	if cooldown, ok := g.cooldowns[actionName]; ok {
		if last, ok := g.lastExecution[actionName]; ok {
			if time.Since(last) < cooldown {
				return fmt.Errorf("action %q is in cooldown (%.0fs remaining)",
					actionName, (cooldown - time.Since(last)).Seconds())
			}
		}
	}

	return nil
}

// CheckRateLimit verifies that a repair rule has not exceeded its hourly limit.
// Returns nil if allowed, or an error if the rate limit is exceeded.
func (g *Guardrails) CheckRateLimit(rule string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	max, ok := g.maxRepairs[rule]
	if !ok {
		// No limit configured — allow.
		return nil
	}

	// Prune entries older than 1 hour.
	cutoff := time.Now().Add(-1 * time.Hour)
	history := g.repairHistory[rule]
	pruned := make([]time.Time, 0, len(history))
	for _, t := range history {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	g.repairHistory[rule] = pruned

	if len(pruned) >= max {
		return fmt.Errorf("rule %q rate limit exceeded (%d/%d repairs in the last hour)",
			rule, len(pruned), max)
	}

	return nil
}

// RecordRepair records that a repair was executed for rate-limiting purposes.
func (g *Guardrails) RecordRepair(rule string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.repairHistory[rule] = append(g.repairHistory[rule], time.Now())
}

// RecordActionExecution records that an action was executed for cooldown tracking.
func (g *Guardrails) RecordActionExecution(actionName string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lastExecution[actionName] = time.Now()
}
