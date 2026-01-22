package ca

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SecretSource loads CA certificates from a Kubernetes Secret
type SecretSource struct {
	client    client.Client
	namespace string
	name      string
	key       string
	loader    *Loader
	logger    *zap.Logger
}

// NewSecretSource creates a new Secret-based CA source
func NewSecretSource(client client.Client, namespace, name, key string, loader *Loader, logger *zap.Logger) *SecretSource {
	return &SecretSource{
		client:    client,
		namespace: namespace,
		name:      name,
		key:       key,
		loader:    loader,
		logger:    logger,
	}
}

// Load loads CA certificates from the Secret
func (s *SecretSource) Load(ctx context.Context) ([]*x509.Certificate, error) {
	s.logger.Debug("Loading CA bundle from Secret",
		zap.String("namespace", s.namespace),
		zap.String("name", s.name),
		zap.String("key", s.key))

	// Fetch Secret
	secret := &corev1.Secret{}
	err := s.client.Get(ctx, client.ObjectKey{
		Namespace: s.namespace,
		Name:      s.name,
	}, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Secret %s/%s: %w", s.namespace, s.name, err)
	}

	// Extract PEM data from key (Secret.Data is []byte, not string)
	pemBytes, ok := secret.Data[s.key]
	if !ok {
		return nil, fmt.Errorf("key %s not found in Secret %s/%s", s.key, s.namespace, s.name)
	}

	if len(pemBytes) == 0 {
		return nil, fmt.Errorf("key %s in Secret %s/%s is empty", s.key, s.namespace, s.name)
	}

	// Parse PEM data
	var certs []*x509.Certificate
	rest := pemBytes

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
				Reason: fmt.Sprintf("failed to parse certificate from Secret: %v", err),
			}
		}

		certs = append(certs, cert)
	}

	if len(certs) == 0 {
		return nil, &InvalidPEMError{
			Reason: "no valid certificate blocks found in Secret",
		}
	}

	s.logger.Info("Loaded CA bundle from Secret",
		zap.String("source", s.ID()),
		zap.Int("certificates", len(certs)))

	return certs, nil
}

// ID returns a unique identifier for this source
func (s *SecretSource) ID() string {
	return fmt.Sprintf("secret:%s/%s#%s", s.namespace, s.name, s.key)
}

// Type returns the source type
func (s *SecretSource) Type() string {
	return "secret"
}

// GetClient returns the Kubernetes client (used by watcher)
func (s *SecretSource) GetClient() interface{} {
	return s.client
}

// GetNamespace returns the namespace (used by watcher)
func (s *SecretSource) GetNamespace() string {
	return s.namespace
}

// GetName returns the Secret name (used by watcher)
func (s *SecretSource) GetName() string {
	return s.name
}
