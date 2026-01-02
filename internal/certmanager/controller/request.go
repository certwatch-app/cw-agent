package controller

import (
	"context"
	"sync"
	"time"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/certwatch-app/cw-agent/internal/certmanager/metrics"
	"github.com/certwatch-app/cw-agent/internal/certmanager/types"
)

// CertificateRequestReconciler watches cert-manager CertificateRequest resources
type CertificateRequestReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Logger *zap.Logger

	// Callback for immediate sync on failure
	OnFailure func(req types.CertificateRequestStatus)

	// Track requests for metrics
	mu       sync.RWMutex
	requests map[string]types.CertificateRequestStatus // key: namespace/name
}

// NewCertificateRequestReconciler creates a new reconciler
func NewCertificateRequestReconciler(c client.Client, scheme *runtime.Scheme, logger *zap.Logger) *CertificateRequestReconciler {
	return &CertificateRequestReconciler{
		Client:   c,
		Scheme:   scheme,
		Logger:   logger,
		requests: make(map[string]types.CertificateRequestStatus),
	}
}

// Reconcile handles CertificateRequest changes
func (r *CertificateRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	log := r.Logger.With(
		zap.String("namespace", req.Namespace),
		zap.String("name", req.Name),
	)

	var cr cmapi.CertificateRequest
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// CertificateRequest was deleted
			log.Debug("certificaterequest deleted")
			r.removeRequest(req.Namespace, req.Name)
			metrics.ReconcileTotal.WithLabelValues("certificaterequest", "deleted").Inc()
			return ctrl.Result{}, nil
		}
		log.Error("failed to get certificaterequest", zap.Error(err))
		metrics.ReconcileTotal.WithLabelValues("certificaterequest", "error").Inc()
		return ctrl.Result{}, err
	}

	// Extract status
	status := r.extractStatus(&cr)
	r.storeRequest(status)

	// Update metrics
	r.updateMetrics(status)

	// Trigger immediate sync on failure
	if (status.Failed || status.Denied) && r.OnFailure != nil {
		log.Info("certificaterequest failure detected, triggering immediate sync",
			zap.Bool("failed", status.Failed),
			zap.Bool("denied", status.Denied),
			zap.String("category", status.FailureCategory),
		)
		r.OnFailure(status)
	}

	log.Debug("certificaterequest reconciled",
		zap.Bool("approved", status.Approved),
		zap.Bool("ready", status.Ready),
		zap.Bool("failed", status.Failed),
		zap.Bool("denied", status.Denied),
	)

	metrics.ReconcileTotal.WithLabelValues("certificaterequest", "success").Inc()
	metrics.ReconcileDuration.WithLabelValues("certificaterequest").Observe(time.Since(start).Seconds())

	return ctrl.Result{}, nil
}

func (r *CertificateRequestReconciler) extractStatus(cr *cmapi.CertificateRequest) types.CertificateRequestStatus {
	status := types.CertificateRequestStatus{
		Namespace: cr.Namespace,
		Name:      cr.Name,
		CreatedAt: cr.CreationTimestamp.Time,
	}

	// Get owner certificate name from owner references
	for _, ref := range cr.OwnerReferences {
		if ref.Kind == "Certificate" {
			status.CertificateName = ref.Name
			break
		}
	}

	// Extract conditions
	for _, cond := range cr.Status.Conditions {
		switch cond.Type {
		case cmapi.CertificateRequestConditionApproved:
			status.Approved = cond.Status == cmmeta.ConditionTrue
		case cmapi.CertificateRequestConditionDenied:
			status.Denied = cond.Status == cmmeta.ConditionTrue
			if status.Denied {
				status.FailureReason = cond.Reason
				status.FailureMessage = cond.Message
			}
		case cmapi.CertificateRequestConditionReady:
			status.Ready = cond.Status == cmmeta.ConditionTrue
			// Only mark as failed when explicitly failed (reason == "Failed")
			// For "Pending" with error messages, rely on Events for failure detection
			if cond.Status == cmmeta.ConditionFalse && cond.Reason == "Failed" {
				status.Failed = true
				status.FailureReason = cond.Reason
				status.FailureMessage = cond.Message
			}
		case cmapi.CertificateRequestConditionInvalidRequest:
			if cond.Status == cmmeta.ConditionTrue {
				status.Failed = true
				status.FailureReason = cond.Reason
				status.FailureMessage = cond.Message
			}
		}
	}

	// Failure time from status
	if cr.Status.FailureTime != nil {
		t := cr.Status.FailureTime.Time
		status.FailureTime = &t
		status.Failed = true
	}

	// Categorize failure if applicable
	if status.Failed || status.Denied {
		status.FailureCategory = types.CategorizeFailure(status.FailureReason, status.FailureMessage)
	}

	// Calculate duration if issued
	if status.Ready && len(cr.Status.Certificate) > 0 {
		now := time.Now()
		status.IssuedAt = &now
		status.Duration = now.Sub(status.CreatedAt)
	}

	return status
}

func (r *CertificateRequestReconciler) storeRequest(status types.CertificateRequestStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := status.Namespace + "/" + status.Name
	r.requests[key] = status
}

func (r *CertificateRequestReconciler) removeRequest(namespace, name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := namespace + "/" + name
	delete(r.requests, key)
}

// GetRequests returns all tracked certificate requests
func (r *CertificateRequestReconciler) GetRequests() []types.CertificateRequestStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	requests := make([]types.CertificateRequestStatus, 0, len(r.requests))
	for k := range r.requests {
		requests = append(requests, r.requests[k])
	}
	return requests
}

// GetFailedRequests returns only failed or denied certificate requests
func (r *CertificateRequestReconciler) GetFailedRequests() []types.CertificateRequestStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	failed := make([]types.CertificateRequestStatus, 0)
	for k := range r.requests {
		if r.requests[k].Failed || r.requests[k].Denied {
			failed = append(failed, r.requests[k])
		}
	}
	return failed
}

func (r *CertificateRequestReconciler) updateMetrics(status types.CertificateRequestStatus) {
	// Track request status
	var statusLabel string
	switch {
	case status.Denied:
		statusLabel = "denied"
	case status.Failed:
		statusLabel = "failed"
	case status.Ready:
		statusLabel = "ready"
	case status.Approved:
		statusLabel = "approved"
	default:
		statusLabel = "pending"
	}

	metrics.RequestTotal.WithLabelValues(status.Namespace, statusLabel).Inc()

	// Track duration for successful issuance
	if status.Ready && status.Duration > 0 {
		// Get issuer kind from parent certificate if possible
		issuerKind := "unknown"
		metrics.RequestDuration.WithLabelValues(status.Namespace, issuerKind).Observe(status.Duration.Seconds())
	}
}

// SetupWithManager sets up the controller with the Manager
func (r *CertificateRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cmapi.CertificateRequest{}).
		Named("certificaterequest").
		Complete(r)
}
