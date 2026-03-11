package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	bucketIncidents   = []byte("incidents")
	bucketRepairs     = []byte("repairs")
	bucketConfigCache = []byte("config_cache")
)

// BoltStore implements Store using BoltDB.
type BoltStore struct {
	db        *bolt.DB
	closeOnce sync.Once
}

// NewBoltStore opens or creates a BoltDB database at the given path.
func NewBoltStore(path string) (*BoltStore, error) {
	// Ensure parent directories exist.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("creating storage directory: %w", err)
	}

	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening bolt database at %s: %w", path, err)
	}

	// Ensure buckets exist.
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{bucketIncidents, bucketRepairs, bucketConfigCache} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return fmt.Errorf("creating bucket %s: %w", b, err)
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	return &BoltStore{db: db}, nil
}

// SaveIncident persists an incident record.
func (s *BoltStore) SaveIncident(incident *Incident) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketIncidents)
		data, err := json.Marshal(incident)
		if err != nil {
			return fmt.Errorf("marshaling incident: %w", err)
		}
		return b.Put([]byte(incident.ID), data)
	})
}

// GetIncident retrieves a single incident by ID.
func (s *BoltStore) GetIncident(id string) (*Incident, error) {
	var incident Incident
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketIncidents)
		data := b.Get([]byte(id))
		if data == nil {
			return fmt.Errorf("incident %q not found", id)
		}
		return json.Unmarshal(data, &incident)
	})
	if err != nil {
		return nil, err
	}
	return &incident, nil
}

// ListIncidents returns all persisted incidents.
func (s *BoltStore) ListIncidents() ([]*Incident, error) {
	var incidents []*Incident
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketIncidents)
		return b.ForEach(func(k, v []byte) error {
			var inc Incident
			if err := json.Unmarshal(v, &inc); err != nil {
				return err
			}
			incidents = append(incidents, &inc)
			return nil
		})
	})
	return incidents, err
}

// DeleteIncident removes an incident by ID.
func (s *BoltStore) DeleteIncident(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketIncidents).Delete([]byte(id))
	})
}

// SaveRepairRecord persists a repair execution record.
func (s *BoltStore) SaveRepairRecord(record *RepairRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketRepairs)
		data, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("marshaling repair record: %w", err)
		}
		return b.Put([]byte(record.ID), data)
	})
}

// ListRepairRecords returns all persisted repair records.
func (s *BoltStore) ListRepairRecords() ([]*RepairRecord, error) {
	var records []*RepairRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketRepairs)
		return b.ForEach(func(k, v []byte) error {
			var rec RepairRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			records = append(records, &rec)
			return nil
		})
	})
	return records, err
}

// SaveConfigCache persists a configuration cache entry.
func (s *BoltStore) SaveConfigCache(key string, data []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketConfigCache).Put([]byte(key), data)
	})
}

// GetConfigCache retrieves a cached configuration entry.
func (s *BoltStore) GetConfigCache(key string) ([]byte, error) {
	var data []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketConfigCache).Get([]byte(key))
		if v == nil {
			return fmt.Errorf("config cache key %q not found", key)
		}
		data = make([]byte, len(v))
		copy(data, v)
		return nil
	})
	return data, err
}

// Close closes the underlying BoltDB database.
// It is safe to call Close multiple times from any goroutine.
func (s *BoltStore) Close() error {
	var err error
	s.closeOnce.Do(func() {
		if s.db != nil {
			err = s.db.Close()
		}
	})
	return err
}
