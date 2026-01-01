package config

import (
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestLoad_Defaults(t *testing.T) {
	v := viper.New()
	v.Set("api.key", "test-key")
	v.Set("agent.name", "test-agent")

	cfg, err := Load(v)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Check defaults
	if cfg.API.Endpoint != "https://api.certwatch.app" {
		t.Errorf("API.Endpoint = %v, want https://api.certwatch.app", cfg.API.Endpoint)
	}
	if cfg.API.Timeout != 30*time.Second {
		t.Errorf("API.Timeout = %v, want 30s", cfg.API.Timeout)
	}
	if cfg.Agent.LogLevel != "info" {
		t.Errorf("Agent.LogLevel = %v, want info", cfg.Agent.LogLevel)
	}
	if cfg.Agent.MetricsPort != 9402 {
		t.Errorf("Agent.MetricsPort = %v, want 9402", cfg.Agent.MetricsPort)
	}
	if cfg.Agent.SyncInterval != 30*time.Second {
		t.Errorf("Agent.SyncInterval = %v, want 30s", cfg.Agent.SyncInterval)
	}
	if !cfg.Agent.WatchAllNS {
		t.Error("Agent.WatchAllNS = false, want true")
	}
	if cfg.Agent.HeartbeatInterval != 30*time.Second {
		t.Errorf("Agent.HeartbeatInterval = %v, want 30s", cfg.Agent.HeartbeatInterval)
	}
}

func TestLoad_ClusterNameDefaultsToAgentName(t *testing.T) {
	v := viper.New()
	v.Set("api.key", "test-key")
	v.Set("agent.name", "my-agent")

	cfg, err := Load(v)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Agent.ClusterName != "my-agent" {
		t.Errorf("Agent.ClusterName = %v, want my-agent", cfg.Agent.ClusterName)
	}
}

func TestLoad_ClusterNameOverride(t *testing.T) {
	v := viper.New()
	v.Set("api.key", "test-key")
	v.Set("agent.name", "my-agent")
	v.Set("agent.cluster_name", "my-cluster")

	cfg, err := Load(v)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Agent.ClusterName != "my-cluster" {
		t.Errorf("Agent.ClusterName = %v, want my-cluster", cfg.Agent.ClusterName)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	v := viper.New()
	v.Set("api.endpoint", "https://custom.api.com")
	v.Set("api.key", "custom-key")
	v.Set("api.timeout", "60s")
	v.Set("agent.name", "custom-agent")
	v.Set("agent.log_level", "debug")
	v.Set("agent.metrics_port", 9500)
	v.Set("agent.sync_interval", "1m")
	v.Set("agent.heartbeat_interval", "15s")
	v.Set("agent.watch_all_namespaces", false)
	v.Set("agent.namespaces", []string{"default", "production"})

	cfg, err := Load(v)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.API.Endpoint != "https://custom.api.com" {
		t.Errorf("API.Endpoint = %v, want https://custom.api.com", cfg.API.Endpoint)
	}
	if cfg.API.Key != "custom-key" {
		t.Errorf("API.Key = %v, want custom-key", cfg.API.Key)
	}
	if cfg.API.Timeout != 60*time.Second {
		t.Errorf("API.Timeout = %v, want 60s", cfg.API.Timeout)
	}
	if cfg.Agent.Name != "custom-agent" {
		t.Errorf("Agent.Name = %v, want custom-agent", cfg.Agent.Name)
	}
	if cfg.Agent.LogLevel != "debug" {
		t.Errorf("Agent.LogLevel = %v, want debug", cfg.Agent.LogLevel)
	}
	if cfg.Agent.MetricsPort != 9500 {
		t.Errorf("Agent.MetricsPort = %v, want 9500", cfg.Agent.MetricsPort)
	}
	if cfg.Agent.SyncInterval != time.Minute {
		t.Errorf("Agent.SyncInterval = %v, want 1m", cfg.Agent.SyncInterval)
	}
	if cfg.Agent.HeartbeatInterval != 15*time.Second {
		t.Errorf("Agent.HeartbeatInterval = %v, want 15s", cfg.Agent.HeartbeatInterval)
	}
	if cfg.Agent.WatchAllNS {
		t.Error("Agent.WatchAllNS = true, want false")
	}
	if len(cfg.Agent.Namespaces) != 2 {
		t.Errorf("len(Agent.Namespaces) = %v, want 2", len(cfg.Agent.Namespaces))
	}
}

func TestValidate_MissingAPIKey(t *testing.T) {
	cfg := &Config{
		Agent: AgentConfig{Name: "test"},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() error = nil, want error for missing api.key")
	}
}

func TestValidate_MissingAgentName(t *testing.T) {
	cfg := &Config{
		API: APIConfig{Key: "test-key"},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() error = nil, want error for missing agent.name")
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := &Config{
		API: APIConfig{Key: "test-key"},
		Agent: AgentConfig{
			Name:         "test",
			MetricsPort:  9402,
			SyncInterval: 30 * time.Second,
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_SyncIntervalTooShort(t *testing.T) {
	cfg := &Config{
		API: APIConfig{Key: "test-key"},
		Agent: AgentConfig{
			Name:         "test",
			SyncInterval: 5 * time.Second, // Too short
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() error = nil, want error for sync_interval < 10s")
	}
}

func TestValidate_InvalidMetricsPort(t *testing.T) {
	cfg := &Config{
		API: APIConfig{Key: "test-key"},
		Agent: AgentConfig{
			Name:         "test",
			MetricsPort:  -1, // Invalid
			SyncInterval: 30 * time.Second,
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() error = nil, want error for invalid metrics_port")
	}
}
