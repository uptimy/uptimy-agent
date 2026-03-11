package incidents_test

import (
	"context"
	"testing"
	"time"

	"github.com/uptimy/uptimy-agent/internal/checks"
	"github.com/uptimy/uptimy-agent/internal/incidents"
	"github.com/uptimy/uptimy-agent/internal/logging"
	"github.com/uptimy/uptimy-agent/internal/storage"
)

// memStore is a minimal in-memory storage.Store for testing.
type memStore struct{}

func (s *memStore) SaveIncident(inc *storage.Incident) error            { return nil }
func (s *memStore) GetIncident(id string) (*storage.Incident, error)    { return nil, nil }
func (s *memStore) ListIncidents() ([]*storage.Incident, error)         { return nil, nil }
func (s *memStore) DeleteIncident(id string) error                      { return nil }
func (s *memStore) SaveRepairRecord(r *storage.RepairRecord) error      { return nil }
func (s *memStore) ListRepairRecords() ([]*storage.RepairRecord, error) { return nil, nil }
func (s *memStore) SaveConfigCache(key string, data []byte) error       { return nil }
func (s *memStore) GetConfigCache(key string) ([]byte, error)           { return nil, nil }
func (s *memStore) Close() error                                        { return nil }

func TestManager_CreatesIncidentOnFailure(t *testing.T) {
	store := &memStore{}
	events := make(chan incidents.Event, 10)
	logger := logging.Nop()

	mgr := incidents.NewManager(store, events, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := make(chan checks.CheckResult, 10)
	go mgr.Run(ctx, results)

	// Send a failed check result.
	results <- checks.CheckResult{
		Name:      "test-check",
		Service:   "test-service",
		Status:    checks.StatusFailed,
		Timestamp: time.Now(),
	}

	// Wait for event.
	select {
	case ev := <-events:
		if ev.Type != incidents.EventOpened {
			t.Errorf("expected EventOpened, got %s", ev.Type)
		}
		if ev.Incident.CheckName != "test-check" {
			t.Errorf("expected check name test-check, got %s", ev.Incident.CheckName)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for incident event")
	}

	if mgr.ActiveCount() != 1 {
		t.Errorf("expected 1 active incident, got %d", mgr.ActiveCount())
	}
}

func TestManager_DeduplicatesExistingIncident(t *testing.T) {
	store := &memStore{}
	events := make(chan incidents.Event, 10)
	logger := logging.Nop()

	mgr := incidents.NewManager(store, events, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := make(chan checks.CheckResult, 10)
	go mgr.Run(ctx, results)

	// Send two failed results for the same check.
	for i := 0; i < 2; i++ {
		results <- checks.CheckResult{
			Name:    "test-check",
			Service: "test-service",
			Status:  checks.StatusFailed,
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Poll until the incident manager has processed both results.
	deadline := time.After(2 * time.Second)
	for {
		inc, ok := mgr.GetActive("test-check")
		if ok && inc.FailureCount >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for deduplication")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Should still have only 1 active incident (deduplicated).
	if mgr.ActiveCount() != 1 {
		t.Errorf("expected 1 active incident (deduplicated), got %d", mgr.ActiveCount())
	}

	inc, ok := mgr.GetActive("test-check")
	if !ok {
		t.Fatal("expected active incident for test-check")
	}
	if inc.FailureCount < 2 {
		t.Errorf("expected failure count >= 2, got %d", inc.FailureCount)
	}
}

func TestManager_ResolvesOnHealthy(t *testing.T) {
	store := &memStore{}
	events := make(chan incidents.Event, 10)
	logger := logging.Nop()

	mgr := incidents.NewManager(store, events, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := make(chan checks.CheckResult, 10)
	go mgr.Run(ctx, results)

	// Create an incident.
	results <- checks.CheckResult{
		Name:    "test-check",
		Service: "test-service",
		Status:  checks.StatusFailed,
	}

	// Wait until the incident is created before sending healthy.
	waitDeadline := time.After(2 * time.Second)
	for mgr.ActiveCount() == 0 {
		select {
		case <-waitDeadline:
			t.Fatal("timed out waiting for incident creation")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Resolve with healthy result.
	results <- checks.CheckResult{
		Name:    "test-check",
		Service: "test-service",
		Status:  checks.StatusHealthy,
	}

	// Wait for resolve event.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-events:
			if ev.Type == incidents.EventResolved {
				if mgr.ActiveCount() != 0 {
					t.Errorf("expected 0 active incidents after resolve, got %d", mgr.ActiveCount())
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for resolve event")
		}
	}
}
