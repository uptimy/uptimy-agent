package actions

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"go.uber.org/zap"
)

// StartServiceAction starts a systemd service via systemctl.
type StartServiceAction struct {
	logger *zap.SugaredLogger
}

func NewStartServiceAction(logger *zap.SugaredLogger) *StartServiceAction {
	return &StartServiceAction{logger: logger}
}

func (a *StartServiceAction) Name() string { return "start_service" }

func (a *StartServiceAction) Execute(ctx context.Context, params map[string]string) error {
	service := params["service"]
	if service == "" {
		return fmt.Errorf("start_service: 'service' parameter is required")
	}

	// Validate the service name to prevent command injection.
	if err := validateServiceName(service); err != nil {
		return fmt.Errorf("start_service: %w", err)
	}

	a.logger.Infow("starting systemd service", "service", service)

	cmd := exec.CommandContext(ctx, "systemctl", "start", service)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("start_service: systemctl start %s failed: %w (output: %s)",
			service, err, strings.TrimSpace(string(output)))
	}

	// Verify the service is active after start.
	verifyCmd := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", service)
	if err := verifyCmd.Run(); err != nil {
		return fmt.Errorf("start_service: service %s did not become active after start", service)
	}

	a.logger.Infow("service started and active", "service", service)
	return nil
}
