package checks_test

import (
	"context"
	"testing"
	"time"

	"github.com/uptimy/uptimy-agent/internal/checks"
	"github.com/uptimy/uptimy-agent/internal/logging"
)

// mockCheck is a test check that returns a configurable result.
type mockCheck struct {
	name   string
	result checks.CheckResult
}

func (m *mockCheck) Name() string                             { return m.name }
func (m *mockCheck) Run(_ context.Context) checks.CheckResult { return m.result }

func TestScheduler_RunsChecksAndDeliversResults(t *testing.T) {
	registry := checks.NewRegistry()
	results := make(chan checks.CheckResult, 10)
	logger := logging.Nop()

	scheduler := checks.NewScheduler(registry, results, 2, logger)

	mc := &mockCheck{
		name: "test-check",
		result: checks.CheckResult{
			Name:      "test-check",
			Service:   "test",
			Status:    checks.StatusHealthy,
			Timestamp: time.Now(),
		},
	}

	registry.Register(mc)
	scheduler.AddCheck(mc, 100*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go scheduler.Start(ctx)

	// Should receive at least one result.
	select {
	case result := <-results:
		if result.Name != "test-check" {
			t.Errorf("expected check name test-check, got %s", result.Name)
		}
		if result.Status != checks.StatusHealthy {
			t.Errorf("expected healthy, got %s", result.Status)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for check result")
	}

	cancel()
	scheduler.Stop()
}

func TestScheduler_MultipleChecks(t *testing.T) {
	registry := checks.NewRegistry()
	results := make(chan checks.CheckResult, 20)
	logger := logging.Nop()

	scheduler := checks.NewScheduler(registry, results, 4, logger)

	for i := 0; i < 3; i++ {
		name := "check-" + string(rune('a'+i))
		mc := &mockCheck{
			name: name,
			result: checks.CheckResult{
				Name:      name,
				Status:    checks.StatusHealthy,
				Timestamp: time.Now(),
			},
		}
		registry.Register(mc)
		scheduler.AddCheck(mc, 100*time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go scheduler.Start(ctx)

	// Collect results.
	seen := make(map[string]bool)
	timer := time.NewTimer(400 * time.Millisecond)
	defer timer.Stop()

	for {
		select {
		case result := <-results:
			seen[result.Name] = true
		case <-timer.C:
			goto done
		}
	}
done:

	if len(seen) < 3 {
		t.Errorf("expected results from 3 checks, got %d: %v", len(seen), seen)
	}

	cancel()
	scheduler.Stop()
}
