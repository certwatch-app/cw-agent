package ca

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sWatcher watches a Kubernetes ConfigMap or Secret for changes using polling.
//
// Design Note: We use polling instead of the Kubernetes Watch API because:
// - controller-runtime's client.Client doesn't have a Watch() method
// - client.WithWatch requires special client setup (NewWithWatch)
// - Polling is simpler and more reliable for infrequent updates (CA bundles)
// - 30-second polling is perfectly acceptable for CA bundle changes
//
// For more on Watch API: https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/client
type K8sWatcher struct {
	client       client.Client
	namespace    string
	name         string
	resourceType string        // "ConfigMap" or "Secret"
	lastVersion  string        // ResourceVersion for change detection
	pollInterval time.Duration // Polling interval (default: 30s)
	onChange     func()
	logger       *zap.Logger
	stopCh       chan struct{}
}

// NewK8sWatcher creates a new Kubernetes resource watcher
func NewK8sWatcher(clientInterface interface{}, namespace, name, resourceType string, onChange func(), logger *zap.Logger) (*K8sWatcher, error) {
	// Type assert the client
	k8sClient, ok := clientInterface.(client.Client)
	if !ok {
		return nil, fmt.Errorf("invalid client type, expected controller-runtime client.Client")
	}

	return &K8sWatcher{
		client:       k8sClient,
		namespace:    namespace,
		name:         name,
		resourceType: resourceType,
		pollInterval: 30 * time.Second, // Poll every 30 seconds
		onChange:     onChange,
		logger:       logger,
		stopCh:       make(chan struct{}),
	}, nil
}

// Start starts watching the Kubernetes resource using polling
func (w *K8sWatcher) Start(ctx context.Context) error {
	// Get initial resource version
	if err := w.updateResourceVersion(ctx); err != nil {
		w.logger.Warn("Failed to get initial resource version",
			zap.String("type", w.resourceType),
			zap.String("namespace", w.namespace),
			zap.String("name", w.name),
			zap.Error(err))
	}

	w.logger.Info("Started polling Kubernetes resource",
		zap.String("type", w.resourceType),
		zap.String("namespace", w.namespace),
		zap.String("name", w.name),
		zap.Duration("interval", w.pollInterval))

	go w.pollLoop(ctx)
	return nil
}

// pollLoop polls the Kubernetes resource periodically
func (w *K8sWatcher) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Debug("Kubernetes watcher stopped (context canceled)")
			return

		case <-w.stopCh:
			w.logger.Debug("Kubernetes watcher stopped")
			return

		case <-ticker.C:
			if err := w.checkForChanges(ctx); err != nil {
				w.logger.Error("Failed to check for changes",
					zap.String("resource", fmt.Sprintf("%s/%s", w.namespace, w.name)),
					zap.Error(err))
			}
		}
	}
}

// checkForChanges checks if the resource has changed
func (w *K8sWatcher) checkForChanges(ctx context.Context) error {
	var currentVersion string
	var err error

	// Get current resource version based on type
	switch w.resourceType {
	case "ConfigMap":
		cm := &corev1.ConfigMap{}
		err = w.client.Get(ctx, client.ObjectKey{
			Namespace: w.namespace,
			Name:      w.name,
		}, cm)
		if err == nil {
			currentVersion = cm.ResourceVersion
		}

	case "Secret":
		secret := &corev1.Secret{}
		err = w.client.Get(ctx, client.ObjectKey{
			Namespace: w.namespace,
			Name:      w.name,
		}, secret)
		if err == nil {
			currentVersion = secret.ResourceVersion
		}

	default:
		return fmt.Errorf("unsupported resource type: %s", w.resourceType)
	}

	if err != nil {
		return fmt.Errorf("failed to get resource: %w", err)
	}

	// Check if version changed
	if currentVersion != w.lastVersion && w.lastVersion != "" {
		w.logger.Info("Resource changed",
			zap.String("type", w.resourceType),
			zap.String("name", w.name),
			zap.String("old_version", w.lastVersion),
			zap.String("new_version", currentVersion))

		w.lastVersion = currentVersion
		w.onChange()
	} else if w.lastVersion == "" {
		// First check, just store the version
		w.lastVersion = currentVersion
		w.logger.Debug("Initial resource version stored",
			zap.String("type", w.resourceType),
			zap.String("name", w.name),
			zap.String("version", currentVersion))
	}

	return nil
}

// updateResourceVersion fetches and stores the current resource version
func (w *K8sWatcher) updateResourceVersion(ctx context.Context) error {
	var currentVersion string
	var err error

	switch w.resourceType {
	case "ConfigMap":
		cm := &corev1.ConfigMap{}
		err = w.client.Get(ctx, client.ObjectKey{
			Namespace: w.namespace,
			Name:      w.name,
		}, cm)
		if err == nil {
			currentVersion = cm.ResourceVersion
		}

	case "Secret":
		secret := &corev1.Secret{}
		err = w.client.Get(ctx, client.ObjectKey{
			Namespace: w.namespace,
			Name:      w.name,
		}, secret)
		if err == nil {
			currentVersion = secret.ResourceVersion
		}

	default:
		return fmt.Errorf("unsupported resource type: %s", w.resourceType)
	}

	if err != nil {
		return err
	}

	w.lastVersion = currentVersion
	return nil
}

// Stop stops the Kubernetes watcher
func (w *K8sWatcher) Stop() error {
	close(w.stopCh)
	return nil
}
