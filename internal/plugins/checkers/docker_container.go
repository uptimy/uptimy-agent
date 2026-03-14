package checkers

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/uptimy/uptimy-agent/internal/checks"
)

// DockerContainerCheck verifies that a Docker container is running
// using the Docker Engine SDK.
type DockerContainerCheck struct {
	name          string
	service       string
	containerName string
	timeout       time.Duration
}

// NewDockerContainerCheck creates a new Docker container health check.
func NewDockerContainerCheck(name, service, containerName string, timeout time.Duration) *DockerContainerCheck {
	return &DockerContainerCheck{
		name:          name,
		service:       service,
		containerName: containerName,
		timeout:       timeout,
	}
}

// Name returns the check's unique identifier.
func (c *DockerContainerCheck) Name() string { return c.name }

// Run executes the Docker container check and returns the result.
func (c *DockerContainerCheck) Run(ctx context.Context) checks.CheckResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	metadata := map[string]string{"container_name": c.containerName}

	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return checks.CheckResult{
			Name:      c.name,
			Service:   c.service,
			Status:    checks.StatusFailed,
			Error:     fmt.Errorf("failed to create docker client: %w", err),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
			Metadata:  metadata,
		}
	}
	defer func() { _ = cli.Close() }()

	info, err := cli.ContainerInspect(ctx, c.containerName)
	if err != nil {
		return checks.CheckResult{
			Name:      c.name,
			Service:   c.service,
			Status:    checks.StatusFailed,
			Error:     fmt.Errorf("container %s not found or docker unavailable: %w", c.containerName, err),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
			Metadata:  metadata,
		}
	}

	c.populateMetadata(metadata, &info)

	checkStatus, statusErr := c.evaluateState(info.State)

	return checks.CheckResult{
		Name:      c.name,
		Service:   c.service,
		Status:    checkStatus,
		Error:     statusErr,
		Timestamp: time.Now(),
		Duration:  time.Since(start),
		Metadata:  metadata,
	}
}

// populateMetadata fills in metadata from the inspect response.
func (c *DockerContainerCheck) populateMetadata(metadata map[string]string, info *container.InspectResponse) {
	if info.State != nil {
		metadata["container_status"] = info.State.Status
		metadata["started_at"] = info.State.StartedAt
		if info.State.Health != nil {
			metadata["health_status"] = info.State.Health.Status
			metadata["health_failing_streak"] = fmt.Sprintf("%d", info.State.Health.FailingStreak)
		}
	}
	if info.ContainerJSONBase != nil {
		metadata["image"] = info.Image
		metadata["restart_count"] = fmt.Sprintf("%d", info.RestartCount)
	}
}

// evaluateState maps Docker container state to a check status.
// "running" → healthy (or degraded if Docker HEALTHCHECK reports unhealthy)
// "paused", "restarting", "created" → degraded
// "exited", "dead", "removing" → failed
func (c *DockerContainerCheck) evaluateState(state *container.State) (checks.CheckStatus, error) {
	if state == nil {
		return checks.StatusFailed, fmt.Errorf("container %s has no state", c.containerName)
	}

	switch state.Status {
	case container.StateRunning:
		// Container is running, but check Docker's own HEALTHCHECK if present.
		if state.Health != nil && state.Health.Status == container.Unhealthy {
			return checks.StatusDegraded, fmt.Errorf("container %s is running but healthcheck is unhealthy", c.containerName)
		}
		return checks.StatusHealthy, nil

	case container.StatePaused, container.StateRestarting, container.StateCreated:
		return checks.StatusDegraded, fmt.Errorf("container %s is %s", c.containerName, state.Status)

	default: // exited, dead, removing
		return checks.StatusFailed, fmt.Errorf("container %s is %s", c.containerName, state.Status)
	}
}
