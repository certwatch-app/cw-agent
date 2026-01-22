// Package harness provides test infrastructure for CA validation integration tests.
package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/certwatch-app/cw-agent/internal/config"
)

// ConfigBuilder provides a fluent API for building test configurations.
// It simplifies creating complex configurations for integration tests.
type ConfigBuilder struct {
	cfg *config.Config
	t   *testing.T
}

// NewConfigBuilder creates a new config builder with sensible defaults.
func NewConfigBuilder(t *testing.T) *ConfigBuilder {
	// Default config
	cfg := &config.Config{
		API: config.APIConfig{
			Endpoint: "https://api.certwatch.app",
			Key:      "cw_test_key_12345",
			Timeout:  30 * time.Second,
		},
		Agent: config.AgentConfig{
			Name:              "test-agent",
			LogLevel:          "debug",
			SyncInterval:      5 * time.Minute,
			ScanInterval:      1 * time.Minute,
			HeartbeatInterval: 30 * time.Second,
			Concurrency:       10,
			MetricsPort:       8080,
		},
		Certificates: []config.CertificateConfig{},
	}

	return &ConfigBuilder{
		cfg: cfg,
		t:   t,
	}
}

// WithAPIConfig sets the API configuration.
func (b *ConfigBuilder) WithAPIConfig(api config.APIConfig) *ConfigBuilder {
	b.cfg.API = api
	return b
}

// WithAgentConfig sets the agent configuration.
func (b *ConfigBuilder) WithAgentConfig(agent config.AgentConfig) *ConfigBuilder {
	b.cfg.Agent = agent
	return b
}

// WithGlobalCA sets the global CA configuration.
func (b *ConfigBuilder) WithGlobalCA(ca config.CAConfig) *ConfigBuilder {
	b.cfg.CA = &ca
	return b
}

// WithAutoReload enables or disables auto-reload for CA bundles.
func (b *ConfigBuilder) WithAutoReload(enabled bool) *ConfigBuilder {
	if b.cfg.CA == nil {
		b.cfg.CA = &config.CAConfig{}
	}
	b.cfg.CA.AutoReload = &enabled
	return b
}

// WithMetricsPort sets the Prometheus metrics port.
func (b *ConfigBuilder) WithMetricsPort(port int) *ConfigBuilder {
	b.cfg.Agent.MetricsPort = port
	return b
}

// AddCertificate adds a certificate configuration.
// caOverride is optional (can be nil) and allows per-certificate CA config overrides.
func (b *ConfigBuilder) AddCertificate(hostname string, port int, caOverride *config.CAConfig) *ConfigBuilder {
	cert := config.CertificateConfig{
		Hostname: hostname,
		Port:     port,
		CA:       caOverride,
	}
	b.cfg.Certificates = append(b.cfg.Certificates, cert)
	return b
}

// AddCertificateWithNotes adds a certificate configuration with notes and tags.
func (b *ConfigBuilder) AddCertificateWithNotes(hostname string, port int, notes string, tags []string, caOverride *config.CAConfig) *ConfigBuilder {
	cert := config.CertificateConfig{
		Hostname: hostname,
		Port:     port,
		Notes:    notes,
		Tags:     tags,
		CA:       caOverride,
	}
	b.cfg.Certificates = append(b.cfg.Certificates, cert)
	return b
}

// Build returns the constructed configuration.
func (b *ConfigBuilder) Build() *config.Config {
	return b.cfg
}

// WriteYAML writes the configuration to a YAML file and returns the path.
func (b *ConfigBuilder) WriteYAML(filename string) string {
	b.t.Helper()

	// Marshal to YAML
	data, err := yaml.Marshal(b.cfg)
	if err != nil {
		b.t.Fatalf("Failed to marshal config to YAML: %v", err)
	}

	// Write to temp directory
	path := filepath.Join(b.t.TempDir(), filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		b.t.Fatalf("Failed to write config file: %v", err)
	}

	return path
}

// WriteYAMLTo writes the configuration to a specific directory path.
func (b *ConfigBuilder) WriteYAMLTo(dir, filename string) string {
	b.t.Helper()

	// Marshal to YAML
	data, err := yaml.Marshal(b.cfg)
	if err != nil {
		b.t.Fatalf("Failed to marshal config to YAML: %v", err)
	}

	// Write to specified directory
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		b.t.Fatalf("Failed to write config file: %v", err)
	}

	return path
}

// QuickConfig creates a simple configuration for a single hostname with custom CA.
// This is a helper for common test scenarios.
func QuickConfig(t *testing.T, hostname string, port int, trustMode, validationMode string, caBundles []string) *config.Config {
	t.Helper()

	autoReload := true
	return &config.Config{
		API: config.APIConfig{
			Endpoint: "https://api.certwatch.app",
			Key:      "cw_test_key_12345",
			Timeout:  30 * time.Second,
		},
		Agent: config.AgentConfig{
			Name:              "test-agent",
			LogLevel:          "debug",
			SyncInterval:      5 * time.Minute,
			ScanInterval:      1 * time.Minute,
			HeartbeatInterval: 30 * time.Second,
			Concurrency:       10,
			MetricsPort:       8080,
		},
		CA: &config.CAConfig{
			TrustMode:      trustMode,
			ValidationMode: validationMode,
			CABundles:      caBundles,
			AutoReload:     &autoReload,
		},
		Certificates: []config.CertificateConfig{
			{
				Hostname: hostname,
				Port:     port,
			},
		},
	}
}

// QuickConfigWithOverride creates a configuration with global CA and per-certificate override.
func QuickConfigWithOverride(t *testing.T, hostname string, port int, globalCA *config.CAConfig, certCA *config.CAConfig) *config.Config {
	t.Helper()

	return &config.Config{
		API: config.APIConfig{
			Endpoint: "https://api.certwatch.app",
			Key:      "cw_test_key_12345",
			Timeout:  30 * time.Second,
		},
		Agent: config.AgentConfig{
			Name:              "test-agent",
			LogLevel:          "debug",
			SyncInterval:      5 * time.Minute,
			ScanInterval:      1 * time.Minute,
			HeartbeatInterval: 30 * time.Second,
			Concurrency:       10,
			MetricsPort:       8080,
		},
		CA: globalCA,
		Certificates: []config.CertificateConfig{
			{
				Hostname: hostname,
				Port:     port,
				CA:       certCA,
			},
		},
	}
}

// PrintConfig prints the configuration to stdout (useful for debugging tests).
func (b *ConfigBuilder) PrintConfig() {
	data, err := yaml.Marshal(b.cfg)
	if err != nil {
		fmt.Printf("Failed to marshal config: %v\n", err)
		return
	}
	fmt.Printf("--- Configuration ---\n%s\n", string(data))
}
