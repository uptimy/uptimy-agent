// Package incidents defines the Incident model and lifecycle events.
package incidents

import "time"

// Status represents the lifecycle state of an incident.
type Status = string

// Status values.
const (
	StatusOpen      Status = "open"
	StatusRepairing Status = "repairing"
	StatusVerifying Status = "verifying"
	StatusResolved  Status = "resolved"
	StatusFailed    Status = "failed"
)

// Incident tracks an active problem detected by the check engine.
type Incident struct {
	ID           string     `json:"id"`
	CheckName    string     `json:"check_name"`
	Service      string     `json:"service"`
	Status       Status     `json:"status"`
	FailureCount int        `json:"failure_count"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
}

// Copy returns a shallow copy of the Incident to avoid data races
// when emitting events outside of a lock.
func (inc *Incident) Copy() *Incident {
	cp := *inc
	if inc.ResolvedAt != nil {
		resolved := *inc.ResolvedAt
		cp.ResolvedAt = &resolved
	}
	return &cp
}

// EventType describes the kind of incident lifecycle change.
type EventType string

// EventType values.
const (
	EventOpened    EventType = "opened"
	EventUpdated   EventType = "updated"
	EventRepairing EventType = "repairing"
	EventResolved  EventType = "resolved"
	EventFailed    EventType = "failed"
)

// Event is emitted whenever an incident changes state.
type Event struct {
	Incident *Incident
	Type     EventType
}
