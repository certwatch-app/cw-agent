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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/certwatch-app/cw-agent/internal/ca"
	"github.com/certwatch-app/cw-agent/internal/config"
)

// Scanner handles TLS certificate scanning
// Fields are ordered for optimal memory alignment
type Scanner struct {
	logger       *zap.Logger
	caLoader     *ca.Loader         // Phase 1: CA loader for validation
	cacheManager *ca.CACacheManager // Phase 2: Cache manager for hot-reload
	k8sClient    interface{}        // Phase 2: Kubernetes client (controller-runtime client.Client)
	timeout      time.Duration
	concurrency  int
}

// New creates a new Scanner
func New(timeout time.Duration, concurrency int, logger *zap.Logger) *Scanner {
	caLoader := ca.NewLoader(logger)
	cacheManager := ca.NewCACacheManager(caLoader, logger)

	return &Scanner{
		timeout:      timeout,
		concurrency:  concurrency,
		logger:       logger,
		caLoader:     caLoader,
		cacheManager: cacheManager,
	}
}

// NewWithK8sClient creates a Scanner with Kubernetes client support
// This is used by the cert-manager controller to enable ConfigMap/Secret sources
func NewWithK8sClient(timeout time.Duration, concurrency int, k8sClient interface{}, logger *zap.Logger) *Scanner {
	scanner := New(timeout, concurrency, logger)
	scanner.k8sClient = k8sClient
	return scanner
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

	// Create context-aware TLS dialer
	tlsDialer := &tls.Dialer{
		NetDialer: &net.Dialer{
			Timeout: s.timeout,
		},
		Config: tlsConfig,
	}

	// Establish connection with context
	netConn, err := tlsDialer.DialContext(ctx, "tcp", addr)
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
	defer netConn.Close() //nolint:errcheck // TLS connection close in defer; certificate data already extracted, close error non-actionable

	// Type assert to TLS connection
	conn, ok := netConn.(*tls.Conn)
	if !ok {
		result.Success = false
		result.Error = "connection is not a TLS connection"
		return result
	}

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

// ScanAllWithCA scans all configured certificates with CA validation support
// Supports per-certificate CA override
func (s *Scanner) ScanAllWithCA(ctx context.Context, certs []config.CertificateConfig, globalCA *config.CAConfig) []ScanResult {
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

			// Determine effective CA config (per-cert override or global)
			effectiveCA := globalCA
			if c.CA != nil {
				effectiveCA = c.CA
			}

			results[idx] = s.ScanWithCA(ctx, c.Hostname, c.Port, effectiveCA)
		}(i, cert)
	}

	wg.Wait()
	return results
}

// ScanWithCA performs a TLS connection with CA validation based on configuration
func (s *Scanner) ScanWithCA(ctx context.Context, hostname string, port int, caConfig *config.CAConfig) ScanResult {
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

	// Create context-aware TLS dialer
	tlsDialer := &tls.Dialer{
		NetDialer: &net.Dialer{
			Timeout: s.timeout,
		},
		Config: tlsConfig,
	}

	// Establish connection with context
	netConn, err := tlsDialer.DialContext(ctx, "tcp", addr)
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
	defer netConn.Close() //nolint:errcheck // TLS connection close in defer; certificate data already extracted, close error non-actionable

	// Type assert to TLS connection
	conn, ok := netConn.(*tls.Conn)
	if !ok {
		result.Success = false
		result.Error = "connection is not a TLS connection"
		return result
	}

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

	// Parse chain with CA validation
	result.Chain = s.parseChainWithCA(state.PeerCertificates, hostname, caConfig)

	s.logger.Debug("scan successful",
		zap.String("hostname", hostname),
		zap.Int("port", port),
		zap.String("subject", result.Certificate.Subject),
		zap.Int("days_until_expiry", result.Certificate.DaysUntilExpiry),
		zap.String("validation_mode", result.Chain.ValidationMode),
	)

	return result
}

// parseChainWithCA parses the certificate chain with optional CA validation
func (s *Scanner) parseChainWithCA(certs []*x509.Certificate, hostname string, caConfig *config.CAConfig) *ChainInfo {
	chain := &ChainInfo{
		Valid:        true,
		Issues:       make([]ChainIssue, 0),
		Certificates: make([]ChainCertificate, 0, len(certs)),
	}

	// Set validation mode (default to "chain" if no config)
	validationMode := "chain"
	if caConfig != nil {
		validationMode = caConfig.ValidationMode
	}
	chain.ValidationMode = validationMode

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

	// Perform CA validation if not in "none" mode
	if validationMode != "none" && caConfig != nil {
		if err := s.performCAValidation(certs, hostname, caConfig, chain); err != nil {
			chain.Valid = false
			chain.ValidationError = err.Error()
			chain.Issues = append(chain.Issues, ChainIssue{
				Type:             "ca_validation_failed",
				Message:          err.Error(),
				CertificateIndex: 0,
			})
			s.logger.Debug("CA validation failed",
				zap.String("hostname", hostname),
				zap.Error(err))
		}
	}

	return chain
}

// performCAValidation performs x509.Verify with the configured CA pool
func (s *Scanner) performCAValidation(certs []*x509.Certificate, hostname string, caConfig *config.CAConfig, chain *ChainInfo) error {
	if len(certs) == 0 {
		return fmt.Errorf("no certificates to validate")
	}

	// Phase 2: Build CA sources and use cache manager
	sources, err := s.buildCASources(context.Background(), caConfig)
	if err != nil {
		return fmt.Errorf("failed to build CA sources: %w", err)
	}

	// Get or load CA pool (cached for performance)
	certPool, err := s.cacheManager.GetOrLoad(context.Background(), sources, caConfig.TrustMode)
	if err != nil {
		return fmt.Errorf("failed to load CA pool: %w", err)
	}

	// Build intermediates pool (all certs except leaf)
	intermediates := x509.NewCertPool()
	for i := 1; i < len(certs); i++ {
		intermediates.AddCert(certs[i])
	}

	// Set up verification options
	opts := x509.VerifyOptions{
		Roots:         certPool,
		Intermediates: intermediates,
		DNSName:       hostname,
		CurrentTime:   time.Now(),
	}

	// For "basic" mode, skip hostname verification
	if caConfig.ValidationMode == "basic" {
		opts.DNSName = ""
	}

	// Perform validation
	verifiedChains, err := certs[0].Verify(opts)
	if err != nil {
		return fmt.Errorf("certificate validation failed: %w", err)
	}

	// Store verified chains
	chain.VerifiedChains = make([][]ChainCertificate, len(verifiedChains))
	for i, verifiedChain := range verifiedChains {
		chain.VerifiedChains[i] = make([]ChainCertificate, len(verifiedChain))
		for j, cert := range verifiedChain {
			chain.VerifiedChains[i][j] = ChainCertificate{
				Subject:   cert.Subject.CommonName,
				Issuer:    cert.Issuer.CommonName,
				NotBefore: cert.NotBefore.UTC(),
				NotAfter:  cert.NotAfter.UTC(),
			}
		}
	}

	// Extract trusted root from first verified chain
	if len(verifiedChains) > 0 && len(verifiedChains[0]) > 0 {
		rootCert := verifiedChains[0][len(verifiedChains[0])-1]
		chain.TrustedRoot = rootCert.Subject.CommonName
	}

	s.logger.Debug("CA validation successful",
		zap.String("hostname", hostname),
		zap.String("trusted_root", chain.TrustedRoot),
		zap.Int("verified_chains", len(verifiedChains)))

	return nil
}

// buildCASources converts CAConfig to CASource instances (Phase 2)
func (s *Scanner) buildCASources(ctx context.Context, caConfig *config.CAConfig) ([]ca.CASource, error) {
	var sources []ca.CASource

	// Phase 1 backward compatibility: ca_bundles → FileSource
	for _, path := range caConfig.CABundles {
		sources = append(sources, ca.NewFileSource(path, s.caLoader, s.logger))
	}

	// Phase 1 backward compatibility: inline_certs → InlineSource
	for _, pemData := range caConfig.InlineCerts {
		sources = append(sources, ca.NewInlineSource(pemData, s.caLoader, s.logger))
	}

	// Phase 2: ca_sources
	for _, sourceConfig := range caConfig.CASources {
		source, err := s.createCASource(ctx, &sourceConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create CA source %s: %w", sourceConfig.Type, err)
		}
		sources = append(sources, source)
	}

	return sources, nil
}

// createCASource creates a CASource from config (Phase 2)
func (s *Scanner) createCASource(ctx context.Context, cfg *config.CASourceConfig) (ca.CASource, error) {
	switch cfg.Type {
	case "file":
		return ca.NewFileSource(cfg.Path, s.caLoader, s.logger), nil

	case "configmap":
		if s.k8sClient == nil {
			return nil, fmt.Errorf("kubernetes client not available for configmap source (use NewWithK8sClient)")
		}
		// Type assert k8sClient to client.Client
		k8sClient, ok := s.k8sClient.(client.Client)
		if !ok {
			return nil, fmt.Errorf("invalid Kubernetes client type: %T (expected controller-runtime client.Client)", s.k8sClient)
		}
		return ca.NewConfigMapSource(k8sClient, cfg.Namespace, cfg.Name, cfg.Key, s.caLoader, s.logger), nil

	case "secret":
		if s.k8sClient == nil {
			return nil, fmt.Errorf("kubernetes client not available for secret source (use NewWithK8sClient)")
		}
		// Type assert k8sClient to client.Client
		k8sClient, ok := s.k8sClient.(client.Client)
		if !ok {
			return nil, fmt.Errorf("invalid Kubernetes client type: %T (expected controller-runtime client.Client)", s.k8sClient)
		}
		return ca.NewSecretSource(k8sClient, cfg.Namespace, cfg.Name, cfg.Key, s.caLoader, s.logger), nil

	case "pkcs12":
		return ca.NewPKCS12Source(cfg.Path, cfg.PasswordEnv, cfg.PasswordFile, s.caLoader, s.logger), nil

	default:
		return nil, fmt.Errorf("unknown CA source type: %s", cfg.Type)
	}
}

// Shutdown gracefully shuts down the scanner (Phase 2)
func (s *Scanner) Shutdown() {
	if s.cacheManager != nil {
		s.cacheManager.Shutdown()
	}
}

// StartWatching starts watching a CA source (Phase 2)
func (s *Scanner) StartWatching(ctx context.Context, source ca.CASource) error {
	return s.cacheManager.StartWatching(ctx, source)
}

// OnCAReload registers a callback for CA reload events (Phase 2)
func (s *Scanner) OnCAReload(callback func(sourceID string)) {
	s.cacheManager.OnReload(callback)
}

// InvalidateCache manually invalidates the cache for a specific CA source (Phase 2)
// This is useful for testing or manual cache invalidation scenarios
func (s *Scanner) InvalidateCache(sourceID string) {
	if s.cacheManager != nil {
		s.cacheManager.InvalidateCache(sourceID)
	}
}

// BuildCASources is exported for agent use (Phase 2)
func (s *Scanner) BuildCASources(ctx context.Context, caConfig *config.CAConfig) ([]ca.CASource, error) {
	return s.buildCASources(ctx, caConfig)
}
