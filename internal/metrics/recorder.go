// Package metrics provides the Recorder — the single observability facade
// used by domain modules to record Prometheus metrics and telemetry events.
package metrics

import (
	"fmt"
	"time"

	"github.com/uptimy/uptimy-agent/internal/checks"
	"github.com/uptimy/uptimy-agent/internal/incidents"
	"github.com/uptimy/uptimy-agent/internal/telemetry"
)

// Reporter sends incident and repair reports to the control plane.
// Implementations must be safe for concurrent use.
type Reporter interface {
	ReportIncident(id, checkName, service, status string, failureCount int, eventCreatedAt time.Time)
	ReportRepair(incidentID, rule, recipe, status string, eventCreatedAt time.Time, duration time.Duration)
}

// Recorder is the single instrumentation interface for the agent.
// It writes to both Prometheus metrics and the telemetry event buffer,
// ensuring consistent observability across all domain modules.
type Recorder struct {
	m        *telemetry.Metrics
	buffer   *telemetry.RingBuffer
	reporter Reporter
}

// NewRecorder creates a Recorder backed by Prometheus metrics and a
// telemetry event buffer.
func NewRecorder(m *telemetry.Metrics, buffer *telemetry.RingBuffer) *Recorder {
	return &Recorder{
		m:      m,
		buffer: buffer,
	}
}

// SetReporter sets the optional control plane reporter.
// Must be called before the agent starts processing events.
func (r *Recorder) SetReporter(reporter Reporter) {
	r.reporter = reporter
}

func (r *Recorder) emit(eventType string, data map[string]string) {
	r.buffer.Push(telemetry.Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	})
}

// RecordCheckResult increments check counters and emits a telemetry event.
func (r *Recorder) RecordCheckResult(result *checks.CheckResult) {
	r.m.ChecksTotal.Inc()
	if result.Status == checks.StatusFailed {
		r.m.ChecksFailed.Inc()
	}

	r.emit("check_result", map[string]string{
		"check":    result.Name,
		"service":  result.Service,
		"status":   string(result.Status),
		"duration": fmt.Sprintf("%f", result.Duration.Seconds()),
	})
}

// RecordIncidentOpened increments total incidents and active gauge,
// and emits a telemetry event.
func (r *Recorder) RecordIncidentOpened(inc *incidents.Incident) {
	r.m.IncidentsTotal.Inc()
	r.m.ActiveIncidents.Inc()

	r.emit("incident_created", map[string]string{
		"id":      inc.ID,
		"check":   inc.CheckName,
		"service": inc.Service,
	})

	if r.reporter != nil {
		r.reporter.ReportIncident(
			inc.ID,
			inc.CheckName,
			inc.Service,
			inc.Status,
			inc.FailureCount,
			time.Now(),
		)
	}
}

// RecordIncidentResolved decrements the active gauge, observes resolution
// time, and emits a telemetry event.
func (r *Recorder) RecordIncidentResolved(inc *incidents.Incident, resolutionSeconds float64) {
	r.m.ActiveIncidents.Dec()
	r.m.IncidentResolution.Observe(resolutionSeconds)

	r.emit("incident_resolved", map[string]string{
		"id":                 inc.ID,
		"check":              inc.CheckName,
		"service":            inc.Service,
		"resolution_seconds": fmt.Sprintf("%f", resolutionSeconds),
	})

	if r.reporter != nil {
		r.reporter.ReportIncident(
			inc.ID,
			inc.CheckName,
			inc.Service,
			inc.Status,
			inc.FailureCount,
			time.Now(),
		)
	}
}

// RecordRepairStarted increments the repairs attempted counter and emits
// a telemetry event. It also tracks the repair start time for correlation
// with RecordRepairCompleted.
func (r *Recorder) RecordRepairStarted(incidentID, rule, recipe string) {
	r.m.RepairsAttempted.Inc()

	r.emit("repair_started", map[string]string{
		"incident": incidentID,
		"rule":     rule,
		"recipe":   recipe,
	})

	if r.reporter != nil {
		r.reporter.ReportRepair(
			incidentID,
			rule,
			recipe,
			"started",
			time.Now(),
			0,
		)
	}
}

// RecordRepairCompleted records a repair outcome (success or failure),
// observes the duration, and emits a telemetry event.
func (r *Recorder) RecordRepairCompleted(incidentID, rule, recipe, status string, durationSeconds float64) {
	r.m.RepairDuration.Observe(durationSeconds)

	if status == "success" {
		r.m.RepairsSuccess.Inc()
	}

	r.emit("repair_completed", map[string]string{
		"incident": incidentID,
		"rule":     rule,
		"recipe":   recipe,
		"status":   status,
		"duration": fmt.Sprintf("%f", durationSeconds),
	})

	if r.reporter != nil {
		r.reporter.ReportRepair(
			incidentID,
			rule,
			recipe,
			status,
			time.Now(),
			time.Duration(durationSeconds),
		)
	}
}

// IncrementUptime increments the agent uptime counter by one second.
func (r *Recorder) IncrementUptime() {
	r.m.AgentUptime.Inc()
}
