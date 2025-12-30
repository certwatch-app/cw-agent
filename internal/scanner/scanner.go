package scanner

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/certwatch-app/cw-agent/internal/config"
)

// Scanner handles TLS certificate scanning
// Fields are ordered for optimal memory alignment
type Scanner struct {
	logger      *zap.Logger
	timeout     time.Duration
	concurrency int
}

// New creates a new Scanner
func New(timeout time.Duration, concurrency int, logger *zap.Logger) *Scanner {
	return &Scanner{
		timeout:     timeout,
		concurrency: concurrency,
		logger:      logger,
	}
}

// ScanAll scans all configured certificates concurrently
func (s *Scanner) ScanAll(ctx context.Context, certs []config.CertificateConfig) []ScanResult {
	results := make([]ScanResult, len(certs))
	var wg sync.WaitGroup

	// Use a semaphore channel for concurrency control
	sem := make(chan struct{}, s.concurrency)

	for i, cert := range certs {
		wg.Add(1)
		go func(idx int, c config.CertificateConfig) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx] = ScanResult{
					Hostname:  c.Hostname,
					Port:      c.Port,
					Success:   false,
					Error:     "context canceled",
					ScannedAt: time.Now().UTC(),
				}
				return
			}

			results[idx] = s.Scan(ctx, c.Hostname, c.Port)
		}(i, cert)
	}

	wg.Wait()
	return results
}

// Scan performs a TLS connection and extracts certificate information
func (s *Scanner) Scan(ctx context.Context, hostname string, port int) ScanResult {
	result := ScanResult{
		Hostname:  hostname,
		Port:      port,
		ScannedAt: time.Now().UTC(),
	}

	addr := fmt.Sprintf("%s:%d", hostname, port)

	// Create TLS config
	// We intentionally skip TLS verification and validate manually to inspect the full chain
	tlsConfig := &tls.Config{
		ServerName:         hostname,
		InsecureSkipVerify: true, //nolint:gosec // We validate manually to inspect the full certificate chain
	}

	// Create dialer with timeout
	dialer := &net.Dialer{
		Timeout: s.timeout,
	}

	// Establish connection with context
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("connection failed: %v", err)
		s.logger.Debug("scan failed",
			zap.String("hostname", hostname),
			zap.Int("port", port),
			zap.Error(err),
		)
		return result
	}
	defer conn.Close()

	// Get peer certificates
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		result.Success = false
		result.Error = "no certificates received"
		return result
	}

	// Parse leaf certificate
	leaf := state.PeerCertificates[0]
	result.Success = true
	result.Certificate = s.parseCertificate(leaf)

	// Parse chain
	result.Chain = s.parseChain(state.PeerCertificates, hostname)

	s.logger.Debug("scan successful",
		zap.String("hostname", hostname),
		zap.Int("port", port),
		zap.String("subject", result.Certificate.Subject),
		zap.Int("days_until_expiry", result.Certificate.DaysUntilExpiry),
	)

	return result
}

func (s *Scanner) parseCertificate(cert *x509.Certificate) *CertificateInfo {
	// Calculate SHA256 fingerprint
	fingerprint := sha256.Sum256(cert.Raw)
	fingerprintHex := hex.EncodeToString(fingerprint[:])

	// Extract issuer organization
	issuerOrg := ""
	if len(cert.Issuer.Organization) > 0 {
		issuerOrg = cert.Issuer.Organization[0]
	}

	// Calculate days until expiry
	daysUntilExpiry := int(time.Until(cert.NotAfter).Hours() / 24)

	// Extract SAN list
	sanList := make([]string, 0, len(cert.DNSNames)+len(cert.IPAddresses))
	sanList = append(sanList, cert.DNSNames...)
	for _, ip := range cert.IPAddresses {
		sanList = append(sanList, ip.String())
	}

	return &CertificateInfo{
		Subject:           cert.Subject.CommonName,
		Issuer:            cert.Issuer.CommonName,
		IssuerOrg:         issuerOrg,
		SerialNumber:      cert.SerialNumber.String(),
		FingerprintSHA256: fingerprintHex,
		NotBefore:         cert.NotBefore.UTC(),
		NotAfter:          cert.NotAfter.UTC(),
		SANList:           sanList,
		DaysUntilExpiry:   daysUntilExpiry,
	}
}

func (s *Scanner) parseChain(certs []*x509.Certificate, hostname string) *ChainInfo {
	chain := &ChainInfo{
		Valid:        true,
		Issues:       make([]ChainIssue, 0),
		Certificates: make([]ChainCertificate, 0, len(certs)),
	}

	now := time.Now()

	// Build chain certificates list
	for i, cert := range certs {
		chain.Certificates = append(chain.Certificates, ChainCertificate{
			Subject:   cert.Subject.CommonName,
			Issuer:    cert.Issuer.CommonName,
			NotBefore: cert.NotBefore.UTC(),
			NotAfter:  cert.NotAfter.UTC(),
		})

		// Check for expiration
		if now.After(cert.NotAfter) {
			chain.Valid = false
			chain.Issues = append(chain.Issues, ChainIssue{
				Type:             "expired",
				Message:          fmt.Sprintf("Certificate expired on %s", cert.NotAfter.Format(time.RFC3339)),
				CertificateIndex: i,
			})
		}

		// Check for not yet valid
		if now.Before(cert.NotBefore) {
			chain.Valid = false
			chain.Issues = append(chain.Issues, ChainIssue{
				Type:             "not_yet_valid",
				Message:          fmt.Sprintf("Certificate not valid until %s", cert.NotBefore.Format(time.RFC3339)),
				CertificateIndex: i,
			})
		}

		// Check for self-signed leaf
		if i == 0 && cert.Subject.String() == cert.Issuer.String() {
			chain.Issues = append(chain.Issues, ChainIssue{
				Type:             "self_signed",
				Message:          "Leaf certificate is self-signed",
				CertificateIndex: i,
			})
		}
	}

	// Verify hostname matches
	if len(certs) > 0 {
		leaf := certs[0]
		if err := leaf.VerifyHostname(hostname); err != nil {
			chain.Issues = append(chain.Issues, ChainIssue{
				Type:             "hostname_mismatch",
				Message:          fmt.Sprintf("Certificate does not match hostname: %v", err),
				CertificateIndex: 0,
			})
		}
	}

	// Check for weak signature algorithms
	for i, cert := range certs {
		if isWeakSignature(cert.SignatureAlgorithm.String()) {
			chain.Issues = append(chain.Issues, ChainIssue{
				Type:             "weak_crypto",
				Message:          fmt.Sprintf("Weak signature algorithm: %s", cert.SignatureAlgorithm.String()),
				CertificateIndex: i,
			})
		}
	}

	return chain
}

func isWeakSignature(algo string) bool {
	weak := []string{"MD2", "MD5", "SHA1"}
	algo = strings.ToUpper(algo)
	for _, w := range weak {
		if strings.Contains(algo, w) {
			return true
		}
	}
	return false
}
