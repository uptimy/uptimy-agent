package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"go.uber.org/zap"
)

// StartContainerAction starts a stopped Docker container using the Docker Engine SDK.
type StartContainerAction struct {
	logger *zap.SugaredLogger
}

// NewStartContainerAction creates a StartContainerAction.
func NewStartContainerAction(logger *zap.SugaredLogger) *StartContainerAction {
	return &StartContainerAction{logger: logger}
}

// Name returns the action name.
func (a *StartContainerAction) Name() string { return "start_container" }

// Execute runs the start container action.
func (a *StartContainerAction) Execute(ctx context.Context, params map[string]string) error {
	containerName := params["container"]
	if containerName == "" {
		return fmt.Errorf("start_container: 'container' parameter is required")
	}

	a.logger.Infow("starting docker container", "container", containerName)

	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("start_container: failed to create docker client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	if startErr := cli.ContainerStart(ctx, containerName, container.StartOptions{}); startErr != nil {
		return fmt.Errorf("start_container: failed to start container %s: %w", containerName, startErr)
	}

	// Verify the container is running.
	info, err := cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return fmt.Errorf("start_container: failed to inspect container %s after start: %w", containerName, err)
	}

	if info.State == nil || info.State.Status != container.StateRunning {
		status := "unknown"
		if info.State != nil {
			status = info.State.Status
		}
		return fmt.Errorf("start_container: container %s did not become running (status: %s)", containerName, status)
	}

	a.logger.Infow("container started successfully", "container", containerName, "started_at", time.Now().Format(time.RFC3339))
	return nil
}
