// Package runtime provides the Runtime supervisor that initializes,
// wires, and manages the lifecycle of all agent components.
package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/uptimy/uptimy-agent/internal/checks"
	"github.com/uptimy/uptimy-agent/internal/config"
	"github.com/uptimy/uptimy-agent/internal/incidents"
	"github.com/uptimy/uptimy-agent/internal/kubernetes"
	"github.com/uptimy/uptimy-agent/internal/logging"
	"github.com/uptimy/uptimy-agent/internal/plugins/actions"
	"github.com/uptimy/uptimy-agent/internal/plugins/checkers"
	"github.com/uptimy/uptimy-agent/internal/repair"
	"github.com/uptimy/uptimy-agent/internal/storage"
	"github.com/uptimy/uptimy-agent/internal/telemetry"
	"github.com/uptimy/uptimy-agent/pkg/client"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	k8sclient "k8s.io/client-go/kubernetes"
)

// Runtime is the top-level supervisor that coordinates all agent modules.
type Runtime struct {
	Config             *config.Config
	Logger             *zap.SugaredLogger
	Store              storage.Store
	CheckRegistry      *checks.Registry
	CheckScheduler     *checks.Scheduler
	IncidentManager    *incidents.Manager
	RepairEngine       *repair.Engine
	ActionRegistry     *repair.ActionRegistry
	Guardrails         *repair.Guardrails
	TelemetryClient    *telemetry.Client
	MetricsExporter    *telemetry.Exporter
	Metrics            *telemetry.Metrics
	KubeWatcher        *kubernetes.Watcher
	ControlPlaneClient *client.ControlPlaneClient

	// Internal channels connecting modules.
	checkResults   chan checks.CheckResult
	incidentEvents chan incidents.Event
}

// New creates a fully wired Runtime from the given Config.
func New(cfg *config.Config) (*Runtime, error) {
	// Logging
	logger, err := logging.NewLogger(cfg.Logging.Level, cfg.Logging.Format)
	if err != nil {
		return nil, fmt.Errorf("initializing logger: %w", err)
	}

	// Storage
	store, err := storage.NewBoltStore(cfg.Storage.Path)
	if err != nil {
		return nil, fmt.Errorf("initializing storage: %w", err)
	}

	// Metrics & Telemetry
	metrics := telemetry.NewMetrics()
	telClient := telemetry.NewClient(cfg.Telemetry.BufferSize, metrics, logger)
	exporter := telemetry.NewExporter(cfg.Telemetry.MetricsPort, metrics.Registry, logger)

	// Channels
	checkResults := make(chan checks.CheckResult, 256)
	incidentEvents := make(chan incidents.Event, 256)

	// Check engine
	checkRegistry := checks.NewRegistry()
	scheduler := checks.NewScheduler(checkRegistry, checkResults, cfg.Agent.WorkerPoolSize, logger)
	if err := checkers.BuildFromConfig(cfg.Checks, checkRegistry, scheduler, logger); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("building checks from config: %w", err)
	}

	// Incident manager
	incMgr := incidents.NewManager(store, incidentEvents, logger)

	// Rehydrate active incidents from storage so repairs resume after restart.
	if err := incMgr.Rehydrate(); err != nil {
		logger.Warnw("failed to rehydrate incidents, starting fresh", "error", err)
	}

	// Repair engine
	actionRegistry := repair.NewActionRegistry()
	guardrails := repair.NewGuardrails()

	// Detect Kubernetes availability and create a shared client.
	kubeAvailable := kubernetes.IsRunningInCluster()
	if cfg.Kubernetes.Enabled == config.KubeDisabled {
		kubeAvailable = false
	}

	var kubeClient k8sclient.Interface
	if kubeAvailable {
		kc, err := kubernetes.NewClient()
		if err != nil {
			logger.Warnw("failed to create kubernetes client, k8s actions will be disabled", "error", err)
		} else {
			kubeClient = kc
		}
	}

	// Register built-in actions.
	builtinActions := []repair.Action{
		actions.NewWaitAction(logger),
		actions.NewHealthcheckAction(checkRegistry, logger),
		actions.NewRestartPodAction(kubeClient, logger),
		actions.NewRestartContainerAction(logger),
		actions.NewRestartServiceAction(logger),
		actions.NewStartServiceAction(logger),
		actions.NewStopServiceAction(logger),
		actions.NewRollbackDeploymentAction(kubeClient, logger),
		actions.NewScaleReplicasAction(kubeClient, logger),
		actions.NewClearTempAction(logger),
		actions.NewRotateLogsAction(logger),
		actions.NewWebhookAction(logger),
	}

	for _, a := range builtinActions {
		if err := actionRegistry.Register(a); err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("registering action %q: %w", a.Name(), err)
		}
	}

	repairEngine := repair.NewEngine(actionRegistry, guardrails, store, incMgr, logger)
	repairEngine.LoadConfig(cfg.Repairs, cfg.Recipes)

	// Kubernetes watcher (optional)
	var kubeWatcher *kubernetes.Watcher
	if kubeClient != nil && cfg.Kubernetes.Enabled != config.KubeDisabled {
		kubeWatcher = kubernetes.NewWatcherWithClient(kubeClient, cfg.Kubernetes.Namespace, checkResults, logger)
	}

	// Control plane client (optional)
	var cpClient *client.ControlPlaneClient
	if cfg.ControlPlane.Enabled && cfg.ControlPlane.Endpoint != "" {
		cpClient = client.NewControlPlaneClient(cfg.ControlPlane.Endpoint, cfg.ControlPlane.Token, logger)
	}

	return &Runtime{
		Config:             cfg,
		Logger:             logger,
		Store:              store,
		CheckRegistry:      checkRegistry,
		CheckScheduler:     scheduler,
		IncidentManager:    incMgr,
		RepairEngine:       repairEngine,
		ActionRegistry:     actionRegistry,
		Guardrails:         guardrails,
		TelemetryClient:    telClient,
		MetricsExporter:    exporter,
		Metrics:            metrics,
		KubeWatcher:        kubeWatcher,
		ControlPlaneClient: cpClient,
		checkResults:       checkResults,
		incidentEvents:     incidentEvents,
	}, nil
}

// Start launches all agent goroutines. Blocks until ctx is canceled
// or a fatal error occurs.
func (r *Runtime) Start(ctx context.Context) error {
	r.Logger.Infow("starting uptimy agent",
		"name", r.Config.Agent.Name,
		"checks", len(r.Config.Checks),
		"repairs", len(r.Config.Repairs),
	)

	g, ctx := errgroup.WithContext(ctx)

	// Metrics exporter
	if r.Config.Telemetry.Enabled {
		g.Go(func() error {
			errCh := make(chan error, 1)
			go func() { errCh <- r.MetricsExporter.Start() }()
			select {
			case err := <-errCh:
				return err
			case <-ctx.Done():
				return r.MetricsExporter.Shutdown(context.Background())
			}
		})
	}

	// Check scheduler
	g.Go(func() error {
		r.CheckScheduler.Start(ctx)
		return nil
	})

	// Incident manager
	g.Go(func() error {
		r.IncidentManager.Run(ctx, r.checkResults)
		return nil
	})

	// Repair engine
	g.Go(func() error {
		r.RepairEngine.Run(ctx, r.incidentEvents)
		return nil
	})

	// Kubernetes watcher (optional)
	if r.KubeWatcher != nil {
		g.Go(func() error {
			r.KubeWatcher.Run(ctx)
			return nil
		})
	}

	// Control plane client (optional)
	if r.ControlPlaneClient != nil {
		g.Go(func() error {
			r.ControlPlaneClient.RunWithReconnect(ctx)
			return nil
		})
	}

	// Uptime counter
	g.Go(func() error {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				r.Metrics.AgentUptime.Inc()
			}
		}
	})

	return g.Wait()
}

// Shutdown gracefully stops all components.
func (r *Runtime) Shutdown(ctx context.Context) error {
	r.Logger.Infow("shutting down agent")

	// Stop scheduler first to prevent new check results.
	r.CheckScheduler.Stop()

	// Shutdown metrics exporter.
	if r.Config.Telemetry.Enabled {
		if err := r.MetricsExporter.Shutdown(ctx); err != nil {
			r.Logger.Warnw("metrics exporter shutdown error", "error", err)
		}
	}

	// Disconnect control plane client.
	if r.ControlPlaneClient != nil {
		if err := r.ControlPlaneClient.Disconnect(); err != nil {
			r.Logger.Warnw("control plane client disconnect error", "error", err)
		}
	}

	// Close storage last.
	if err := r.Store.Close(); err != nil {
		r.Logger.Warnw("storage close error", "error", err)
	}

	r.Logger.Infow("agent shutdown complete")
	return nil
}
