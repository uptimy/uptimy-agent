package actions

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "k8s.io/client-go/kubernetes"
)

// RollbackDeploymentAction rolls back a Kubernetes deployment to its
// previous revision by patching the deployment's pod template to match
// the second-most-recent ReplicaSet.
type RollbackDeploymentAction struct {
	client k8sclient.Interface
	logger *zap.SugaredLogger
}

// NewRollbackDeploymentAction creates a RollbackDeploymentAction.
func NewRollbackDeploymentAction(client k8sclient.Interface, logger *zap.SugaredLogger) *RollbackDeploymentAction {
	return &RollbackDeploymentAction{client: client, logger: logger}
}

// Name returns the action name.
func (a *RollbackDeploymentAction) Name() string { return "rollback_deployment" }

// Execute runs the rollback deployment action.
func (a *RollbackDeploymentAction) Execute(ctx context.Context, params map[string]string) error {
	if a.client == nil {
		return fmt.Errorf("rollback_deployment: kubernetes is not available")
	}

	namespace := params["namespace"]
	if namespace == "" {
		namespace = "default"
	}
	deployment := params["deployment"]
	if deployment == "" {
		return fmt.Errorf("rollback_deployment: 'deployment' parameter is required")
	}

	a.logger.Infow("rolling back deployment", "namespace", namespace, "deployment", deployment)

	// Fetch the deployment to find the previous ReplicaSet revision.
	deploy, err := a.client.AppsV1().Deployments(namespace).Get(ctx, deployment, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("rollback_deployment: getting deployment %s/%s: %w", namespace, deployment, err)
	}

	// List ReplicaSets owned by this deployment.
	rsList, err := a.client.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(deploy.Spec.Selector),
	})
	if err != nil {
		return fmt.Errorf("rollback_deployment: listing replicasets: %w", err)
	}

	// Find the second-highest revision (the previous one).
	prev := findPreviousRevision(rsList.Items, deploy)
	if prev == nil {
		return fmt.Errorf("rollback_deployment: no previous revision found for %s/%s", namespace, deployment)
	}

	a.logger.Infow("found previous revision",
		"namespace", namespace,
		"deployment", deployment,
		"previousRevision", prev.Annotations["deployment.kubernetes.io/revision"],
	)

	// Patch the deployment's pod template to match the previous ReplicaSet.
	// This triggers Kubernetes to roll out to that template (a rollback).
	patchData, err := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"template": prev.Spec.Template,
		},
	})
	if err != nil {
		return fmt.Errorf("rollback_deployment: marshaling patch: %w", err)
	}

	_, err = a.client.AppsV1().Deployments(namespace).Patch(
		ctx, deployment, types.MergePatchType, patchData, metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("rollback_deployment: patching deployment %s/%s: %w", namespace, deployment, err)
	}

	a.logger.Infow("deployment rollback initiated", "namespace", namespace, "deployment", deployment)
	return nil
}

// findPreviousRevision returns the ReplicaSet with the second-highest
// revision number that is owned by the given deployment.
func findPreviousRevision(rsList []appsv1.ReplicaSet, deploy *appsv1.Deployment) *appsv1.ReplicaSet {
	var maxRevision, prevRevision int64
	var prev *appsv1.ReplicaSet

	for i := range rsList {
		rs := &rsList[i]

		// Only consider ReplicaSets owned by this deployment.
		if !isOwnedBy(rs.OwnerReferences, deploy.UID) {
			continue
		}

		rev := parseRevision(rs.Annotations["deployment.kubernetes.io/revision"])
		if rev > maxRevision {
			prevRevision = maxRevision
			prev = nil
			maxRevision = rev
		}
		if rev == prevRevision && rev > 0 {
			prev = rs
		}
		if rev > prevRevision && rev < maxRevision {
			prevRevision = rev
			prev = rs
		}
	}

	return prev
}

func isOwnedBy(refs []metav1.OwnerReference, uid types.UID) bool {
	for _, ref := range refs {
		if ref.UID == uid {
			return true
		}
	}
	return false
}

func parseRevision(s string) int64 {
	var v int64
	_, _ = fmt.Sscan(s, &v)
	return v
}
