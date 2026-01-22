package ca

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"

	"go.uber.org/zap"
)

// FileSource loads CA certificates from a file on disk
type FileSource struct {
	path   string
	loader *Loader
	logger *zap.Logger
}

// NewFileSource creates a new file-based CA source
func NewFileSource(path string, loader *Loader, logger *zap.Logger) *FileSource {
	return &FileSource{
		path:   path,
		loader: loader,
		logger: logger,
	}
}

// Load loads CA certificates from the file
func (s *FileSource) Load(ctx context.Context) ([]*x509.Certificate, error) {
	s.logger.Debug("Loading CA bundle from file", zap.String("path", s.path))

	// Validate file before loading (TOCTOU protection)
	if err := s.loader.validateCABundle(s.path); err != nil {
		return nil, err
	}

	// Read file
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &FileNotFoundError{Path: s.path}
		}
		return nil, fmt.Errorf("failed to read CA bundle: %w", err)
	}

	// Parse PEM data
	var certs []*x509.Certificate
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
				Path:   s.path,
				Reason: fmt.Sprintf("failed to parse certificate: %v", err),
			}
		}

		certs = append(certs, cert)
	}

	if len(certs) == 0 {
		return nil, &EmptyBundleError{Path: s.path}
	}

	s.logger.Info("Loaded CA bundle from file",
		zap.String("source", s.ID()),
		zap.Int("certificates", len(certs)))

	return certs, nil
}

// ID returns a unique identifier for this source
func (s *FileSource) ID() string {
	return fmt.Sprintf("file:%s", s.path)
}

// Type returns the source type
func (s *FileSource) Type() string {
	return "file"
}

// GetPath returns the file path (used by watcher)
func (s *FileSource) GetPath() string {
	return s.path
}
