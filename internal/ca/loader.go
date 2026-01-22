// Package ca provides Certificate Authority bundle loading and validation.
package ca

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"

	"go.uber.org/zap"
)

// Loader handles loading and managing CA certificate pools
type Loader struct {
	logger *zap.Logger
}

// NewLoader creates a new CA loader
func NewLoader(logger *zap.Logger) *Loader {
	return &Loader{
		logger: logger,
	}
}

// LoadCertPool loads a certificate pool based on the specified configuration
// trustMode: "system", "custom", or "combined"
// caBundles: paths to CA bundle files (for custom/combined modes)
// inlineCerts: inline PEM-encoded certificates (for custom/combined modes)
func (l *Loader) LoadCertPool(trustMode string, caBundles []string, inlineCerts []string) (*x509.CertPool, error) {
	l.logger.Debug("Loading CA cert pool",
		zap.String("trust_mode", trustMode),
		zap.Int("ca_bundle_count", len(caBundles)),
		zap.Int("inline_cert_count", len(inlineCerts)))

	var pool *x509.CertPool
	var err error

	switch trustMode {
	case "system":
		pool, err = l.loadSystemCAs()
		if err != nil {
			return nil, fmt.Errorf("failed to load system CAs: %w", err)
		}

	case "custom":
		pool = x509.NewCertPool()
		// Load custom CAs from files and inline certs
		if err := l.loadCustomCAs(pool, caBundles, inlineCerts); err != nil {
			return nil, fmt.Errorf("failed to load custom CAs: %w", err)
		}

	case "combined":
		// Start with system CAs
		pool, err = l.loadSystemCAs()
		if err != nil {
			l.logger.Warn("Failed to load system CAs for combined mode, continuing with empty pool", zap.Error(err))
			pool = x509.NewCertPool()
		}
		// Add custom CAs on top
		if err := l.loadCustomCAs(pool, caBundles, inlineCerts); err != nil {
			return nil, fmt.Errorf("failed to load custom CAs in combined mode: %w", err)
		}

	default:
		return nil, fmt.Errorf("invalid trust_mode: %s", trustMode)
	}

	l.logger.Info("CA cert pool loaded successfully",
		zap.String("trust_mode", trustMode))

	return pool, nil
}

// loadSystemCAs loads the system's trusted CA certificates
func (l *Loader) loadSystemCAs() (*x509.CertPool, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSystemCAUnavailable, err)
	}

	if pool == nil {
		return nil, ErrSystemCAUnavailable
	}

	l.logger.Debug("System CAs loaded successfully")
	return pool, nil
}

// loadCustomCAs loads custom CA certificates from files and inline certs into the pool
func (l *Loader) loadCustomCAs(pool *x509.CertPool, caBundles []string, inlineCerts []string) error {
	certCount := 0

	// Load from files
	for _, bundlePath := range caBundles {
		count, err := l.loadPEMFile(pool, bundlePath)
		if err != nil {
			return fmt.Errorf("failed to load CA bundle %s: %w", bundlePath, err)
		}
		certCount += count
		l.logger.Debug("Loaded CA bundle",
			zap.String("path", bundlePath),
			zap.Int("cert_count", count))
	}

	// Load inline certificates
	for i, certPEM := range inlineCerts {
		count, err := l.parseInlineCertificates(pool, certPEM)
		if err != nil {
			return fmt.Errorf("failed to parse inline_cert[%d]: %w", i, err)
		}
		certCount += count
		l.logger.Debug("Loaded inline certificate",
			zap.Int("index", i),
			zap.Int("cert_count", count))
	}

	if certCount == 0 {
		return &EmptyBundleError{}
	}

	l.logger.Info("Custom CAs loaded",
		zap.Int("total_certificates", certCount))

	return nil
}

// loadPEMFile loads CA certificates from a PEM file with security validation
func (l *Loader) loadPEMFile(pool *x509.CertPool, path string) (int, error) {
	// Validate file before loading (TOCTOU protection)
	if err := l.validateCABundle(path); err != nil {
		return 0, err
	}

	// Read file
	//nolint:gosec // G304: File path from config, validated by validateCABundle (symlink/permission checks)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, &FileNotFoundError{Path: path}
		}
		return 0, fmt.Errorf("failed to read CA bundle: %w", err)
	}

	// Parse PEM data
	count := 0
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}

		if block.Type != "CERTIFICATE" {
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return 0, &InvalidPEMError{
				Path:   path,
				Reason: fmt.Sprintf("failed to parse certificate: %v", err),
			}
		}

		pool.AddCert(cert)
		count++
	}

	if count == 0 {
		return 0, &EmptyBundleError{Path: path}
	}

	return count, nil
}

// parseInlineCertificates parses inline PEM-encoded certificates
func (l *Loader) parseInlineCertificates(pool *x509.CertPool, certPEM string) (int, error) {
	if certPEM == "" {
		return 0, &EmptyBundleError{}
	}

	data := []byte(certPEM)
	count := 0
	rest := data

	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}

		if block.Type != "CERTIFICATE" {
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return 0, &InvalidPEMError{
				Reason: fmt.Sprintf("failed to parse certificate: %v", err),
			}
		}

		pool.AddCert(cert)
		count++
	}

	if count == 0 {
		return 0, &InvalidPEMError{
			Reason: "no valid certificate blocks found",
		}
	}

	return count, nil
}

// validateCABundle validates a CA bundle file for security issues
// This provides TOCTOU (Time-of-Check-Time-of-Use) protection
func (l *Loader) validateCABundle(path string) error {
	info, err := os.Lstat(path) // Use Lstat to detect symlinks
	if err != nil {
		if os.IsNotExist(err) {
			return &FileNotFoundError{Path: path}
		}
		return fmt.Errorf("failed to stat CA bundle: %w", err)
	}

	// Reject symlinks
	if info.Mode()&os.ModeSymlink != 0 {
		return &InvalidFileTypeError{
			Path: path,
			Type: "symlink",
		}
	}

	// Reject directories
	if info.IsDir() {
		return &InvalidFileTypeError{
			Path: path,
			Type: "directory",
		}
	}

	// Check file permissions
	mode := info.Mode().Perm()

	// Reject world-writable files (security risk)
	if mode&0o002 != 0 {
		return &InsecurePermissionsError{
			Path:        path,
			Permissions: fmt.Sprintf("%04o", mode),
		}
	}

	// Warn on group-writable files
	if mode&0o020 != 0 {
		l.logger.Warn("CA bundle is group-writable, consider restricting permissions",
			zap.String("path", path),
			zap.String("permissions", fmt.Sprintf("%04o", mode)))
	}

	return nil
}
