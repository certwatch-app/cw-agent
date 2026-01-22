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

// ConfigMapSource loads CA certificates from a Kubernetes ConfigMap
type ConfigMapSource struct {
	client    client.Client
	namespace string
	name      string
	key       string
	loader    *Loader
	logger    *zap.Logger
}

// NewConfigMapSource creates a new ConfigMap-based CA source
func NewConfigMapSource(client client.Client, namespace, name, key string, loader *Loader, logger *zap.Logger) *ConfigMapSource {
	return &ConfigMapSource{
		client:    client,
		namespace: namespace,
		name:      name,
		key:       key,
		loader:    loader,
		logger:    logger,
	}
}

// Load loads CA certificates from the ConfigMap
func (s *ConfigMapSource) Load(ctx context.Context) ([]*x509.Certificate, error) {
	s.logger.Debug("Loading CA bundle from ConfigMap",
		zap.String("namespace", s.namespace),
		zap.String("name", s.name),
		zap.String("key", s.key))

	// Fetch ConfigMap
	cm := &corev1.ConfigMap{}
	err := s.client.Get(ctx, client.ObjectKey{
		Namespace: s.namespace,
		Name:      s.name,
	}, cm)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ConfigMap %s/%s: %w", s.namespace, s.name, err)
	}

	// Extract PEM data from key
	pemData, ok := cm.Data[s.key]
	if !ok {
		return nil, fmt.Errorf("key %s not found in ConfigMap %s/%s", s.key, s.namespace, s.name)
	}

	if pemData == "" {
		return nil, fmt.Errorf("key %s in ConfigMap %s/%s is empty", s.key, s.namespace, s.name)
	}

	// Parse PEM data
	var certs []*x509.Certificate
	data := []byte(pemData)
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
				Reason: fmt.Sprintf("failed to parse certificate from ConfigMap: %v", err),
			}
		}

		certs = append(certs, cert)
	}

	if len(certs) == 0 {
		return nil, &InvalidPEMError{
			Reason: "no valid certificate blocks found in ConfigMap",
		}
	}

	s.logger.Info("Loaded CA bundle from ConfigMap",
		zap.String("source", s.ID()),
		zap.Int("certificates", len(certs)))

	return certs, nil
}

// ID returns a unique identifier for this source
func (s *ConfigMapSource) ID() string {
	return fmt.Sprintf("configmap:%s/%s#%s", s.namespace, s.name, s.key)
}

// Type returns the source type
func (s *ConfigMapSource) Type() string {
	return "configmap"
}

// GetClient returns the Kubernetes client (used by watcher)
func (s *ConfigMapSource) GetClient() interface{} {
	return s.client
}

// GetNamespace returns the namespace (used by watcher)
func (s *ConfigMapSource) GetNamespace() string {
	return s.namespace
}

// GetName returns the ConfigMap name (used by watcher)
func (s *ConfigMapSource) GetName() string {
	return s.name
}
