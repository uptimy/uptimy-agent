package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"go.uber.org/zap"
)

// RestartContainerAction restarts a Docker container using the Docker Engine SDK.
type RestartContainerAction struct {
	logger *zap.SugaredLogger
}

// NewRestartContainerAction creates a RestartContainerAction.
func NewRestartContainerAction(logger *zap.SugaredLogger) *RestartContainerAction {
	return &RestartContainerAction{logger: logger}
}

// Name returns the action name.
func (a *RestartContainerAction) Name() string { return "restart_container" }

// Execute runs the restart container action.
func (a *RestartContainerAction) Execute(ctx context.Context, params map[string]string) error {
	containerName := params["container"]
	if containerName == "" {
		return fmt.Errorf("restart_container: 'container' parameter is required")
	}

	var stopTimeout *int
	if v, ok := params["timeout"]; ok {
		if d, err := time.ParseDuration(v); err == nil {
			secs := int(d.Seconds())
			stopTimeout = &secs
		}
	}

	a.logger.Infow("restarting docker container", "container", containerName)

	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("restart_container: failed to create docker client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	opts := container.StopOptions{Timeout: stopTimeout}
	if restartErr := cli.ContainerRestart(ctx, containerName, opts); restartErr != nil {
		return fmt.Errorf("restart_container: failed to restart container %s: %w", containerName, err)
	}

	// Verify the container is running after restart.
	info, err := cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return fmt.Errorf("restart_container: failed to inspect container %s after restart: %w", containerName, err)
	}

	if info.State == nil || info.State.Status != container.StateRunning {
		status := "unknown"
		if info.State != nil {
			status = info.State.Status
		}
		return fmt.Errorf("restart_container: container %s did not become running after restart (status: %s)", containerName, status)
	}

	a.logger.Infow("container restarted successfully", "container", containerName)
	return nil
}
