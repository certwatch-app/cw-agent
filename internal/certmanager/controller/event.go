package controller

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/certwatch-app/cw-agent/internal/certmanager/metrics"
	"github.com/certwatch-app/cw-agent/internal/certmanager/types"
)

// EventWatcher watches Kubernetes Events for cert-manager related events
type EventWatcher struct {
	client.Client
	Scheme *runtime.Scheme
	Logger *zap.Logger

	// Callback for immediate sync on failure event
	OnFailureEvent func(event types.CertManagerEvent)

	// Buffer recent events for batch sync
	mu     sync.RWMutex
	events []types.CertManagerEvent
	maxAge time.Duration // How long to keep events in buffer
}

// NewEventWatcher creates a new event watcher
func NewEventWatcher(c client.Client, scheme *runtime.Scheme, logger *zap.Logger) *EventWatcher {
	return &EventWatcher{
		Client: c,
		Scheme: scheme,
		Logger: logger,
		events: make([]types.CertManagerEvent, 0),
		maxAge: 30 * time.Minute, // Keep events for 30 minutes
	}
}

// Reconcile handles Event changes
func (w *EventWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	log := w.Logger.With(
		zap.String("namespace", req.Namespace),
		zap.String("name", req.Name),
	)

	var event corev1.Event
	if err := w.Get(ctx, req.NamespacedName, &event); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Event was deleted (normal for old events)
			return ctrl.Result{}, nil
		}
		log.Error("failed to get event", zap.Error(err))
		return ctrl.Result{}, err
	}

	// Only process cert-manager related events
	if !w.isCertManagerEvent(&event) {
		return ctrl.Result{}, nil
	}

	// Extract event data
	cmEvent := w.extractEvent(&event)

	// Store event in buffer
	w.storeEvent(cmEvent)

	// Update metrics
	w.updateMetrics(cmEvent)

	// Trigger immediate sync on failure
	if cmEvent.IsFailure && w.OnFailureEvent != nil {
		log.Info("cert-manager failure event detected, triggering immediate sync",
			zap.String("reason", cmEvent.Reason),
			zap.String("category", cmEvent.FailureCategory),
			zap.String("certificate", cmEvent.CertificateName),
		)
		w.OnFailureEvent(cmEvent)
	}

	log.Debug("cert-manager event processed",
		zap.String("reason", cmEvent.Reason),
		zap.String("certificate", cmEvent.CertificateName),
		zap.Bool("isFailure", cmEvent.IsFailure),
	)

	metrics.ReconcileTotal.WithLabelValues("event", "success").Inc()
	metrics.ReconcileDuration.WithLabelValues("event").Observe(time.Since(start).Seconds())

	return ctrl.Result{}, nil
}

// isCertManagerEvent checks if the event is related to cert-manager resources
func (w *EventWatcher) isCertManagerEvent(event *corev1.Event) bool {
	// Filter by involved object kind
	kind := event.InvolvedObject.Kind
	certManagerKinds := map[string]bool{
		"Certificate":        true,
		"CertificateRequest": true,
		"Order":              true,
		"Challenge":          true,
		"Issuer":             true,
		"ClusterIssuer":      true,
	}

	if !certManagerKinds[kind] {
		return false
	}

	// Filter by API group - must be cert-manager.io
	apiVersion := event.InvolvedObject.APIVersion
	return strings.Contains(apiVersion, "cert-manager.io")
}

func (w *EventWatcher) extractEvent(event *corev1.Event) types.CertManagerEvent {
	cmEvent := types.CertManagerEvent{
		CertificateNamespace: event.InvolvedObject.Namespace,
		CertificateName:      event.InvolvedObject.Name,
		Reason:               event.Reason,
		Message:              event.Message,
		Type:                 event.Type,
	}

	// Get timestamp - prefer LastTimestamp, fall back to EventTime
	switch {
	case !event.LastTimestamp.IsZero():
		cmEvent.Timestamp = event.LastTimestamp.Time
	case !event.EventTime.IsZero():
		cmEvent.Timestamp = event.EventTime.Time
	default:
		cmEvent.Timestamp = time.Now()
	}

	// Determine if this is a failure event
	cmEvent.IsFailure = types.IsFailureEvent(event.Reason)

	// Categorize failure
	if cmEvent.IsFailure {
		cmEvent.FailureCategory = types.CategorizeFailure(event.Reason, event.Message)
	}

	return cmEvent
}

func (w *EventWatcher) storeEvent(event types.CertManagerEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Add event to buffer
	w.events = append(w.events, event)

	// Clean up old events
	cutoff := time.Now().Add(-w.maxAge)
	filtered := make([]types.CertManagerEvent, 0, len(w.events))
	for i := range w.events {
		if w.events[i].Timestamp.After(cutoff) {
			filtered = append(filtered, w.events[i])
		}
	}
	w.events = filtered
}

// GetEvents returns all buffered events and clears the buffer
func (w *EventWatcher) GetEvents() []types.CertManagerEvent {
	w.mu.Lock()
	defer w.mu.Unlock()

	events := w.events
	w.events = make([]types.CertManagerEvent, 0)
	return events
}

// GetRecentEvents returns events from the buffer without clearing
func (w *EventWatcher) GetRecentEvents(since time.Duration) []types.CertManagerEvent {
	w.mu.RLock()
	defer w.mu.RUnlock()

	cutoff := time.Now().Add(-since)
	events := make([]types.CertManagerEvent, 0)
	for i := range w.events {
		if w.events[i].Timestamp.After(cutoff) {
			events = append(events, w.events[i])
		}
	}
	return events
}

// GetFailureEvents returns only failure events from buffer
func (w *EventWatcher) GetFailureEvents() []types.CertManagerEvent {
	w.mu.RLock()
	defer w.mu.RUnlock()

	failures := make([]types.CertManagerEvent, 0)
	for i := range w.events {
		if w.events[i].IsFailure {
			failures = append(failures, w.events[i])
		}
	}
	return failures
}

func (w *EventWatcher) updateMetrics(event types.CertManagerEvent) {
	failureCategory := ""
	if event.IsFailure {
		failureCategory = event.FailureCategory
	}
	metrics.EventTotal.WithLabelValues(event.Reason, event.Type, failureCategory).Inc()
}

// certManagerEventFilter filters events to only cert-manager related ones
func certManagerEventFilter() predicate.Funcs {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		event, ok := obj.(*corev1.Event)
		if !ok {
			return false
		}

		// Quick filter by API group in involved object
		apiVersion := event.InvolvedObject.APIVersion
		return strings.Contains(apiVersion, "cert-manager.io")
	})
}

// SetupWithManager sets up the controller with the Manager
func (w *EventWatcher) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Event{}).
		WithEventFilter(certManagerEventFilter()).
		Named("event").
		Complete(w)
}
