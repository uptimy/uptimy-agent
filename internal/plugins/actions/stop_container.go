package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"go.uber.org/zap"
)

// StopContainerAction stops a running Docker container using the Docker Engine SDK.
type StopContainerAction struct {
	logger *zap.SugaredLogger
}

// NewStopContainerAction creates a StopContainerAction.
func NewStopContainerAction(logger *zap.SugaredLogger) *StopContainerAction {
	return &StopContainerAction{logger: logger}
}

// Name returns the action name.
func (a *StopContainerAction) Name() string { return "stop_container" }

// Execute runs the stop container action.
func (a *StopContainerAction) Execute(ctx context.Context, params map[string]string) error {
	containerName := params["container"]
	if containerName == "" {
		return fmt.Errorf("stop_container: 'container' parameter is required")
	}

	var stopTimeout *int
	if v, ok := params["timeout"]; ok {
		if d, err := time.ParseDuration(v); err == nil {
			secs := int(d.Seconds())
			stopTimeout = &secs
		}
	}

	a.logger.Infow("stopping docker container", "container", containerName)

	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("stop_container: failed to create docker client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	opts := container.StopOptions{Timeout: stopTimeout}
	if stopErr := cli.ContainerStop(ctx, containerName, opts); stopErr != nil {
		return fmt.Errorf("stop_container: failed to stop container %s: %w", containerName, stopErr)
	}

	// Verify the container is no longer running.
	info, err := cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return fmt.Errorf("stop_container: failed to inspect container %s after stop: %w", containerName, err)
	}

	if info.State != nil && info.State.Running {
		return fmt.Errorf("stop_container: container %s is still running after stop", containerName)
	}

	a.logger.Infow("container stopped successfully", "container", containerName)
	return nil
}
