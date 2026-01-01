// Package sync provides the API client for syncing with CertWatch cloud.
package sync

import (
	"time"
)

// SyncRequest represents the agent sync request payload
type SyncRequest struct {
	AgentID                  string                `json:"agent_id,omitempty"`
	PreviousAgentID          string                `json:"previous_agent_id,omitempty"` // For migration when agent name changes
	AgentName                string                `json:"agent_name"`
	AgentVersion             string                `json:"agent_version,omitempty"`
	AgentHost                string                `json:"agent_hostname,omitempty"`
	HeartbeatIntervalSeconds int                   `json:"heartbeat_interval_seconds,omitempty"` // Heartbeat interval for offline detection
	Certificates             []CertificateSyncData `json:"certificates"`
}

// CertificateSyncData represents certificate data sent to the API
// Fields are ordered for optimal memory alignment
type CertificateSyncData struct {
	NotBefore         *time.Time       `json:"not_before,omitempty"`
	NotAfter          *time.Time       `json:"not_after,omitempty"`
	LastCheckAt       *time.Time       `json:"last_check_at,omitempty"`
	ChainValid        *bool            `json:"chain_valid,omitempty"`
	Hostname          string           `json:"hostname"`
	Notes             string           `json:"notes,omitempty"`
	Subject           string           `json:"subject,omitempty"`
	Issuer            string           `json:"issuer,omitempty"`
	IssuerOrg         string           `json:"issuer_org,omitempty"`
	SerialNumber      string           `json:"serial_number,omitempty"`
	FingerprintSHA256 string           `json:"fingerprint_sha256,omitempty"`
	LastError         string           `json:"last_error,omitempty"`
	Tags              []string         `json:"tags,omitempty"`
	SANList           []string         `json:"san_list,omitempty"`
	ChainIssues       []ChainIssueData `json:"chain_issues,omitempty"`
	Port              int              `json:"port"`
}

// ChainIssueData represents a chain issue in the sync payload
type ChainIssueData struct {
	Type             string `json:"type"`
	Message          string `json:"message"`
	CertificateIndex int    `json:"certificate_index,omitempty"`
}

// SyncResponse represents the API response from sync
// Fields are ordered for optimal memory alignment
type SyncResponse struct {
	Error   *APIError        `json:"error,omitempty"`
	AgentID string           `json:"agent_id"`
	Data    SyncResponseData `json:"data"`
	Success bool             `json:"success"`
}

// SyncResponseData contains the sync result details
type SyncResponseData struct {
	SyncedAt  time.Time   `json:"synced_at"`
	Errors    []SyncError `json:"errors,omitempty"`
	Created   int         `json:"created"`
	Updated   int         `json:"updated"`
	Unchanged int         `json:"unchanged"`
	Orphaned  int         `json:"orphaned"`
	Migrated  int         `json:"migrated"` // Certs migrated from previous agent
}

// SyncError represents an error for a specific certificate during sync
type SyncError struct {
	Hostname string `json:"hostname"`
	Error    string `json:"error"`
	Port     int    `json:"port"`
}

// APIError represents an API error response
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// HeartbeatRequest represents the agent heartbeat request payload
type HeartbeatRequest struct {
	AgentID          string     `json:"agent_id"`
	AgentName        string     `json:"agent_name"`
	AgentVersion     string     `json:"agent_version,omitempty"`
	CertificateCount int        `json:"certificate_count,omitempty"`
	Status           string     `json:"status,omitempty"` // "healthy", "degraded", "unhealthy"
	LastScanAt       *time.Time `json:"last_scan_at,omitempty"`
	LastSyncAt       *time.Time `json:"last_sync_at,omitempty"`
}

// HeartbeatResponse represents the API response from heartbeat
type HeartbeatResponse struct {
	Success    bool      `json:"success"`
	AgentID    string    `json:"agent_id"`
	ServerTime time.Time `json:"server_time"`
}

// CertManagerSyncRequest is the request for syncing cert-manager certificates
type CertManagerSyncRequest struct {
	AgentID      string                   `json:"agent_id,omitempty"`
	AgentName    string                   `json:"agent_name"`
	AgentVersion string                   `json:"agent_version,omitempty"`
	ClusterName  string                   `json:"cluster_name"`
	Certificates []CertManagerCertificate `json:"certificates"`
}

// CertManagerCertificate is a cert-manager certificate for sync
type CertManagerCertificate struct {
	// Identity
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	SecretName string `json:"secret_name"`

	// Spec
	CommonName string   `json:"common_name,omitempty"`
	DNSNames   []string `json:"dns_names,omitempty"`

	// Issuer
	IssuerName  string `json:"issuer_name"`
	IssuerKind  string `json:"issuer_kind"`
	IssuerGroup string `json:"issuer_group,omitempty"`

	// Status
	Ready       bool   `json:"ready"`
	ReadyReason string `json:"ready_reason,omitempty"`
	Issuing     bool   `json:"issuing"`

	// Timing
	NotBefore   *time.Time `json:"not_before,omitempty"`
	NotAfter    *time.Time `json:"not_after,omitempty"`
	RenewalTime *time.Time `json:"renewal_time,omitempty"`

	// Health
	Revision       int `json:"revision"`
	FailedAttempts int `json:"failed_attempts"`
}

// CertManagerSyncResponse is the response from the cert-manager sync endpoint
type CertManagerSyncResponse struct {
	Success bool                        `json:"success"`
	AgentID string                      `json:"agent_id"`
	Error   *APIError                   `json:"error,omitempty"`
	Data    CertManagerSyncResponseData `json:"data"`
}

// CertManagerSyncResponseData contains the sync result details for cert-manager
type CertManagerSyncResponseData struct {
	SyncedAt  time.Time `json:"synced_at"`
	Created   int       `json:"created"`
	Updated   int       `json:"updated"`
	Unchanged int       `json:"unchanged"`
	Deleted   int       `json:"deleted"`
}

// ============================================================================
// Phase 2: Event Sync Types
// ============================================================================

// CertManagerEventSyncRequest is the request for syncing cert-manager events
type CertManagerEventSyncRequest struct {
	AgentID     string             `json:"agent_id,omitempty"`
	AgentName   string             `json:"agent_name"`
	ClusterName string             `json:"cluster_name"`
	Events      []CertManagerEvent `json:"events"`
}

// CertManagerEvent represents a cert-manager related Kubernetes Event for sync
type CertManagerEvent struct {
	CertificateNamespace string    `json:"certificate_namespace"`
	CertificateName      string    `json:"certificate_name"`
	Reason               string    `json:"reason"`
	Message              string    `json:"message"`
	Type                 string    `json:"event_type"` // Normal, Warning
	Timestamp            time.Time `json:"timestamp"`
	IsFailure            bool      `json:"is_failure"`
	FailureCategory      string    `json:"failure_category,omitempty"` // issuer, acme, validation, policy
}

// CertManagerEventSyncResponse is the response from the event sync endpoint
type CertManagerEventSyncResponse struct {
	Success bool                             `json:"success"`
	Error   *APIError                        `json:"error,omitempty"`
	Data    CertManagerEventSyncResponseData `json:"data"`
}

// CertManagerEventSyncResponseData contains the event sync result details
type CertManagerEventSyncResponseData struct {
	SyncedAt     time.Time `json:"synced_at"`
	EventsStored int       `json:"events_stored"`
}

// ============================================================================
// Phase 2: CertificateRequest Sync Types
// ============================================================================

// CertManagerRequestSyncRequest is the request for syncing CertificateRequests
type CertManagerRequestSyncRequest struct {
	AgentID     string               `json:"agent_id,omitempty"`
	AgentName   string               `json:"agent_name"`
	ClusterName string               `json:"cluster_name"`
	Requests    []CertManagerRequest `json:"requests"`
}

// CertManagerRequest represents a cert-manager CertificateRequest for sync
type CertManagerRequest struct {
	Namespace       string     `json:"namespace"`
	Name            string     `json:"name"`
	CertificateName string     `json:"certificate_name,omitempty"`
	Approved        bool       `json:"approved"`
	Denied          bool       `json:"denied"`
	Ready           bool       `json:"ready"`
	Failed          bool       `json:"failed"`
	FailureReason   string     `json:"failure_reason,omitempty"`
	FailureMessage  string     `json:"failure_message,omitempty"`
	FailureCategory string     `json:"failure_category,omitempty"`
	FailureTime     *time.Time `json:"failure_time,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	IssuedAt        *time.Time `json:"issued_at,omitempty"`
	DurationMs      int64      `json:"duration_ms,omitempty"`
}

// CertManagerRequestSyncResponse is the response from the request sync endpoint
type CertManagerRequestSyncResponse struct {
	Success bool                               `json:"success"`
	Error   *APIError                          `json:"error,omitempty"`
	Data    CertManagerRequestSyncResponseData `json:"data"`
}

// CertManagerRequestSyncResponseData contains the request sync result details
type CertManagerRequestSyncResponseData struct {
	SyncedAt       time.Time `json:"synced_at"`
	RequestsStored int       `json:"requests_stored"`
}
