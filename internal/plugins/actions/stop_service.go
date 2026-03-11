package actions

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"go.uber.org/zap"
)

// StopServiceAction stops a systemd service via systemctl.
type StopServiceAction struct {
	logger *zap.SugaredLogger
}

func NewStopServiceAction(logger *zap.SugaredLogger) *StopServiceAction {
	return &StopServiceAction{logger: logger}
}

func (a *StopServiceAction) Name() string { return "stop_service" }

func (a *StopServiceAction) Execute(ctx context.Context, params map[string]string) error {
	service := params["service"]
	if service == "" {
		return fmt.Errorf("stop_service: 'service' parameter is required")
	}

	// Validate the service name to prevent command injection.
	if err := validateServiceName(service); err != nil {
		return fmt.Errorf("stop_service: %w", err)
	}

	a.logger.Infow("stopping systemd service", "service", service)

	cmd := exec.CommandContext(ctx, "systemctl", "stop", service)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("stop_service: systemctl stop %s failed: %w (output: %s)",
			service, err, strings.TrimSpace(string(output)))
	}

	// Verify the service is inactive after stop.
	verifyCmd := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", service)
	if verifyCmd.Run() == nil {
		return fmt.Errorf("stop_service: service %s is still active after stop", service)
	}

	a.logger.Infow("service stopped successfully", "service", service)
	return nil
}
