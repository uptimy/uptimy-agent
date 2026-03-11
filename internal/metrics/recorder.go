// Package metrics provides convenience wrappers around telemetry.Metrics
// for recording check results, repair outcomes, and incident lifecycle.
package metrics

import (
	"github.com/uptimy/uptimy-agent/internal/checks"
	"github.com/uptimy/uptimy-agent/internal/telemetry"
)

// Recorder wraps telemetry.Metrics with high-level recording methods.
type Recorder struct {
	m *telemetry.Metrics
}

// NewRecorder creates a Recorder for the given Metrics.
func NewRecorder(m *telemetry.Metrics) *Recorder {
	return &Recorder{m: m}
}

// RecordCheckResult increments check counters.
func (r *Recorder) RecordCheckResult(result *checks.CheckResult) {
	r.m.ChecksTotal.Inc()
	if result.Status == checks.StatusFailed {
		r.m.ChecksFailed.Inc()
	}
}

// RecordRepairAttempt increments the repairs attempted counter.
func (r *Recorder) RecordRepairAttempt() {
	r.m.RepairsAttempted.Inc()
}

// RecordRepairSuccess increments the repairs success counter.
func (r *Recorder) RecordRepairSuccess() {
	r.m.RepairsSuccess.Inc()
}

// RecordRepairFailure is a no-op hook for future failure-specific metrics.
func (r *Recorder) RecordRepairFailure() {}

// RecordRepairDuration observes a repair duration in seconds.
func (r *Recorder) RecordRepairDuration(seconds float64) {
	r.m.RepairDuration.Observe(seconds)
}

// RecordIncident increments total incidents and active gauge.
func (r *Recorder) RecordIncident() {
	r.m.IncidentsTotal.Inc()
	r.m.ActiveIncidents.Inc()
}

// RecordIncidentResolved decrements active gauge and observes resolution time.
func (r *Recorder) RecordIncidentResolved(resolutionSeconds float64) {
	r.m.ActiveIncidents.Dec()
	r.m.IncidentResolution.Observe(resolutionSeconds)
}

// IncrementUptime increments the agent uptime counter by one second.
func (r *Recorder) IncrementUptime() {
	r.m.AgentUptime.Inc()
}
