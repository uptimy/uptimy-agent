package incidents

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uptimy/uptimy-agent/internal/checks"
	"github.com/uptimy/uptimy-agent/internal/storage"
	"go.uber.org/zap"
)

// MetricsRecorder is the subset of metrics.Recorder used by the incident
// manager to record check results and incident lifecycle metrics.
type MetricsRecorder interface {
	RecordCheckResult(result *checks.CheckResult)
	RecordIncidentOpened(inc *Incident)
	RecordIncidentResolved(inc *Incident, resolutionSeconds float64)
}

// Manager processes check results, creates and deduplicates incidents,
// manages their lifecycle, and emits events for downstream consumers.
type Manager struct {
	logger   *zap.SugaredLogger
	store    storage.Store
	events   chan Event
	recorder MetricsRecorder

	mu     sync.RWMutex
	active map[string]*Incident // keyed by check name

	idCounter atomic.Int64
}

// NewManager creates an IncidentManager.
// events channel is where incident lifecycle events are published.
func NewManager(store storage.Store, events chan Event, recorder MetricsRecorder, logger *zap.SugaredLogger) *Manager {
	m := &Manager{
		logger:   logger,
		store:    store,
		events:   events,
		recorder: recorder,
		active:   make(map[string]*Incident),
	}
	// Initialize the atomic counter to a timestamp-based value so IDs
	// never collide with IDs generated before a restart.
	m.idCounter.Store(time.Now().UnixMilli())
	return m
}

// Rehydrate loads active (non-resolved) incidents from storage so repairs
// can continue after an agent restart.
func (m *Manager) Rehydrate() error {
	stored, err := m.store.ListIncidents()
	if err != nil {
		return fmt.Errorf("listing stored incidents: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, si := range stored {
		if si.Status == StatusResolved {
			continue
		}
		inc := &Incident{
			ID:           si.ID,
			CheckName:    si.CheckName,
			Service:      si.Service,
			Status:       si.Status,
			FailureCount: si.FailureCount,
			CreatedAt:    si.CreatedAt,
			UpdatedAt:    si.UpdatedAt,
			ResolvedAt:   si.ResolvedAt,
		}
		m.active[inc.CheckName] = inc
		m.logger.Infow("rehydrated incident", "id", inc.ID, "check", inc.CheckName, "status", inc.Status)
	}

	return nil
}

// Run processes check results from the results channel until ctx is canceled.
func (m *Manager) Run(ctx context.Context, results <-chan checks.CheckResult) {
	for {
		select {
		case <-ctx.Done():
			return
		case result := <-results:
			m.processResult(&result)
		}
	}
}

// processResult handles a single check result.
func (m *Manager) processResult(result *checks.CheckResult) {
	// Record every check result through the Recorder (Prometheus + telemetry).
	m.recorder.RecordCheckResult(result)

	// Events to emit after releasing the lock.
	var events []Event

	m.mu.Lock()

	existing, hasActive := m.active[result.Name]

	switch result.Status {
	case checks.StatusHealthy:
		if hasActive && existing.Status != StatusResolved {
			events = m.resolveIncidentLocked(existing)
		}
	case checks.StatusFailed, checks.StatusDegraded:
		if hasActive {
			// Deduplicate: increment failure count on existing incident.
			existing.FailureCount++
			existing.UpdatedAt = time.Now()
			if err := m.persistIncident(existing); err != nil {
				m.logger.Errorw("failed to persist incident update", "id", existing.ID, "error", err)
			}

			events = append(events, Event{Incident: existing.Copy(), Type: EventUpdated})
			m.logger.Infow("incident updated",
				"id", existing.ID,
				"check", existing.CheckName,
				"failures", existing.FailureCount,
			)
		} else {
			// New incident.
			inc := m.createIncident(result)
			m.active[result.Name] = inc
			if err := m.persistIncident(inc); err != nil {
				m.logger.Errorw("failed to persist new incident", "id", inc.ID, "error", err)
			}
			m.recorder.RecordIncidentOpened(inc)

			events = append(events, Event{Incident: inc.Copy(), Type: EventOpened})
			m.logger.Infow("incident opened",
				"id", inc.ID,
				"check", inc.CheckName,
				"service", inc.Service,
			)
		}
	}

	m.mu.Unlock()

	// Emit events outside the lock to prevent deadlocks.
	for _, event := range events {
		m.emit(event)
	}
}

// createIncident builds a new Incident from a failed check result.
func (m *Manager) createIncident(result *checks.CheckResult) *Incident {
	seq := m.idCounter.Add(1)
	now := time.Now()
	return &Incident{
		ID:           fmt.Sprintf("inc-%d", seq),
		CheckName:    result.Name,
		Service:      result.Service,
		Status:       StatusOpen,
		FailureCount: 1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// resolveIncidentLocked transitions an incident to resolved.
// Must be called with m.mu held. Returns events to emit after releasing the lock.
func (m *Manager) resolveIncidentLocked(inc *Incident) []Event {
	now := time.Now()
	inc.Status = StatusResolved
	inc.UpdatedAt = now
	inc.ResolvedAt = &now
	if err := m.persistIncident(inc); err != nil {
		m.logger.Errorw("failed to persist resolved incident", "id", inc.ID, "error", err)
	}
	m.recorder.RecordIncidentResolved(inc, now.Sub(inc.CreatedAt).Seconds())

	// Remove from active map.
	delete(m.active, inc.CheckName)

	m.logger.Infow("incident resolved",
		"id", inc.ID,
		"check", inc.CheckName,
		"duration", now.Sub(inc.CreatedAt),
	)

	return []Event{{Incident: inc.Copy(), Type: EventResolved}}
}

// SetStatus updates an incident's status externally (e.g., from the repair engine).
func (m *Manager) SetStatus(checkName string, status Status) {
	var events []Event

	m.mu.Lock()

	inc, ok := m.active[checkName]
	if !ok {
		m.mu.Unlock()
		return
	}

	inc.Status = status
	inc.UpdatedAt = time.Now()
	if err := m.persistIncident(inc); err != nil {
		m.logger.Errorw("failed to persist incident status change", "id", inc.ID, "error", err)
	}

	var eventType EventType
	switch status {
	case StatusRepairing:
		eventType = EventRepairing
		events = append(events, Event{Incident: inc.Copy(), Type: eventType})
	case StatusFailed:
		eventType = EventFailed
		events = append(events, Event{Incident: inc.Copy(), Type: eventType})
	case StatusResolved:
		events = m.resolveIncidentLocked(inc)
	default:
		eventType = EventUpdated
		events = append(events, Event{Incident: inc.Copy(), Type: eventType})
	}

	m.mu.Unlock()

	// Emit events outside the lock to prevent deadlocks.
	for _, event := range events {
		m.emit(event)
	}
}

// GetActive returns a copy of the currently active incident for a check, if any.
func (m *Manager) GetActive(checkName string) (*Incident, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inc, ok := m.active[checkName]
	if !ok {
		return nil, false
	}
	// Return a copy to avoid data races.
	cp := *inc
	return &cp, true
}

// ActiveCount returns the number of currently active incidents.
func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.active)
}

func (m *Manager) persistIncident(inc *Incident) error {
	si := &storage.Incident{
		ID:           inc.ID,
		CheckName:    inc.CheckName,
		Service:      inc.Service,
		Status:       inc.Status,
		FailureCount: inc.FailureCount,
		CreatedAt:    inc.CreatedAt,
		UpdatedAt:    inc.UpdatedAt,
		ResolvedAt:   inc.ResolvedAt,
	}
	return m.store.SaveIncident(si)
}

func (m *Manager) emit(event Event) {
	select {
	case m.events <- event:
	default:
		m.logger.Errorw("incident event channel full, dropping event",
			"incident", event.Incident.ID,
			"type", event.Type,
		)
	}
}
