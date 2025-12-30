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
	Certificates []CertificateConfig `mapstructure:"certificates"`
}

// APIConfig contains API connection settings
type APIConfig struct {
	Endpoint string        `mapstructure:"endpoint"`
	Key      string        `mapstructure:"key"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

// AgentConfig contains agent behavior settings
// Fields are ordered for optimal memory alignment
type AgentConfig struct {
	Name         string        `mapstructure:"name"`
	LogLevel     string        `mapstructure:"log_level"`
	SyncInterval time.Duration `mapstructure:"sync_interval"`
	ScanInterval time.Duration `mapstructure:"scan_interval"`
	Concurrency  int           `mapstructure:"concurrency"`
}

// CertificateConfig represents a certificate to monitor
// Fields are ordered for optimal memory alignment
type CertificateConfig struct {
	Hostname string   `mapstructure:"hostname"`
	Notes    string   `mapstructure:"notes"`
	Tags     []string `mapstructure:"tags"`
	Port     int      `mapstructure:"port"`
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

	// Agent defaults
	v.SetDefault("agent.name", "default-agent")
	v.SetDefault("agent.sync_interval", "5m")
	v.SetDefault("agent.scan_interval", "1m")
	v.SetDefault("agent.concurrency", 10)
	v.SetDefault("agent.log_level", "info")
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
