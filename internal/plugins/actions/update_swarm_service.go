package actions

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/swarm"
	dockerclient "github.com/docker/docker/client"
	"go.uber.org/zap"
)

// UpdateSwarmServiceAction force-updates a Docker Swarm service to trigger
// a rolling restart of its tasks, using the Docker Engine SDK.
type UpdateSwarmServiceAction struct {
	logger *zap.SugaredLogger
}

// NewUpdateSwarmServiceAction creates an UpdateSwarmServiceAction.
func NewUpdateSwarmServiceAction(logger *zap.SugaredLogger) *UpdateSwarmServiceAction {
	return &UpdateSwarmServiceAction{logger: logger}
}

// Name returns the action name.
func (a *UpdateSwarmServiceAction) Name() string { return "update_swarm_service" }

// Execute runs the swarm service force-update action.
func (a *UpdateSwarmServiceAction) Execute(ctx context.Context, params map[string]string) error {
	serviceName := params["service"]
	if serviceName == "" {
		return fmt.Errorf("update_swarm_service: 'service' parameter is required")
	}

	a.logger.Infow("force-updating swarm service", "service", serviceName)

	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("update_swarm_service: failed to create docker client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	// Inspect the service to get its current spec and version.
	svc, _, err := cli.ServiceInspectWithRaw(ctx, serviceName, swarm.ServiceInspectOptions{})
	if err != nil {
		return fmt.Errorf("update_swarm_service: failed to inspect service %s: %w", serviceName, err)
	}

	// Bump ForceUpdate to trigger a rolling restart without changing the image or config.
	svc.Spec.TaskTemplate.ForceUpdate++

	resp, err := cli.ServiceUpdate(ctx, svc.ID, svc.Version, svc.Spec, swarm.ServiceUpdateOptions{})
	if err != nil {
		return fmt.Errorf("update_swarm_service: failed to update service %s: %w", serviceName, err)
	}

	for _, w := range resp.Warnings {
		a.logger.Warnw("swarm service update warning", "service", serviceName, "warning", w)
	}

	a.logger.Infow("swarm service force-updated successfully", "service", serviceName)
	return nil
}
