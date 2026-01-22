// Package config handles configuration loading and validation for the CertWatch Agent.
package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config represents the complete agent configuration
type Config struct {
	API          APIConfig           `mapstructure:"api"`
	Agent        AgentConfig         `mapstructure:"agent"`
	CA           *CAConfig           `mapstructure:"ca"`
	Certificates []CertificateConfig `mapstructure:"certificates"`
}

// APIConfig contains API connection settings
type APIConfig struct {
	Endpoint       string               `mapstructure:"endpoint"`
	Key            string               `mapstructure:"key"`
	Timeout        time.Duration        `mapstructure:"timeout"`
	Retry          RetryConfig          `mapstructure:"retry"`
	CircuitBreaker CircuitBreakerConfig `mapstructure:"circuit_breaker"`
}

// RetryConfig contains retry behavior settings
type RetryConfig struct {
	MaxRetries     int           `mapstructure:"max_retries"`
	InitialBackoff time.Duration `mapstructure:"initial_backoff"`
	MaxBackoff     time.Duration `mapstructure:"max_backoff"`
	Multiplier     float64       `mapstructure:"multiplier"`
}

// CircuitBreakerConfig contains circuit breaker settings
type CircuitBreakerConfig struct {
	Enabled     bool          `mapstructure:"enabled"`
	MaxFailures int           `mapstructure:"max_failures"`
	Timeout     time.Duration `mapstructure:"timeout"`
}

// AgentConfig contains agent behavior settings
// Fields are ordered for optimal memory alignment
type AgentConfig struct {
	Name              string        `mapstructure:"name"`
	LogLevel          string        `mapstructure:"log_level"`
	SyncInterval      time.Duration `mapstructure:"sync_interval"`
	ScanInterval      time.Duration `mapstructure:"scan_interval"`
	HeartbeatInterval time.Duration `mapstructure:"heartbeat_interval"`
	Concurrency       int           `mapstructure:"concurrency"`
	MetricsPort       int           `mapstructure:"metrics_port"`
}

// CAConfig contains Certificate Authority validation settings
type CAConfig struct {
	TrustMode      string           `mapstructure:"trust_mode"`      // system, custom, combined
	ValidationMode string           `mapstructure:"validation_mode"` // none, basic, chain
	CABundles      []string         `mapstructure:"ca_bundles"`      // paths to CA bundle files (Phase 1, backward compat)
	InlineCerts    []string         `mapstructure:"inline_certs"`    // inline PEM-encoded CA certificates (Phase 1, backward compat)
	CASources      []CASourceConfig `mapstructure:"ca_sources"`      // Phase 2: configurable CA sources
	AutoReload     *bool            `mapstructure:"auto_reload"`     // Phase 2: enable hot-reload (default: true)
	ReloadInterval time.Duration    `mapstructure:"reload_interval"` // Phase 2: fallback polling interval (default: 0 = watch only)
}

// CASourceConfig defines a CA bundle source (Phase 2)
type CASourceConfig struct {
	Type         string `mapstructure:"type"`          // file, configmap, secret, pkcs12
	Path         string `mapstructure:"path"`          // For file, pkcs12
	Namespace    string `mapstructure:"namespace"`     // For configmap, secret
	Name         string `mapstructure:"name"`          // For configmap, secret
	Key          string `mapstructure:"key"`           // For configmap, secret
	PasswordEnv  string `mapstructure:"password_env"`  // For pkcs12
	PasswordFile string `mapstructure:"password_file"` // For pkcs12
}

// CertificateConfig represents a certificate to monitor
// Fields are ordered for optimal memory alignment
type CertificateConfig struct {
	Hostname string    `mapstructure:"hostname"`
	Notes    string    `mapstructure:"notes"`
	Tags     []string  `mapstructure:"tags"`
	CA       *CAConfig `mapstructure:"ca"` // per-certificate CA override
	Port     int       `mapstructure:"port"`
}

// Load reads configuration from viper
func Load(v *viper.Viper) (*Config, error) {
	// Set defaults
	setDefaults(v)

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Apply defaults for certificate ports
	for i := range cfg.Certificates {
		if cfg.Certificates[i].Port == 0 {
			cfg.Certificates[i].Port = 443
		}
	}

	return cfg, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// API defaults
	v.SetDefault("api.endpoint", "https://api.certwatch.app")
	v.SetDefault("api.timeout", "30s")
	v.SetDefault("api.retry.max_retries", 3)
	v.SetDefault("api.retry.initial_backoff", "1s")
	v.SetDefault("api.retry.max_backoff", "30s")
	v.SetDefault("api.retry.multiplier", 2.0)
	v.SetDefault("api.circuit_breaker.enabled", true)
	v.SetDefault("api.circuit_breaker.max_failures", 5)
	v.SetDefault("api.circuit_breaker.timeout", "60s")

	// Agent defaults
	v.SetDefault("agent.name", "default-agent")
	v.SetDefault("agent.sync_interval", "5m")
	v.SetDefault("agent.scan_interval", "1m")
	v.SetDefault("agent.heartbeat_interval", "30s")
	v.SetDefault("agent.concurrency", 10)
	v.SetDefault("agent.log_level", "info")
	v.SetDefault("agent.metrics_port", 8080)

	// CA defaults (only if ca section exists)
	v.SetDefault("ca.trust_mode", "system")
	v.SetDefault("ca.validation_mode", "chain")
	v.SetDefault("ca.auto_reload", true)     // Phase 2: enable hot-reload by default
	v.SetDefault("ca.reload_interval", "0s") // Phase 2: watch-only, no polling
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate API config
	if err := c.validateAPI(); err != nil {
		return fmt.Errorf("api: %w", err)
	}

	// Validate agent config
	if err := c.validateAgent(); err != nil {
		return fmt.Errorf("agent: %w", err)
	}

	// Validate CA config (if present)
	if c.CA != nil {
		if err := c.validateCA(c.CA); err != nil {
			return fmt.Errorf("ca: %w", err)
		}
	}

	// Validate certificates
	if err := c.validateCertificates(); err != nil {
		return fmt.Errorf("certificates: %w", err)
	}

	return nil
}

func (c *Config) validateAPI() error {
	if c.API.Endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}

	u, err := url.Parse(c.API.Endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("endpoint must use http or https scheme")
	}

	if c.API.Key == "" {
		return fmt.Errorf("key is required")
	}

	if !strings.HasPrefix(c.API.Key, "cw_") {
		return fmt.Errorf("key must start with 'cw_' prefix")
	}

	if c.API.Timeout < time.Second {
		return fmt.Errorf("timeout must be at least 1 second")
	}

	// Validate retry config
	if c.API.Retry.MaxRetries < 0 || c.API.Retry.MaxRetries > 10 {
		return fmt.Errorf("retry.max_retries must be between 0 and 10")
	}

	if c.API.Retry.MaxRetries > 0 {
		if c.API.Retry.InitialBackoff < 100*time.Millisecond {
			return fmt.Errorf("retry.initial_backoff must be at least 100ms")
		}

		if c.API.Retry.MaxBackoff < c.API.Retry.InitialBackoff {
			return fmt.Errorf("retry.max_backoff must be greater than initial_backoff")
		}

		if c.API.Retry.Multiplier < 1.0 || c.API.Retry.Multiplier > 10.0 {
			return fmt.Errorf("retry.multiplier must be between 1.0 and 10.0")
		}
	}

	// Validate circuit breaker config
	if c.API.CircuitBreaker.Enabled {
		if c.API.CircuitBreaker.MaxFailures < 1 || c.API.CircuitBreaker.MaxFailures > 20 {
			return fmt.Errorf("circuit_breaker.max_failures must be between 1 and 20")
		}

		if c.API.CircuitBreaker.Timeout < 10*time.Second {
			return fmt.Errorf("circuit_breaker.timeout must be at least 10 seconds")
		}
	}

	return nil
}

func (c *Config) validateAgent() error {
	if c.Agent.Name == "" {
		return fmt.Errorf("name is required")
	}

	if len(c.Agent.Name) > 100 {
		return fmt.Errorf("name must be at most 100 characters")
	}

	if c.Agent.SyncInterval < 30*time.Second {
		return fmt.Errorf("sync_interval must be at least 30 seconds")
	}

	if c.Agent.ScanInterval < 10*time.Second {
		return fmt.Errorf("scan_interval must be at least 10 seconds")
	}

	if c.Agent.Concurrency < 1 || c.Agent.Concurrency > 50 {
		return fmt.Errorf("concurrency must be between 1 and 50")
	}

	validLogLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true,
	}
	if !validLogLevels[c.Agent.LogLevel] {
		return fmt.Errorf("log_level must be one of: debug, info, warn, error")
	}

	// HeartbeatInterval of 0 means disabled, otherwise must be at least 10s
	if c.Agent.HeartbeatInterval != 0 && c.Agent.HeartbeatInterval < 10*time.Second {
		return fmt.Errorf("heartbeat_interval must be at least 10 seconds (or 0 to disable)")
	}

	// MetricsPort of 0 means disabled, otherwise must be valid port
	if c.Agent.MetricsPort != 0 && (c.Agent.MetricsPort < 1 || c.Agent.MetricsPort > 65535) {
		return fmt.Errorf("metrics_port must be between 1 and 65535 (or 0 to disable)")
	}

	return nil
}

func (c *Config) validateCA(ca *CAConfig) error {
	// Validate trust_mode
	validTrustModes := map[string]bool{
		"system":   true,
		"custom":   true,
		"combined": true,
	}
	if !validTrustModes[ca.TrustMode] {
		return fmt.Errorf("trust_mode must be one of: system, custom, combined")
	}

	// Validate validation_mode
	validValidationModes := map[string]bool{
		"none":  true,
		"basic": true,
		"chain": true,
	}
	if !validValidationModes[ca.ValidationMode] {
		return fmt.Errorf("validation_mode must be one of: none, basic, chain")
	}

	// For custom or combined trust modes, require CA bundles, inline certs, or ca_sources
	if (ca.TrustMode == "custom" || ca.TrustMode == "combined") &&
		len(ca.CABundles) == 0 && len(ca.InlineCerts) == 0 && len(ca.CASources) == 0 {
		return fmt.Errorf("trust_mode '%s' requires ca_bundles, inline_certs, or ca_sources", ca.TrustMode)
	}

	// Validate CA bundle paths are not empty
	for i, bundle := range ca.CABundles {
		if bundle == "" {
			return fmt.Errorf("ca_bundles[%d]: path cannot be empty", i)
		}
	}

	// Validate inline certs are not empty
	for i, cert := range ca.InlineCerts {
		if cert == "" {
			return fmt.Errorf("inline_certs[%d]: certificate cannot be empty", i)
		}
	}

	// Phase 2: Validate CA sources
	for i, source := range ca.CASources {
		if err := c.validateCASource(&source); err != nil {
			return fmt.Errorf("ca_sources[%d]: %w", i, err)
		}
	}

	// Phase 2: Validate reload_interval (if non-zero, must be at least 1 minute)
	if ca.ReloadInterval != 0 && ca.ReloadInterval < time.Minute {
		return fmt.Errorf("reload_interval must be at least 1 minute (or 0 for watch-only)")
	}

	return nil
}

func (c *Config) validateCASource(source *CASourceConfig) error {
	validTypes := map[string]bool{
		"file": true, "configmap": true, "secret": true, "pkcs12": true,
	}
	if !validTypes[source.Type] {
		return fmt.Errorf("invalid type '%s' (must be: file, configmap, secret, pkcs12)", source.Type)
	}

	switch source.Type {
	case "file":
		if source.Path == "" {
			return fmt.Errorf("path is required for file source")
		}

	case "configmap", "secret":
		if source.Namespace == "" {
			return fmt.Errorf("namespace is required for %s source", source.Type)
		}
		if source.Name == "" {
			return fmt.Errorf("name is required for %s source", source.Type)
		}
		if source.Key == "" {
			return fmt.Errorf("key is required for %s source", source.Type)
		}

	case "pkcs12":
		if source.Path == "" {
			return fmt.Errorf("path is required for pkcs12 source")
		}
		if source.PasswordEnv == "" && source.PasswordFile == "" {
			return fmt.Errorf("password_env or password_file is required for pkcs12 source")
		}
		if source.PasswordEnv != "" && source.PasswordFile != "" {
			return fmt.Errorf("specify only one of password_env or password_file, not both")
		}
	}

	return nil
}

func (c *Config) validateCertificates() error {
	if len(c.Certificates) == 0 {
		return fmt.Errorf("at least one certificate is required")
	}

	if len(c.Certificates) > 1000 {
		return fmt.Errorf("maximum 1000 certificates allowed")
	}

	seen := make(map[string]bool)
	for i, cert := range c.Certificates {
		if cert.Hostname == "" {
			return fmt.Errorf("[%d]: hostname is required", i)
		}

		if cert.Port < 1 || cert.Port > 65535 {
			return fmt.Errorf("[%d]: port must be between 1 and 65535", i)
		}

		key := fmt.Sprintf("%s:%d", cert.Hostname, cert.Port)
		if seen[key] {
			return fmt.Errorf("[%d]: duplicate hostname:port '%s'", i, key)
		}
		seen[key] = true

		// Validate per-certificate CA override if present
		if cert.CA != nil {
			if err := c.validateCA(cert.CA); err != nil {
				return fmt.Errorf("[%d]: ca: %w", i, err)
			}
		}

		for j, tag := range cert.Tags {
			if len(tag) > 50 {
				return fmt.Errorf("[%d]: tag[%d] must be at most 50 characters", i, j)
			}
		}

		if len(cert.Notes) > 500 {
			return fmt.Errorf("[%d]: notes must be at most 500 characters", i)
		}
	}

	return nil
}

// GetHostPort returns the hostname:port string for a certificate config
func (c *CertificateConfig) GetHostPort() string {
	return fmt.Sprintf("%s:%d", c.Hostname, c.Port)
}
