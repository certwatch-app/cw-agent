// Package scanner provides TLS certificate scanning functionality.
package scanner

import (
	"fmt"
	"time"
)

// ScanResult represents the result of scanning a single certificate
// Fields are ordered for optimal memory alignment
type ScanResult struct {
	Certificate *CertificateInfo
	Chain       *ChainInfo
	Hostname    string
	Error       string
	ScannedAt   time.Time
	Port        int
	Success     bool
}

// CertificateInfo contains parsed certificate information
type CertificateInfo struct {
	Subject           string
	Issuer            string
	IssuerOrg         string
	SerialNumber      string
	FingerprintSHA256 string
	SANList           []string
	NotBefore         time.Time
	NotAfter          time.Time
	DaysUntilExpiry   int
}

// ChainInfo contains certificate chain information
// Fields are ordered for optimal memory alignment
type ChainInfo struct {
	Issues       []ChainIssue
	Certificates []ChainCertificate
	Valid        bool
}

// ChainIssue represents an issue with the certificate chain
type ChainIssue struct {
	Type             string `json:"type"`
	Message          string `json:"message"`
	CertificateIndex int    `json:"certificate_index,omitempty"`
}

// ChainCertificate represents a certificate in the chain
// Fields are ordered for optimal memory alignment
type ChainCertificate struct {
	Subject   string    `json:"subject"`
	Issuer    string    `json:"issuer"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
}

// GetHostPort returns the hostname:port string
func (r *ScanResult) GetHostPort() string {
	return formatHostPort(r.Hostname, r.Port)
}

func formatHostPort(hostname string, port int) string {
	if port == 443 {
		return hostname
	}
	return fmt.Sprintf("%s:%d", hostname, port)
}
