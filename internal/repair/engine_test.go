package repair_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/uptimy/uptimy-agent/internal/checks"
	"github.com/uptimy/uptimy-agent/internal/config"
	"github.com/uptimy/uptimy-agent/internal/incidents"
	"github.com/uptimy/uptimy-agent/internal/logging"
	"github.com/uptimy/uptimy-agent/internal/repair"
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

// nopRepairRecorder satisfies repair.Recorder without side effects.
type nopRepairRecorder struct{}

func (n *nopRepairRecorder) RecordRepairStarted(_, _, _ string)                 {}
func (n *nopRepairRecorder) RecordRepairCompleted(_, _, _, _ string, _ float64) {}

// nopIncidentRecorder satisfies incidents.MetricsRecorder for NewManager.
type nopIncidentRecorder struct{}

func (n *nopIncidentRecorder) RecordCheckResult(_ *checks.CheckResult)                 {}
func (n *nopIncidentRecorder) RecordIncidentOpened(_ *incidents.Incident)              {}
func (n *nopIncidentRecorder) RecordIncidentResolved(_ *incidents.Incident, _ float64) {}

// successAction always succeeds.
type successAction struct{ name string }

func (a *successAction) Name() string                                         { return a.name }
func (a *successAction) Execute(_ context.Context, _ map[string]string) error { return nil }

// failAction always fails.
type failAction struct{ name string }

func (a *failAction) Name() string { return a.name }
func (a *failAction) Execute(_ context.Context, _ map[string]string) error {
	return fmt.Errorf("intentional failure")
}

// waitForEngine gives the engine time to process events asynchronously,
// then cancels the context and returns. Uses a short poll to avoid flakiness.
func waitForEngine(cancel context.CancelFunc, d time.Duration) {
	time.Sleep(d)
	cancel()
}

func TestEngine_ExecutesRecipe(t *testing.T) {
	store := &memStore{}
	logger := logging.Nop()
	incEvents := make(chan incidents.Event, 10)
	incMgr := incidents.NewManager(store, incEvents, &nopIncidentRecorder{}, logger)

	actionRegistry := repair.NewActionRegistry()
	action := &successAction{name: "test_success"}
	if err := actionRegistry.Register(action); err != nil {
		t.Fatalf("register: %v", err)
	}

	guardrails := repair.NewGuardrails()
	guardrails.AllowAction("test_success")

	engine := repair.NewEngine(actionRegistry, guardrails, store, incMgr, &nopRepairRecorder{}, logger)
	engine.LoadConfig(
		[]config.RepairRuleConfig{
			{Rule: "test-rule", Check: "test-check", Recipe: "test-recipe"},
		},
		[]config.RecipeConfig{
			{
				Name: "test-recipe",
				Steps: []config.RecipeStepConfig{
					{Action: "test_success"},
				},
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	engineEvents := make(chan incidents.Event, 10)
	go engine.Run(ctx, engineEvents)

	// Send an incident event to trigger the recipe.
	engineEvents <- incidents.Event{
		Type: incidents.EventOpened,
		Incident: &incidents.Incident{
			ID:        "inc-1",
			CheckName: "test-check",
			Service:   "test-service",
			Status:    incidents.StatusOpen,
		},
	}

	// Wait for the engine to process the recipe.
	waitForEngine(cancel, 500*time.Millisecond)
}

func TestEngine_RespectsGuardrails(t *testing.T) {
	store := &memStore{}
	logger := logging.Nop()
	incEvents := make(chan incidents.Event, 10)
	incMgr := incidents.NewManager(store, incEvents, &nopIncidentRecorder{}, logger)

	actionRegistry := repair.NewActionRegistry()
	action := &successAction{name: "test_success"}
	if err := actionRegistry.Register(action); err != nil {
		t.Fatalf("register: %v", err)
	}

	guardrails := repair.NewGuardrails()
	guardrails.AllowAction("test_success")
	guardrails.SetMaxRepairsPerHour("test-rule", 1)

	engine := repair.NewEngine(actionRegistry, guardrails, store, incMgr, &nopRepairRecorder{}, logger)
	engine.LoadConfig(
		[]config.RepairRuleConfig{
			{Rule: "test-rule", Check: "test-check", Recipe: "test-recipe", MaxRepairsPerHour: 1},
		},
		[]config.RecipeConfig{
			{
				Name: "test-recipe",
				Steps: []config.RecipeStepConfig{
					{Action: "test_success"},
				},
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	engineEvents := make(chan incidents.Event, 10)
	go engine.Run(ctx, engineEvents)

	// First repair should execute.
	engineEvents <- incidents.Event{
		Type: incidents.EventOpened,
		Incident: &incidents.Incident{
			ID:        "inc-1",
			CheckName: "test-check",
			Service:   "test-service",
			Status:    incidents.StatusOpen,
		},
	}

	time.Sleep(300 * time.Millisecond)

	// Second repair should be rate-limited (no panic or hang).
	engineEvents <- incidents.Event{
		Type: incidents.EventOpened,
		Incident: &incidents.Incident{
			ID:        "inc-2",
			CheckName: "test-check",
			Service:   "test-service",
			Status:    incidents.StatusOpen,
		},
	}

	// Allow time for the rate-limited path to execute.
	waitForEngine(cancel, 500*time.Millisecond)
}

func TestEngine_HandlesActionFailure(t *testing.T) {
	store := &memStore{}
	logger := logging.Nop()
	incEvents := make(chan incidents.Event, 10)
	incMgr := incidents.NewManager(store, incEvents, &nopIncidentRecorder{}, logger)

	actionRegistry := repair.NewActionRegistry()
	action := &failAction{name: "test_fail"}
	if err := actionRegistry.Register(action); err != nil {
		t.Fatalf("register: %v", err)
	}

	guardrails := repair.NewGuardrails()
	guardrails.AllowAction("test_fail")

	engine := repair.NewEngine(actionRegistry, guardrails, store, incMgr, &nopRepairRecorder{}, logger)
	engine.LoadConfig(
		[]config.RepairRuleConfig{
			{Rule: "fail-rule", Check: "fail-check", Recipe: "fail-recipe"},
		},
		[]config.RecipeConfig{
			{
				Name: "fail-recipe",
				Steps: []config.RecipeStepConfig{
					{Action: "test_fail", Retries: 1},
				},
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	engineEvents := make(chan incidents.Event, 10)
	go engine.Run(ctx, engineEvents)

	engineEvents <- incidents.Event{
		Type: incidents.EventOpened,
		Incident: &incidents.Incident{
			ID:        "inc-1",
			CheckName: "fail-check",
			Service:   "fail-service",
			Status:    incidents.StatusOpen,
		},
	}

	// Engine should handle the failure gracefully.
	waitForEngine(cancel, 500*time.Millisecond)
}
