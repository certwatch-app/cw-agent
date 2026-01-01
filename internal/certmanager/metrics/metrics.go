package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func init() {
	// Register all metrics with controller-runtime's registry
	ctrlmetrics.Registry.MustRegister(
		CertificateReady,
		CertificateIssuing,
		CertificateExpirySeconds,
		CertificateDaysUntilExpiry,
		CertificateFailedAttempts,
		ReconcileTotal,
		ReconcileDuration,
		SyncTotal,
		SyncDuration,
		HeartbeatTotal,
		AgentInfo,
		CertificatesWatched,
	)
}

var (
	// Certificate metrics

	// CertificateReady tracks whether a certificate is ready
	CertificateReady = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "certwatch",
		Subsystem: "certmanager",
		Name:      "certificate_ready",
		Help:      "Whether the certificate is ready (1=ready, 0=not ready)",
	}, []string{"namespace", "name", "issuer_kind", "issuer_name"})

	// CertificateIssuing tracks whether a certificate is being issued
	CertificateIssuing = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "certwatch",
		Subsystem: "certmanager",
		Name:      "certificate_issuing",
		Help:      "Whether the certificate is being issued (1=issuing, 0=not issuing)",
	}, []string{"namespace", "name"})

	// CertificateExpirySeconds tracks certificate expiry as Unix timestamp
	CertificateExpirySeconds = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "certwatch",
		Subsystem: "certmanager",
		Name:      "certificate_expiry_seconds",
		Help:      "Unix timestamp of certificate expiry",
	}, []string{"namespace", "name"})

	// CertificateDaysUntilExpiry tracks days until certificate expires
	CertificateDaysUntilExpiry = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "certwatch",
		Subsystem: "certmanager",
		Name:      "certificate_days_until_expiry",
		Help:      "Days until certificate expires",
	}, []string{"namespace", "name"})

	// CertificateFailedAttempts tracks failed issuance attempts
	CertificateFailedAttempts = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "certwatch",
		Subsystem: "certmanager",
		Name:      "certificate_failed_attempts",
		Help:      "Number of failed issuance attempts",
	}, []string{"namespace", "name"})

	// Controller metrics

	// ReconcileTotal counts reconciliation operations
	ReconcileTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "certwatch",
		Subsystem: "certmanager",
		Name:      "reconcile_total",
		Help:      "Total number of reconciliations",
	}, []string{"controller", "result"})

	// ReconcileDuration tracks reconciliation duration
	ReconcileDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "certwatch",
		Subsystem: "certmanager",
		Name:      "reconcile_duration_seconds",
		Help:      "Duration of reconciliation in seconds",
		Buckets:   prometheus.DefBuckets,
	}, []string{"controller"})

	// Sync metrics

	// SyncTotal counts sync operations
	SyncTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "certwatch",
		Subsystem: "certmanager",
		Name:      "sync_total",
		Help:      "Total number of sync operations",
	}, []string{"status"})

	// SyncDuration tracks sync operation duration
	SyncDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "certwatch",
		Subsystem: "certmanager",
		Name:      "sync_duration_seconds",
		Help:      "Duration of sync operations in seconds",
		Buckets:   prometheus.DefBuckets,
	})

	// HeartbeatTotal counts heartbeat operations
	HeartbeatTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "certwatch",
		Subsystem: "certmanager",
		Name:      "heartbeat_total",
		Help:      "Total number of heartbeat operations",
	}, []string{"status"})

	// Agent info metrics

	// AgentInfo provides agent metadata
	AgentInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "certwatch",
		Subsystem: "certmanager",
		Name:      "agent_info",
		Help:      "Agent information",
	}, []string{"version", "cluster_name"})

	// CertificatesWatched tracks number of watched certificates
	CertificatesWatched = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "certwatch",
		Subsystem: "certmanager",
		Name:      "certificates_watched",
		Help:      "Number of certificates being watched",
	})
)
