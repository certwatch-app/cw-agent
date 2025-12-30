// Package sync provides the API client for syncing with CertWatch cloud.
package sync

import (
	"time"
)

// SyncRequest represents the agent sync request payload
type SyncRequest struct {
	AgentID      string                `json:"agent_id,omitempty"`
	AgentName    string                `json:"agent_name"`
	AgentVersion string                `json:"agent_version,omitempty"`
	AgentHost    string                `json:"agent_hostname,omitempty"`
	Certificates []CertificateSyncData `json:"certificates"`
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
	Errors    []SyncError `json:"errors,omitempty"`
	Created   int         `json:"created"`
	Updated   int         `json:"updated"`
	Unchanged int         `json:"unchanged"`
	Orphaned  int         `json:"orphaned"`
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
