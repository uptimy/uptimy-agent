package kubernetes

import (
	"context"
	"fmt"
	"time"

	"github.com/uptimy/uptimy-agent/internal/checks"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// failureReasons are Kubernetes event reasons that indicate a failure.
var failureReasons = map[string]bool{
	"CrashLoopBackOff": true,
	"Failed":           true,
	"FailedScheduling": true,
	"BackOff":          true,
	"Unhealthy":        true,
	"FailedMount":      true,
	"OOMKilling":       true,
}

// Watcher monitors Kubernetes events and produces CheckResults for failures.
type Watcher struct {
	client    k8sclient.Interface
	namespace string
	results   chan<- checks.CheckResult
	logger    *zap.SugaredLogger
}

// NewWatcher creates a Kubernetes event watcher.
// If namespace is empty, watches all namespaces.
// It auto-discovers the Kubernetes client from the environment.
func NewWatcher(namespace string, results chan<- checks.CheckResult, logger *zap.SugaredLogger) (*Watcher, error) {
	client, err := NewClient()
	if err != nil {
		return nil, err
	}

	return NewWatcherWithClient(client, namespace, results, logger), nil
}

// NewWatcherWithClient creates a Kubernetes event watcher using the
// provided client. This constructor enables unit testing with a fake
// Kubernetes clientset.
func NewWatcherWithClient(client k8sclient.Interface, namespace string, results chan<- checks.CheckResult, logger *zap.SugaredLogger) *Watcher {
	return &Watcher{
		client:    client,
		namespace: namespace,
		results:   results,
		logger:    logger,
	}
}

// buildConfig tries in-cluster config first, then falls back to kubeconfig.
func buildConfig() (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg, nil
	}

	// Fallback to kubeconfig.
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
}

// Run starts watching Kubernetes events. Blocks until ctx is canceled.
func (w *Watcher) Run(ctx context.Context) {
	w.logger.Infow("starting kubernetes watcher", "namespace", w.namespaceDisplay())

	for {
		if err := w.watchEvents(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			w.logger.Warnw("kubernetes watch error, retrying in 10s", "error", err)
			select {
			case <-time.After(10 * time.Second):
			case <-ctx.Done():
				return
			}
		}
	}
}

func (w *Watcher) watchEvents(ctx context.Context) error {
	ns := w.namespace
	if ns == "" {
		ns = metav1.NamespaceAll
	}

	watcher, err := w.client.CoreV1().Events(ns).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("watching events: %w", err)
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("watch channel closed")
			}
			if event.Type == watch.Error {
				return fmt.Errorf("watch error event")
			}
			if event.Type == watch.Added || event.Type == watch.Modified {
				w.processEvent(event.Object)
			}
		}
	}
}

func (w *Watcher) processEvent(obj interface{}) {
	event, ok := obj.(*corev1.Event)
	if !ok {
		return
	}

	// Only process warning events with known failure reasons.
	if event.Type != "Warning" {
		return
	}
	if !failureReasons[event.Reason] {
		return
	}

	checkName := fmt.Sprintf("k8s-%s-%s", event.InvolvedObject.Kind, event.InvolvedObject.Name)
	service := event.InvolvedObject.Namespace + "/" + event.InvolvedObject.Name

	result := checks.CheckResult{
		Name:      checkName,
		Service:   service,
		Status:    checks.StatusFailed,
		Error:     fmt.Errorf("kubernetes event: %s - %s", event.Reason, event.Message),
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"kind":      event.InvolvedObject.Kind,
			"name":      event.InvolvedObject.Name,
			"namespace": event.InvolvedObject.Namespace,
			"reason":    event.Reason,
			"message":   event.Message,
		},
	}

	select {
	case w.results <- result:
		w.logger.Infow("kubernetes failure detected",
			"reason", event.Reason,
			"object", service,
			"message", event.Message,
		)
	default:
		w.logger.Warnw("check results channel full, dropping k8s event",
			"reason", event.Reason,
			"object", service,
		)
	}
}

func (w *Watcher) namespaceDisplay() string {
	if w.namespace == "" {
		return "all"
	}
	return w.namespace
}
