package storage_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/uptimy/uptimy-agent/internal/storage"
)

func tempStore(t *testing.T) (store storage.Store, cleanup func()) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	store, err := storage.NewBoltStore(path)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	return store, func() {
		_ = store.Close()
		_ = os.Remove(path)
	}
}

func TestBoltStore_IncidentCRUD(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	now := time.Now()
	inc := &storage.Incident{
		ID:           "inc-1",
		CheckName:    "test-check",
		Service:      "test-service",
		Status:       "open",
		FailureCount: 1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Save
	if err := store.SaveIncident(inc); err != nil {
		t.Fatalf("SaveIncident: %v", err)
	}

	// Get
	got, err := store.GetIncident("inc-1")
	if err != nil {
		t.Fatalf("GetIncident: %v", err)
	}
	if got.ID != "inc-1" || got.Service != "test-service" {
		t.Errorf("unexpected incident: %+v", got)
	}

	// List
	all, err := store.ListIncidents()
	if err != nil {
		t.Fatalf("ListIncidents: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 incident, got %d", len(all))
	}

	// Delete
	if err := store.DeleteIncident("inc-1"); err != nil {
		t.Fatalf("DeleteIncident: %v", err)
	}
	all, _ = store.ListIncidents()
	if len(all) != 0 {
		t.Errorf("expected 0 incidents after delete, got %d", len(all))
	}
}

func TestBoltStore_RepairRecords(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	now := time.Now()
	rec := &storage.RepairRecord{
		ID:         "rep-1",
		IncidentID: "inc-1",
		Rule:       "test-rule",
		Recipe:     "test-recipe",
		Status:     "success",
		StartedAt:  now,
		FinishedAt: now,
	}

	if err := store.SaveRepairRecord(rec); err != nil {
		t.Fatalf("SaveRepairRecord: %v", err)
	}

	records, err := store.ListRepairRecords()
	if err != nil {
		t.Fatalf("ListRepairRecords: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("expected 1 record, got %d", len(records))
	}
	if records[0].Rule != "test-rule" {
		t.Errorf("unexpected rule: %s", records[0].Rule)
	}
}

func TestBoltStore_ConfigCache(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	data := []byte(`{"key": "value"}`)
	if err := store.SaveConfigCache("test-key", data); err != nil {
		t.Fatalf("SaveConfigCache: %v", err)
	}

	got, err := store.GetConfigCache("test-key")
	if err != nil {
		t.Fatalf("GetConfigCache: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("expected %s, got %s", string(data), string(got))
	}
}

func TestBoltStore_IncidentNotFound(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	_, err := store.GetIncident("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent incident")
	}
}

func TestBoltStore_OpenNonexistentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deep", "test.db")
	store, err := storage.NewBoltStore(path)
	if err != nil {
		t.Fatalf("should create nested dirs: %v", err)
	}
	_ = store.Close()
}

func TestBoltStore_CloseIdempotent(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	if err := store.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	// Second close should not panic.
	_ = store.Close()
}
