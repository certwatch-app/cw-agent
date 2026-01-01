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
