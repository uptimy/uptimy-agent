// Package kubernetes provides Kubernetes cluster event watching for the
// Uptimy Agent. It detects failure events and converts them to check
// results that feed into the incident manager.
package kubernetes

import "os"

// IsRunningInCluster returns true if the agent appears to be running
// inside a Kubernetes cluster (checks for the service host env var).
func IsRunningInCluster() bool {
	return os.Getenv("KUBERNETES_SERVICE_HOST") != ""
}
