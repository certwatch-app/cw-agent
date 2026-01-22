package ca

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"go.uber.org/zap"
)

// InlineSource loads CA certificates from inline PEM data
type InlineSource struct {
	pemData string
	id      string // Generated unique ID from hash
	loader  *Loader
	logger  *zap.Logger
}

// NewInlineSource creates a new inline PEM source
func NewInlineSource(pemData string, loader *Loader, logger *zap.Logger) *InlineSource {
	// Generate ID from SHA256 of PEM data (first 8 bytes of hash)
	hash := sha256.Sum256([]byte(pemData))
	id := fmt.Sprintf("inline:%x", hash[:8])

	return &InlineSource{
		pemData: pemData,
		id:      id,
		loader:  loader,
		logger:  logger,
	}
}

// Load loads CA certificates from inline PEM data
func (s *InlineSource) Load(ctx context.Context) ([]*x509.Certificate, error) {
	s.logger.Debug("Loading CA bundle from inline PEM", zap.String("id", s.id))

	if s.pemData == "" {
		return nil, &EmptyBundleError{}
	}

	// Parse PEM data
	var certs []*x509.Certificate
	data := []byte(s.pemData)
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
			return nil, &InvalidPEMError{
				Reason: fmt.Sprintf("failed to parse certificate: %v", err),
			}
		}

		certs = append(certs, cert)
	}

	if len(certs) == 0 {
		return nil, &InvalidPEMError{
			Reason: "no valid certificate blocks found",
		}
	}

	s.logger.Info("Loaded CA bundle from inline PEM",
		zap.String("source", s.ID()),
		zap.Int("certificates", len(certs)))

	return certs, nil
}

// ID returns a unique identifier for this source
func (s *InlineSource) ID() string {
	return s.id
}

// Type returns the source type
func (s *InlineSource) Type() string {
	return "inline"
}
