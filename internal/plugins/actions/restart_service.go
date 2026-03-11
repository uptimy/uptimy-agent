package actions

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"go.uber.org/zap"
)

// RestartServiceAction restarts a systemd service via systemctl.
type RestartServiceAction struct {
	logger *zap.SugaredLogger
}

func NewRestartServiceAction(logger *zap.SugaredLogger) *RestartServiceAction {
	return &RestartServiceAction{logger: logger}
}

func (a *RestartServiceAction) Name() string { return "restart_service" }

func (a *RestartServiceAction) Execute(ctx context.Context, params map[string]string) error {
	service := params["service"]
	if service == "" {
		return fmt.Errorf("restart_service: 'service' parameter is required")
	}

	// Validate the service name to prevent command injection.
	if err := validateServiceName(service); err != nil {
		return fmt.Errorf("restart_service: %w", err)
	}

	a.logger.Infow("restarting systemd service", "service", service)

	cmd := exec.CommandContext(ctx, "systemctl", "restart", service)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restart_service: systemctl restart %s failed: %w (output: %s)",
			service, err, strings.TrimSpace(string(output)))
	}

	// Verify the service is active after restart.
	verifyCmd := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", service)
	if err := verifyCmd.Run(); err != nil {
		return fmt.Errorf("restart_service: service %s did not become active after restart", service)
	}

	a.logger.Infow("service restarted and active", "service", service)
	return nil
}

// validateServiceName rejects suspicious service names.
// systemd unit names may contain [a-zA-Z0-9:._@-].
func validateServiceName(name string) error {
	for _, r := range name {
		if !isValidServiceRune(r) {
			return fmt.Errorf("invalid character %q in service name", r)
		}
	}
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid service name %q", name)
	}
	return nil
}

func isValidServiceRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_' || r == '-' || r == '.' || r == ':' || r == '@'
}
