package checkers

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/swarm"
	dockerclient "github.com/docker/docker/client"
	"github.com/uptimy/uptimy-agent/internal/checks"
)

// DockerSwarmCheck verifies that Docker Swarm is active on the node
// using the Docker Engine SDK.
type DockerSwarmCheck struct {
	name    string
	service string
	timeout time.Duration
}

// NewDockerSwarmCheck creates a new Docker Swarm health check.
func NewDockerSwarmCheck(name, service string, timeout time.Duration) *DockerSwarmCheck {
	return &DockerSwarmCheck{
		name:    name,
		service: service,
		timeout: timeout,
	}
}

// Name returns the check's unique identifier.
func (c *DockerSwarmCheck) Name() string { return c.name }

// Run executes the Docker Swarm check and returns the result.
func (c *DockerSwarmCheck) Run(ctx context.Context) checks.CheckResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	metadata := map[string]string{}

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

	info, err := cli.Info(ctx)
	if err != nil {
		return checks.CheckResult{
			Name:      c.name,
			Service:   c.service,
			Status:    checks.StatusFailed,
			Error:     fmt.Errorf("failed to query docker info: %w", err),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
			Metadata:  metadata,
		}
	}

	si := info.Swarm
	metadata["local_node_state"] = string(si.LocalNodeState)
	metadata["node_id"] = si.NodeID
	metadata["node_addr"] = si.NodeAddr
	metadata["control_available"] = fmt.Sprintf("%t", si.ControlAvailable)
	metadata["managers"] = fmt.Sprintf("%d", si.Managers)
	metadata["nodes"] = fmt.Sprintf("%d", si.Nodes)
	metadata["reachable_managers"] = fmt.Sprintf("%d", len(si.RemoteManagers))

	checkStatus, statusErr := c.evaluateSwarm(&si)

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

// evaluateSwarm maps the swarm state to a check status.
// active with quorum → healthy
// active but managers lost or error string set → degraded
// pending → degraded
// inactive, error, or anything else → failed
func (c *DockerSwarmCheck) evaluateSwarm(si *swarm.Info) (checks.CheckStatus, error) {
	switch si.LocalNodeState {
	case swarm.LocalNodeStateActive:
		if si.Error != "" {
			return checks.StatusDegraded, fmt.Errorf("swarm active but reports error: %s", si.Error)
		}
		// Check manager quorum: reachable managers should be a majority.
		reachable := len(si.RemoteManagers)
		if si.Managers > 0 && reachable < (si.Managers+1)/2 {
			return checks.StatusDegraded, fmt.Errorf("swarm active but only %d/%d managers reachable", reachable, si.Managers)
		}
		return checks.StatusHealthy, nil

	case swarm.LocalNodeStatePending:
		return checks.StatusDegraded, fmt.Errorf("swarm node is pending, not yet active")

	case swarm.LocalNodeStateError:
		return checks.StatusFailed, fmt.Errorf("swarm is in error state: %s", si.Error)

	default: // inactive, locked, or unknown
		return checks.StatusFailed, fmt.Errorf("swarm is not active, state: %s", si.LocalNodeState)
	}
}
