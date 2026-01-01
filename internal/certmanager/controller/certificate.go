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

// CertificateReconciler watches cert-manager Certificate resources
type CertificateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Logger *zap.Logger

	// Sync state
	mu           sync.RWMutex
	certificates map[string]types.CertificateStatus // key: namespace/name
}

// NewCertificateReconciler creates a new reconciler
func NewCertificateReconciler(c client.Client, scheme *runtime.Scheme, logger *zap.Logger) *CertificateReconciler {
	return &CertificateReconciler{
		Client:       c,
		Scheme:       scheme,
		Logger:       logger,
		certificates: make(map[string]types.CertificateStatus),
	}
}

// Reconcile handles Certificate changes
func (r *CertificateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	log := r.Logger.With(
		zap.String("namespace", req.Namespace),
		zap.String("name", req.Name),
	)

	var cert cmapi.Certificate
	if err := r.Get(ctx, req.NamespacedName, &cert); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Certificate was deleted
			log.Debug("certificate deleted")
			r.removeCertificate(req.Namespace, req.Name)
			metrics.ReconcileTotal.WithLabelValues("certificate", "deleted").Inc()
			return ctrl.Result{}, nil
		}
		log.Error("failed to get certificate", zap.Error(err))
		metrics.ReconcileTotal.WithLabelValues("certificate", "error").Inc()
		return ctrl.Result{}, err
	}

	// Extract status
	status := r.extractStatus(&cert)
	r.storeCertificate(status)

	// Update metrics
	r.updateMetrics(status)

	log.Debug("certificate reconciled",
		zap.Bool("ready", status.Ready),
		zap.Bool("issuing", status.Issuing),
	)

	metrics.ReconcileTotal.WithLabelValues("certificate", "success").Inc()
	metrics.ReconcileDuration.WithLabelValues("certificate").Observe(time.Since(start).Seconds())

	// Requeue to catch external changes (e.g., time-based expiry updates)
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *CertificateReconciler) extractStatus(cert *cmapi.Certificate) types.CertificateStatus {
	status := types.CertificateStatus{
		Namespace:  cert.Namespace,
		Name:       cert.Name,
		SecretName: cert.Spec.SecretName,
		CommonName: cert.Spec.CommonName,
		DNSNames:   cert.Spec.DNSNames,
		IssuerName: cert.Spec.IssuerRef.Name,
		IssuerKind: cert.Spec.IssuerRef.Kind,
	}

	// Default issuer kind if not set
	if status.IssuerKind == "" {
		status.IssuerKind = "Issuer"
	}

	if cert.Spec.IssuerRef.Group != "" {
		status.IssuerGroup = cert.Spec.IssuerRef.Group
	}

	// Extract key algorithm
	if cert.Spec.PrivateKey != nil {
		status.KeyAlgorithm = string(cert.Spec.PrivateKey.Algorithm)
		status.KeySize = cert.Spec.PrivateKey.Size
	}

	// Extract timing from status
	if cert.Status.NotBefore != nil {
		t := cert.Status.NotBefore.Time
		status.NotBefore = &t
	}
	if cert.Status.NotAfter != nil {
		t := cert.Status.NotAfter.Time
		status.NotAfter = &t
	}
	if cert.Status.RenewalTime != nil {
		t := cert.Status.RenewalTime.Time
		status.RenewalTime = &t
	}

	// Extract conditions
	for _, cond := range cert.Status.Conditions {
		switch cond.Type {
		case cmapi.CertificateConditionReady:
			status.Ready = cond.Status == cmmeta.ConditionTrue
			status.ReadyReason = cond.Reason
			status.ReadyMessage = cond.Message
			if !cond.LastTransitionTime.IsZero() {
				t := cond.LastTransitionTime.Time
				status.LastTransition = &t
			}
		case cmapi.CertificateConditionIssuing:
			status.Issuing = cond.Status == cmmeta.ConditionTrue
			status.IssuingReason = cond.Reason
		}
	}

	// Failure tracking
	if cert.Status.Revision != nil {
		status.Revision = *cert.Status.Revision
	}
	if cert.Status.FailedIssuanceAttempts != nil {
		status.FailedAttempts = *cert.Status.FailedIssuanceAttempts
	}
	if cert.Status.LastFailureTime != nil {
		t := cert.Status.LastFailureTime.Time
		status.LastFailureTime = &t
	}

	return status
}

func (r *CertificateReconciler) storeCertificate(status types.CertificateStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := status.Namespace + "/" + status.Name
	r.certificates[key] = status
	metrics.CertificatesWatched.Set(float64(len(r.certificates)))
}

func (r *CertificateReconciler) removeCertificate(namespace, name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := namespace + "/" + name
	delete(r.certificates, key)
	metrics.CertificatesWatched.Set(float64(len(r.certificates)))

	// Clean up metrics for deleted certificate
	metrics.CertificateReady.DeleteLabelValues(namespace, name, "", "")
	metrics.CertificateIssuing.DeleteLabelValues(namespace, name)
	metrics.CertificateExpirySeconds.DeleteLabelValues(namespace, name)
	metrics.CertificateDaysUntilExpiry.DeleteLabelValues(namespace, name)
	metrics.CertificateFailedAttempts.DeleteLabelValues(namespace, name)
}

// GetCertificates returns all watched certificates for syncing
func (r *CertificateReconciler) GetCertificates() []types.CertificateStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	certs := make([]types.CertificateStatus, 0, len(r.certificates))
	for k := range r.certificates {
		certs = append(certs, r.certificates[k])
	}
	return certs
}

// CertificateCount returns the number of watched certificates
func (r *CertificateReconciler) CertificateCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.certificates)
}

func (r *CertificateReconciler) updateMetrics(status types.CertificateStatus) {
	labels := []string{status.Namespace, status.Name}
	issuerLabels := []string{status.Namespace, status.Name, status.IssuerKind, status.IssuerName}

	if status.Ready {
		metrics.CertificateReady.WithLabelValues(issuerLabels...).Set(1)
	} else {
		metrics.CertificateReady.WithLabelValues(issuerLabels...).Set(0)
	}

	if status.Issuing {
		metrics.CertificateIssuing.WithLabelValues(labels...).Set(1)
	} else {
		metrics.CertificateIssuing.WithLabelValues(labels...).Set(0)
	}

	if status.NotAfter != nil {
		metrics.CertificateExpirySeconds.WithLabelValues(labels...).Set(float64(status.NotAfter.Unix()))
		days := time.Until(*status.NotAfter).Hours() / 24
		metrics.CertificateDaysUntilExpiry.WithLabelValues(labels...).Set(days)
	}

	metrics.CertificateFailedAttempts.WithLabelValues(labels...).Set(float64(status.FailedAttempts))
}

// SetupWithManager sets up the controller with the Manager
func (r *CertificateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cmapi.Certificate{}).
		Named("certificate").
		Complete(r)
}
