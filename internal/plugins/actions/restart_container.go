package actions

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"go.uber.org/zap"
)

// RestartContainerAction restarts a Docker container via the Docker CLI.
type RestartContainerAction struct {
	logger *zap.SugaredLogger
}

func NewRestartContainerAction(logger *zap.SugaredLogger) *RestartContainerAction {
	return &RestartContainerAction{logger: logger}
}

func (a *RestartContainerAction) Name() string { return "restart_container" }

func (a *RestartContainerAction) Execute(ctx context.Context, params map[string]string) error {
	container := params["container"]
	if container == "" {
		return fmt.Errorf("restart_container: 'container' parameter is required")
	}

	// Validate the container name/ID to prevent command injection.
	if err := validateContainerName(container); err != nil {
		return fmt.Errorf("restart_container: %w", err)
	}

	timeout := "30"
	if v, ok := params["timeout"]; ok {
		// Accept a Go duration string ("30s") or plain seconds ("30").
		if d, err := time.ParseDuration(v); err == nil {
			timeout = fmt.Sprintf("%d", int(d.Seconds()))
		} else {
			timeout = v
		}
	}

	a.logger.Infow("restarting docker container", "container", container, "timeout", timeout)

	cmd := exec.CommandContext(ctx, "docker", "restart", "--time", timeout, container)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restart_container: docker restart %s failed: %w (output: %s)",
			container, err, strings.TrimSpace(string(output)))
	}

	a.logger.Infow("container restarted successfully", "container", container)
	return nil
}

// validateContainerName rejects suspicious container names to avoid
// shell-injection-style attacks through crafted parameter values.
func validateContainerName(name string) error {
	for _, r := range name {
		if !isValidContainerRune(r) {
			return fmt.Errorf("invalid character %q in container name", r)
		}
	}
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid container name %q", name)
	}
	return nil
}

func isValidContainerRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_' || r == '-' || r == '.'
}
