// Package checks provides the check engine — scheduling, executing,
// and collecting health check results for the Uptimy Agent.
package checks

import (
	"context"
	"encoding/json"
	"time"
)

// CheckStatus represents the result of a health check.
type CheckStatus string

const (
	StatusHealthy  CheckStatus = "healthy"
	StatusDegraded CheckStatus = "degraded"
	StatusFailed   CheckStatus = "failed"
)

// Check is the interface every health-check probe must implement.
type Check interface {
	// Name returns a unique identifier for this check.
	Name() string
	// Run executes the check and returns the result.
	Run(ctx context.Context) CheckResult
}

// CheckResult holds the outcome of a single check execution.
type CheckResult struct {
	Name      string            `json:"name"`
	Service   string            `json:"service"`
	Status    CheckStatus       `json:"status"`
	Error     error             `json:"-"` // use MarshalJSON for proper serialization
	Timestamp time.Time         `json:"timestamp"`
	Duration  time.Duration     `json:"duration"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// MarshalJSON implements json.Marshaler so the Error field serializes
// as a string instead of the default empty-object representation of
// the error interface.
func (r CheckResult) MarshalJSON() ([]byte, error) {
	type Alias CheckResult
	aux := struct {
		Alias
		Error string `json:"error,omitempty"`
	}{
		Alias: Alias(r),
	}
	if r.Error != nil {
		aux.Error = r.Error.Error()
	}
	return json.Marshal(aux)
}
