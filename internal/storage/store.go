// Package storage defines the Store interface and provides a BoltDB
// implementation for persisting agent state locally.
package storage

import (
	"time"
)

// Store is the interface for local state persistence.
// All implementations must be safe for concurrent use.
type Store interface {
	// Incidents
	SaveIncident(incident *Incident) error
	GetIncident(id string) (*Incident, error)
	ListIncidents() ([]*Incident, error)
	DeleteIncident(id string) error

	// Repair records
	SaveRepairRecord(record *RepairRecord) error
	ListRepairRecords() ([]*RepairRecord, error)

	// Configuration cache
	SaveConfigCache(key string, data []byte) error
	GetConfigCache(key string) ([]byte, error)

	// Close releases all resources.
	Close() error
}

// Incident represents a persisted incident record.
// This is the storage/persistence model. The domain model lives in
// internal/incidents.Incident. Conversion between the two happens in
// the incident manager (persistIncident / Rehydrate).
type Incident struct {
	ID           string     `json:"id"`
	CheckName    string     `json:"check_name"`
	Service      string     `json:"service"`
	Status       string     `json:"status"`
	FailureCount int        `json:"failure_count"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
}

// RepairRecord represents a persisted repair execution record.
type RepairRecord struct {
	ID         string    `json:"id"`
	IncidentID string    `json:"incident_id"`
	Rule       string    `json:"rule"`
	Recipe     string    `json:"recipe"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Error      string    `json:"error,omitempty"`
}
