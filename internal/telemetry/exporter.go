package telemetry

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Exporter serves the Prometheus /metrics endpoint.
type Exporter struct {
	server *http.Server
	logger *zap.SugaredLogger
}

// NewExporter creates a metrics HTTP server on the given port.
// It uses the provided registry instead of the global default so metrics
// registration is isolated and safe for concurrent use.
func NewExporter(port int, registry *prometheus.Registry, logger *zap.SugaredLogger) *Exporter {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	return &Exporter{
		server: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
		logger: logger,
	}
}

// Start begins serving metrics. Blocks until the server stops.
func (e *Exporter) Start() error {
	e.logger.Infow("starting metrics exporter", "addr", e.server.Addr)
	if err := e.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("metrics server: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the metrics server.
func (e *Exporter) Shutdown(ctx context.Context) error {
	return e.server.Shutdown(ctx)
}
