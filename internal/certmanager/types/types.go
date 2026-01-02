package types

import "time"

// CertificateStatus represents the extracted state of a cert-manager Certificate
type CertificateStatus struct {
	// Identity
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	SecretName string `json:"secret_name"`

	// Spec (what user requested)
	CommonName   string   `json:"common_name,omitempty"`
	DNSNames     []string `json:"dns_names,omitempty"`
	KeyAlgorithm string   `json:"key_algorithm,omitempty"`
	KeySize      int      `json:"key_size,omitempty"`

	// Issuer Reference
	IssuerName  string `json:"issuer_name"`
	IssuerKind  string `json:"issuer_kind"`
	IssuerGroup string `json:"issuer_group,omitempty"`

	// Status (current state)
	Ready         bool   `json:"ready"`
	ReadyReason   string `json:"ready_reason,omitempty"`
	ReadyMessage  string `json:"ready_message,omitempty"`
	Issuing       bool   `json:"issuing"`
	IssuingReason string `json:"issuing_reason,omitempty"`

	// Timing
	NotBefore      *time.Time `json:"not_before,omitempty"`
	NotAfter       *time.Time `json:"not_after,omitempty"`
	RenewalTime    *time.Time `json:"renewal_time,omitempty"`
	LastTransition *time.Time `json:"last_transition,omitempty"`

	// Health
	Revision        int        `json:"revision"`
	FailedAttempts  int        `json:"failed_attempts"`
	LastFailureTime *time.Time `json:"last_failure_time,omitempty"`
}

// CertManagerSyncPayload is the request body for syncing cert-manager data
type CertManagerSyncPayload struct {
	EventType    string              `json:"event_type"` // "certmanager.certificate_sync"
	Timestamp    time.Time           `json:"timestamp"`
	AgentID      string              `json:"agent_id"`
	ClusterName  string              `json:"cluster_name"`
	Certificates []CertificateStatus `json:"certificates"`
}

// ============================================================================
// Phase 2: CertificateRequest and Event Types
// ============================================================================

// FailureCategory constants for categorizing cert-manager failures
const (
	FailureCategoryIssuer     = "issuer"
	FailureCategoryACME       = "acme"
	FailureCategoryValidation = "validation"
	FailureCategoryPolicy     = "policy"
	FailureCategoryUnknown    = "unknown"
)

// CertificateRequestStatus holds status of a CertificateRequest
type CertificateRequestStatus struct {
	// Identity
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	CertificateName string `json:"certificate_name"` // Owner reference

	// Status conditions
	Approved bool `json:"approved"`
	Denied   bool `json:"denied"`
	Ready    bool `json:"ready"`
	Failed   bool `json:"failed"`

	// Failure details
	FailureReason   string     `json:"failure_reason,omitempty"`
	FailureMessage  string     `json:"failure_message,omitempty"`
	FailureTime     *time.Time `json:"failure_time,omitempty"`
	FailureCategory string     `json:"failure_category,omitempty"` // issuer, acme, validation, policy

	// Timing
	CreatedAt time.Time     `json:"created_at"`
	IssuedAt  *time.Time    `json:"issued_at,omitempty"`
	Duration  time.Duration `json:"duration_ms,omitempty"` // Time to issue
}

// CertManagerEvent represents a cert-manager related Kubernetes Event
type CertManagerEvent struct {
	// Source certificate
	CertificateNamespace string `json:"certificate_namespace"`
	CertificateName      string `json:"certificate_name"`

	// Event details
	Reason    string    `json:"reason"` // Issuing, Failed, OrderFailed, etc.
	Message   string    `json:"message"`
	Type      string    `json:"event_type"` // Normal, Warning
	Timestamp time.Time `json:"timestamp"`

	// Derived failure info
	IsFailure       bool   `json:"is_failure"`
	FailureCategory string `json:"failure_category,omitempty"` // issuer, acme, validation, policy
}

// CertManagerEventSyncPayload is the request body for syncing cert-manager events
type CertManagerEventSyncPayload struct {
	AgentID     string             `json:"agent_id,omitempty"`
	AgentName   string             `json:"agent_name"`
	ClusterName string             `json:"cluster_name"`
	Events      []CertManagerEvent `json:"events"`
}
