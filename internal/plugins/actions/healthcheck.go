package actions

import (
	"context"
	"fmt"

	"github.com/uptimy/uptimy-agent/internal/checks"
	"go.uber.org/zap"
)

// HealthcheckAction runs a named health check as a repair verification step.
type HealthcheckAction struct {
	registry *checks.Registry
	logger   *zap.SugaredLogger
}

func NewHealthcheckAction(registry *checks.Registry, logger *zap.SugaredLogger) *HealthcheckAction {
	return &HealthcheckAction{registry: registry, logger: logger}
}

func (a *HealthcheckAction) Name() string { return "healthcheck" }

func (a *HealthcheckAction) Execute(ctx context.Context, params map[string]string) error {
	checkName, ok := params["check"]
	if !ok {
		return fmt.Errorf("healthcheck action requires 'check' parameter")
	}

	check, found := a.registry.Get(checkName)
	if !found {
		return fmt.Errorf("check %q not found in registry", checkName)
	}

	a.logger.Infow("running verification healthcheck", "check", checkName)

	result := check.Run(ctx)
	if result.Status != checks.StatusHealthy {
		return fmt.Errorf("healthcheck %q returned status %s: %v", checkName, result.Status, result.Error)
	}

	a.logger.Infow("verification healthcheck passed", "check", checkName)
	return nil
}
