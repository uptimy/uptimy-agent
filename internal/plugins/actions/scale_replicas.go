package actions

import (
	"context"
	"fmt"
	"strconv"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "k8s.io/client-go/kubernetes"
)

// ScaleReplicasAction scales a Kubernetes deployment's replica count.
type ScaleReplicasAction struct {
	client k8sclient.Interface
	logger *zap.SugaredLogger
}

// NewScaleReplicasAction creates a ScaleReplicasAction.
func NewScaleReplicasAction(client k8sclient.Interface, logger *zap.SugaredLogger) *ScaleReplicasAction {
	return &ScaleReplicasAction{client: client, logger: logger}
}

// Name returns the action name.
func (a *ScaleReplicasAction) Name() string { return "scale_replicas" }

// Execute runs the scale replicas action.
func (a *ScaleReplicasAction) Execute(ctx context.Context, params map[string]string) error {
	if a.client == nil {
		return fmt.Errorf("scale_replicas: kubernetes is not available")
	}

	namespace := params["namespace"]
	if namespace == "" {
		namespace = "default"
	}
	deployment := params["deployment"]
	if deployment == "" {
		return fmt.Errorf("scale_replicas: 'deployment' parameter is required")
	}
	replicasStr := params["replicas"]
	if replicasStr == "" {
		return fmt.Errorf("scale_replicas: 'replicas' parameter is required")
	}

	replicas, err := strconv.ParseInt(replicasStr, 10, 32)
	if err != nil || replicas < 0 {
		return fmt.Errorf("scale_replicas: 'replicas' must be a non-negative integer (got %q)", replicasStr)
	}

	a.logger.Infow("scaling deployment replicas",
		"namespace", namespace, "deployment", deployment, "replicas", replicas)

	// Fetch the current scale sub-resource.
	scale, err := a.client.AppsV1().Deployments(namespace).GetScale(
		ctx, deployment, metav1.GetOptions{},
	)
	if err != nil {
		return fmt.Errorf("scale_replicas: getting scale for %s/%s: %w", namespace, deployment, err)
	}

	oldReplicas := scale.Spec.Replicas
	scale.Spec.Replicas = int32(replicas)

	_, err = a.client.AppsV1().Deployments(namespace).UpdateScale(
		ctx, deployment, scale, metav1.UpdateOptions{},
	)
	if err != nil {
		return fmt.Errorf("scale_replicas: updating scale for %s/%s: %w", namespace, deployment, err)
	}

	a.logger.Infow("deployment scaled successfully",
		"namespace", namespace,
		"deployment", deployment,
		"from", oldReplicas,
		"to", replicas,
	)
	return nil
}
