package repair_test

import (
	"testing"
	"time"

	"github.com/uptimy/uptimy-agent/internal/repair"
)

func TestGuardrails_AllowedAction(t *testing.T) {
	g := repair.NewGuardrails()

	if err := g.CheckAction("restart_pod"); err != nil {
		t.Errorf("restart_pod should be allowed: %v", err)
	}
	if err := g.CheckAction("wait"); err != nil {
		t.Errorf("wait should be allowed: %v", err)
	}
}

func TestGuardrails_ForbiddenAction(t *testing.T) {
	g := repair.NewGuardrails()

	if err := g.CheckAction("shell_exec"); err == nil {
		t.Error("shell_exec should be forbidden")
	}
	if err := g.CheckAction("delete_files"); err == nil {
		t.Error("delete_files should be forbidden")
	}
}

func TestGuardrails_UnknownAction(t *testing.T) {
	g := repair.NewGuardrails()

	if err := g.CheckAction("some_random_action"); err == nil {
		t.Error("unknown actions should not be allowed by default")
	}
}

func TestGuardrails_AllowCustom(t *testing.T) {
	g := repair.NewGuardrails()
	g.AllowAction("custom_action")

	if err := g.CheckAction("custom_action"); err != nil {
		t.Errorf("custom_action should now be allowed: %v", err)
	}
}

func TestGuardrails_RateLimit(t *testing.T) {
	g := repair.NewGuardrails()
	g.SetMaxRepairsPerHour("test-rule", 2)

	// First two should succeed.
	if err := g.CheckRateLimit("test-rule"); err != nil {
		t.Fatalf("first repair should pass rate limit: %v", err)
	}
	g.RecordRepair("test-rule")

	if err := g.CheckRateLimit("test-rule"); err != nil {
		t.Fatalf("second repair should pass rate limit: %v", err)
	}
	g.RecordRepair("test-rule")

	// Third should be rejected.
	if err := g.CheckRateLimit("test-rule"); err == nil {
		t.Error("third repair should be rate-limited")
	}
}

func TestGuardrails_Cooldown(t *testing.T) {
	g := repair.NewGuardrails()
	g.SetCooldown("restart_pod", 5*time.Second)

	// First execution should succeed (no prior execution recorded).
	if err := g.CheckAction("restart_pod"); err != nil {
		t.Fatalf("first execution should pass cooldown: %v", err)
	}
	g.RecordActionExecution("restart_pod")

	// Immediate second execution should be rejected due to cooldown.
	if err := g.CheckAction("restart_pod"); err == nil {
		t.Error("immediate re-execution should be on cooldown")
	}
}
