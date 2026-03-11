package kubernetes

import (
	"fmt"

	k8sclient "k8s.io/client-go/kubernetes"
)

// NewClient creates a Kubernetes clientset by auto-discovering the
// cluster configuration (in-cluster first, then kubeconfig fallback).
func NewClient() (k8sclient.Interface, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, fmt.Errorf("building kubernetes config: %w", err)
	}

	client, err := k8sclient.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}

	return client, nil
}
