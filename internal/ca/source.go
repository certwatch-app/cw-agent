package ca

import (
	"context"
	"crypto/x509"
)

// CASource represents a source of CA certificates that can be loaded from various backends
// (files, Kubernetes ConfigMaps/Secrets, PKCS12 archives, inline PEM data).
type CASource interface {
	// Load loads CA certificates from the source.
	// Returns a slice of parsed x509 certificates or an error.
	Load(ctx context.Context) ([]*x509.Certificate, error)

	// ID returns a unique identifier for this source.
	// The ID should be stable and deterministic for the same source configuration.
	// Examples: "file:/etc/ssl/ca.pem", "configmap:default/ca-bundle#ca.crt"
	ID() string

	// Type returns the source type identifier.
	// Valid types: "file", "configmap", "secret", "pkcs12", "inline"
	Type() string
}

// CASourceConfig defines the configuration for a CA bundle source.
// This struct is used for YAML/config file deserialization.
type CASourceConfig struct {
	// Type specifies the source type: "file", "configmap", "secret", "pkcs12", "inline"
	Type string `mapstructure:"type"`

	// File source fields
	Path string `mapstructure:"path"` // Used by: file, pkcs12

	// Kubernetes source fields
	Namespace string `mapstructure:"namespace"` // Used by: configmap, secret
	Name      string `mapstructure:"name"`      // Used by: configmap, secret
	Key       string `mapstructure:"key"`       // Used by: configmap, secret (key in data map)

	// PKCS12 source fields
	PasswordEnv  string `mapstructure:"password_env"`  // Environment variable name containing password
	PasswordFile string `mapstructure:"password_file"` // File path containing password

	// Inline source field (not typically in config, used programmatically)
	PEMData string `mapstructure:"pem_data"` // Inline PEM-encoded certificate data
}
