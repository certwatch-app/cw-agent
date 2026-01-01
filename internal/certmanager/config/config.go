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
	return nil
}
