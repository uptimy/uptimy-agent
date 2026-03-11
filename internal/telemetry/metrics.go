// Package telemetry provides Prometheus metrics, ring-buffered event
// collection, and an HTTP exporter for the Uptimy Agent.
package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics holds all Prometheus metric instruments for the agent.
type Metrics struct {
	Registry           *prometheus.Registry
	AgentUptime        prometheus.Counter
	ChecksTotal        prometheus.Counter
	ChecksFailed       prometheus.Counter
	RepairsAttempted   prometheus.Counter
	RepairsSuccess     prometheus.Counter
	IncidentsTotal     prometheus.Counter
	RepairDuration     prometheus.Histogram
	IncidentResolution prometheus.Histogram
	ActiveIncidents    prometheus.Gauge
}

// NewMetrics creates and registers all agent metrics on a dedicated registry.
// Safe to call multiple times (e.g., in parallel tests) because each call
// creates its own registry instead of using the global default.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	// Include standard Go and process collectors.
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	agentUptime := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "uptimy_agent_uptime_seconds_total",
		Help: "Total seconds the agent has been running.",
	})
	checksTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "uptimy_checks_total",
		Help: "Total number of health checks executed.",
	})
	checksFailed := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "uptimy_checks_failed_total",
		Help: "Total number of failed health checks.",
	})
	repairsAttempted := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "uptimy_repairs_attempted_total",
		Help: "Total number of repair recipes attempted.",
	})
	repairsSuccess := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "uptimy_repairs_success_total",
		Help: "Total number of successfully completed repair recipes.",
	})
	incidentsTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "uptimy_incidents_total",
		Help: "Total number of incidents created.",
	})
	repairDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "uptimy_repair_duration_seconds",
		Help:    "Duration of repair recipe executions in seconds.",
		Buckets: prometheus.ExponentialBuckets(1, 2, 10),
	})
	incidentResolution := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "uptimy_incident_resolution_seconds",
		Help:    "Time from incident creation to resolution in seconds.",
		Buckets: prometheus.ExponentialBuckets(5, 2, 12),
	})
	activeIncidents := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "uptimy_active_incidents",
		Help: "Number of currently active (unresolved) incidents.",
	})

	reg.MustRegister(agentUptime, checksTotal, checksFailed,
		repairsAttempted, repairsSuccess, incidentsTotal,
		repairDuration, incidentResolution, activeIncidents)

	return &Metrics{
		Registry:           reg,
		AgentUptime:        agentUptime,
		ChecksTotal:        checksTotal,
		ChecksFailed:       checksFailed,
		RepairsAttempted:   repairsAttempted,
		RepairsSuccess:     repairsSuccess,
		IncidentsTotal:     incidentsTotal,
		RepairDuration:     repairDuration,
		IncidentResolution: incidentResolution,
		ActiveIncidents:    activeIncidents,
	}
}
