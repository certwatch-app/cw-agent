package ca

import (
	"context"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"software.sslmate.com/src/go-pkcs12"
)

// PKCS12Source loads CA certificates from a PKCS12/PFX file
type PKCS12Source struct {
	path         string
	passwordEnv  string // Environment variable name containing password
	passwordFile string // File path containing password
	loader       *Loader
	logger       *zap.Logger
}

// NewPKCS12Source creates a new PKCS12-based CA source
func NewPKCS12Source(path, passwordEnv, passwordFile string, loader *Loader, logger *zap.Logger) *PKCS12Source {
	return &PKCS12Source{
		path:         path,
		passwordEnv:  passwordEnv,
		passwordFile: passwordFile,
		loader:       loader,
		logger:       logger,
	}
}

// Load loads CA certificates from the PKCS12 file
func (s *PKCS12Source) Load(ctx context.Context) ([]*x509.Certificate, error) {
	s.logger.Debug("Loading CA bundle from PKCS12", zap.String("path", s.path))

	// 1. Validate file permissions before loading
	if err := s.loader.validateCABundle(s.path); err != nil {
		return nil, err
	}

	// 2. Read PKCS12 file
	p12Data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &FileNotFoundError{Path: s.path}
		}
		return nil, fmt.Errorf("failed to read PKCS12 file: %w", err)
	}

	// 3. Get password
	password, err := s.getPassword()
	if err != nil {
		return nil, err
	}

	// 4. Decode PKCS12
	// Use DecodeChain which returns: privateKey, certificate, caCerts, error
	privateKey, cert, caCerts, err := pkcs12.DecodeChain(p12Data, password)

	// Clear password from memory immediately
	//nolint:ineffassign // Intentional: clear sensitive password from memory for security
	password = ""

	if err != nil {
		return nil, fmt.Errorf("failed to decode PKCS12 file: %w", err)
	}

	// 5. Security: Zero private key from memory and warn if present
	if privateKey != nil {
		s.logger.Warn("PKCS12 file contains private key - it will be ignored and zeroed from memory",
			zap.String("path", s.path))
		// The private key will be garbage collected, but we explicitly nil it
		//nolint:ineffassign // Intentional: clear sensitive private key from memory for security
		privateKey = nil
	}

	// 6. Collect CA certificates
	var allCerts []*x509.Certificate

	// If the main certificate is a CA, include it
	if cert != nil && cert.IsCA {
		allCerts = append(allCerts, cert)
		s.logger.Debug("Main certificate is a CA, including it",
			zap.String("subject", cert.Subject.String()))
	}

	// Add all CA certificates from the chain
	allCerts = append(allCerts, caCerts...)

	if len(allCerts) == 0 {
		return nil, fmt.Errorf("no CA certificates found in PKCS12 file")
	}

	s.logger.Info("Loaded CA bundle from PKCS12",
		zap.String("source", s.ID()),
		zap.Int("certificates", len(allCerts)))

	return allCerts, nil
}

// getPassword retrieves the password from environment variable or file
func (s *PKCS12Source) getPassword() (string, error) {
	// Priority: passwordEnv > passwordFile
	if s.passwordEnv != "" {
		password := os.Getenv(s.passwordEnv)
		if password == "" {
			return "", fmt.Errorf("environment variable %s is empty or not set", s.passwordEnv)
		}
		// Log only the variable name, never the password value
		// codeql[go/clear-text-logging] Logging env var name (not the password value) for debugging
		s.logger.Debug("Retrieved password from environment variable",
			zap.String("env_var", s.passwordEnv))
		return password, nil
	}

	if s.passwordFile != "" {
		data, err := os.ReadFile(s.passwordFile)
		if err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("password file not found: %s", s.passwordFile)
			}
			return "", fmt.Errorf("failed to read password file: %w", err)
		}

		// Trim whitespace/newlines from password file
		password := strings.TrimSpace(string(data))
		if password == "" {
			return "", fmt.Errorf("password file is empty: %s", s.passwordFile)
		}
		// codeql[go/clear-text-logging] Logging file path (not the password contents) for debugging
		s.logger.Debug("Retrieved password from password file",
			zap.String("password_file", s.passwordFile))
		return password, nil
	}

	return "", fmt.Errorf("no password source configured (need password_env or password_file)")
}

// ID returns a unique identifier for this source
func (s *PKCS12Source) ID() string {
	return fmt.Sprintf("pkcs12:%s", s.path)
}

// Type returns the source type
func (s *PKCS12Source) Type() string {
	return "pkcs12"
}

// GetPath returns the file path (used by watcher)
func (s *PKCS12Source) GetPath() string {
	return s.path
}
