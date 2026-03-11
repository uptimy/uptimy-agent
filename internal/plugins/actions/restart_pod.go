package actions

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "k8s.io/client-go/kubernetes"
)

// RestartPodAction restarts a Kubernetes pod by deleting it.
// The owning controller (Deployment, ReplicaSet, etc.) will recreate
// the pod automatically. When running outside Kubernetes the action
// returns an error.
type RestartPodAction struct {
	client k8sclient.Interface
	logger *zap.SugaredLogger
}

// NewRestartPodAction creates a RestartPodAction.
// Pass a nil client when Kubernetes is not available.
func NewRestartPodAction(client k8sclient.Interface, logger *zap.SugaredLogger) *RestartPodAction {
	return &RestartPodAction{client: client, logger: logger}
}

func (a *RestartPodAction) Name() string { return "restart_pod" }

func (a *RestartPodAction) Execute(ctx context.Context, params map[string]string) error {
	if a.client == nil {
		return fmt.Errorf("restart_pod: kubernetes is not available")
	}

	namespace := params["namespace"]
	if namespace == "" {
		namespace = "default"
	}
	pod := params["pod"]
	if pod == "" {
		return fmt.Errorf("restart_pod: 'pod' parameter is required")
	}

	a.logger.Infow("deleting pod to trigger restart", "namespace", namespace, "pod", pod)

	// GracePeriodSeconds defaults to 30s. Use grace_period param to override.
	gracePeriod := int64(30)
	if v, ok := params["grace_period"]; ok {
		if _, err := fmt.Sscan(v, &gracePeriod); err != nil {
			a.logger.Warnw("invalid grace_period, using default 30s", "raw", v)
			gracePeriod = 30
		}
	}

	err := a.client.CoreV1().Pods(namespace).Delete(ctx, pod, metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
	})
	if err != nil {
		return fmt.Errorf("restart_pod: deleting pod %s/%s: %w", namespace, pod, err)
	}

	a.logger.Infow("pod deleted successfully, controller will recreate it",
		"namespace", namespace, "pod", pod)
	return nil
}
