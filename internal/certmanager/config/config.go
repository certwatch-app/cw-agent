package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the cert-manager agent
type Config struct {
	API   APIConfig   `mapstructure:"api"`
	Agent AgentConfig `mapstructure:"agent"`
	CA    *CAConfig   `mapstructure:"ca"` // Phase 2: CA validation support
}

// APIConfig holds API connection settings
type APIConfig struct {
	Endpoint string        `mapstructure:"endpoint"`
	Key      string        `mapstructure:"key"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

// AgentConfig holds agent-specific settings
type AgentConfig struct {
	Name              string        `mapstructure:"name"`
	ClusterName       string        `mapstructure:"cluster_name"` // Optional, defaults to agent.name
	LogLevel          string        `mapstructure:"log_level"`
	MetricsPort       int           `mapstructure:"metrics_port"`
	SyncInterval      time.Duration `mapstructure:"sync_interval"`
	HeartbeatInterval time.Duration `mapstructure:"heartbeat_interval"`
	WatchAllNS        bool          `mapstructure:"watch_all_namespaces"`
	Namespaces        []string      `mapstructure:"namespaces"` // If not watching all
}

// CAConfig contains Certificate Authority validation settings (Phase 2)
// Note: cert-manager controller only supports ConfigMap/Secret sources (no file paths)
type CAConfig struct {
	TrustMode      string           `mapstructure:"trust_mode"`      // system, custom, combined
	ValidationMode string           `mapstructure:"validation_mode"` // none, basic, chain
	CASources      []CASourceConfig `mapstructure:"ca_sources"`      // ConfigMap/Secret sources only
	AutoReload     *bool            `mapstructure:"auto_reload"`     // Enable hot-reload (default: true)
}

// CASourceConfig defines a CA bundle source for cert-manager
// Only ConfigMap and Secret types are supported (no file paths in containers)
type CASourceConfig struct {
	Type      string `mapstructure:"type"`      // configmap, secret
	Namespace string `mapstructure:"namespace"` // Kubernetes namespace
	Name      string `mapstructure:"name"`      // Resource name
	Key       string `mapstructure:"key"`       // Key in ConfigMap/Secret data
}

// Load loads configuration from viper
func Load(v *viper.Viper) (*Config, error) {
	setDefaults(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// ClusterName defaults to Name if not set
	if cfg.Agent.ClusterName == "" {
		cfg.Agent.ClusterName = cfg.Agent.Name
	}

	return &cfg, cfg.Validate()
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("api.endpoint", "https://api.certwatch.app")
	v.SetDefault("api.timeout", "30s")
	v.SetDefault("agent.log_level", "info")
	v.SetDefault("agent.metrics_port", 9402)
	v.SetDefault("agent.sync_interval", "30s")
	v.SetDefault("agent.heartbeat_interval", "30s")
	v.SetDefault("agent.watch_all_namespaces", true)

	// CA defaults (Phase 2)
	v.SetDefault("ca.trust_mode", "system")
	v.SetDefault("ca.validation_mode", "chain")
	v.SetDefault("ca.auto_reload", true)
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.API.Key == "" {
		return fmt.Errorf("api.key is required")
	}
	if c.Agent.Name == "" {
		return fmt.Errorf("agent.name is required")
	}
	if c.Agent.MetricsPort < 0 || c.Agent.MetricsPort > 65535 {
		return fmt.Errorf("agent.metrics_port must be between 0 and 65535")
	}
	if c.Agent.SyncInterval < 10*time.Second {
		return fmt.Errorf("agent.sync_interval must be at least 10s")
	}

	// Phase 2: Validate CA config if present
	if c.CA != nil {
		if err := c.validateCA(); err != nil {
			return fmt.Errorf("ca: %w", err)
		}
	}

	return nil
}

// validateCA validates CA configuration (Phase 2)
func (c *Config) validateCA() error {
	// Validate trust_mode
	validTrustModes := map[string]bool{
		"system": true, "custom": true, "combined": true,
	}
	if !validTrustModes[c.CA.TrustMode] {
		return fmt.Errorf("trust_mode must be one of: system, custom, combined")
	}

	// Validate validation_mode
	validValidationModes := map[string]bool{
		"none": true, "basic": true, "chain": true,
	}
	if !validValidationModes[c.CA.ValidationMode] {
		return fmt.Errorf("validation_mode must be one of: none, basic, chain")
	}

	// For custom or combined trust modes, require CA sources
	if (c.CA.TrustMode == "custom" || c.CA.TrustMode == "combined") && len(c.CA.CASources) == 0 {
		return fmt.Errorf("trust_mode '%s' requires ca_sources", c.CA.TrustMode)
	}

	// Validate CA sources
	for i, source := range c.CA.CASources {
		if err := c.validateCASource(&source); err != nil {
			return fmt.Errorf("ca_sources[%d]: %w", i, err)
		}
	}

	return nil
}

// validateCASource validates a single CA source config (Phase 2)
func (c *Config) validateCASource(source *CASourceConfig) error {
	// Only ConfigMap and Secret are supported in cert-manager controller
	validTypes := map[string]bool{
		"configmap": true, "secret": true,
	}
	if !validTypes[source.Type] {
		return fmt.Errorf("invalid type '%s' (cert-manager only supports: configmap, secret)", source.Type)
	}

	if source.Namespace == "" {
		return fmt.Errorf("namespace is required for %s source", source.Type)
	}
	if source.Name == "" {
		return fmt.Errorf("name is required for %s source", source.Type)
	}
	if source.Key == "" {
		return fmt.Errorf("key is required for %s source", source.Type)
	}

	return nil
}
