package initcmd

import (
	"testing"
	"time"
)

func TestNewWizardState(t *testing.T) {
	state := NewWizardState()

	if state.ConfigPath != "./certwatch.yaml" {
		t.Errorf("expected ConfigPath './certwatch.yaml', got %q", state.ConfigPath)
	}

	if state.APIEndpoint != "https://api.certwatch.app" {
		t.Errorf("expected APIEndpoint 'https://api.certwatch.app', got %q", state.APIEndpoint)
	}

	if state.SyncInterval != "5m" {
		t.Errorf("expected SyncInterval '5m', got %q", state.SyncInterval)
	}

	if state.ScanInterval != "1m" {
		t.Errorf("expected ScanInterval '1m', got %q", state.ScanInterval)
	}

	if state.LogLevel != "info" {
		t.Errorf("expected LogLevel 'info', got %q", state.LogLevel)
	}

	if state.Concurrency != 10 {
		t.Errorf("expected Concurrency 10, got %d", state.Concurrency)
	}

	if state.CurrentCert.PortStr != "443" {
		t.Errorf("expected CurrentCert.PortStr '443', got %q", state.CurrentCert.PortStr)
	}
}

func TestWizardState_ToConfig(t *testing.T) {
	state := &WizardState{
		ConfigPath:   "./test.yaml",
		APIKey:       "cw_test_key",
		APIEndpoint:  "https://api.certwatch.app",
		APITimeout:   "30s",
		AgentName:    "test-agent",
		SyncInterval: "5m",
		ScanInterval: "1m",
		LogLevel:     "info",
		Concurrency:  10,
		Certificates: []CertificateInput{
			{
				Hostname: "api.example.com",
				PortStr:  "443",
				Tags:     "production, api",
				Notes:    "Main API",
			},
			{
				Hostname: "www.example.com",
				PortStr:  "8443",
				Tags:     "production, web",
				Notes:    "",
			},
		},
	}

	cfg, err := state.ToConfig()
	if err != nil {
		t.Fatalf("ToConfig() error = %v", err)
	}

	// Check API config
	if cfg.API.Endpoint != "https://api.certwatch.app" {
		t.Errorf("expected API.Endpoint 'https://api.certwatch.app', got %q", cfg.API.Endpoint)
	}
	if cfg.API.Key != "cw_test_key" {
		t.Errorf("expected API.Key 'cw_test_key', got %q", cfg.API.Key)
	}
	if cfg.API.Timeout != 30*time.Second {
		t.Errorf("expected API.Timeout 30s, got %v", cfg.API.Timeout)
	}

	// Check Agent config
	if cfg.Agent.Name != "test-agent" {
		t.Errorf("expected Agent.Name 'test-agent', got %q", cfg.Agent.Name)
	}
	if cfg.Agent.SyncInterval != 5*time.Minute {
		t.Errorf("expected Agent.SyncInterval 5m, got %v", cfg.Agent.SyncInterval)
	}
	if cfg.Agent.ScanInterval != time.Minute {
		t.Errorf("expected Agent.ScanInterval 1m, got %v", cfg.Agent.ScanInterval)
	}
	if cfg.Agent.LogLevel != "info" {
		t.Errorf("expected Agent.LogLevel 'info', got %q", cfg.Agent.LogLevel)
	}
	if cfg.Agent.Concurrency != 10 {
		t.Errorf("expected Agent.Concurrency 10, got %d", cfg.Agent.Concurrency)
	}

	// Check Certificates
	if len(cfg.Certificates) != 2 {
		t.Fatalf("expected 2 certificates, got %d", len(cfg.Certificates))
	}

	cert1 := cfg.Certificates[0]
	if cert1.Hostname != "api.example.com" {
		t.Errorf("expected cert1.Hostname 'api.example.com', got %q", cert1.Hostname)
	}
	if cert1.Port != 443 {
		t.Errorf("expected cert1.Port 443, got %d", cert1.Port)
	}
	if len(cert1.Tags) != 2 || cert1.Tags[0] != "production" || cert1.Tags[1] != "api" {
		t.Errorf("expected cert1.Tags [production, api], got %v", cert1.Tags)
	}
	if cert1.Notes != "Main API" {
		t.Errorf("expected cert1.Notes 'Main API', got %q", cert1.Notes)
	}

	cert2 := cfg.Certificates[1]
	if cert2.Hostname != "www.example.com" {
		t.Errorf("expected cert2.Hostname 'www.example.com', got %q", cert2.Hostname)
	}
	if cert2.Port != 8443 {
		t.Errorf("expected cert2.Port 8443, got %d", cert2.Port)
	}
}

func TestParseTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty string", "", nil},
		{"whitespace only", "   ", nil},
		{"single tag", "production", []string{"production"}},
		{"multiple tags", "production, api, critical", []string{"production", "api", "critical"}},
		{"with extra spaces", "  production  ,  api  ", []string{"production", "api"}},
		{"empty elements", "production,,api", []string{"production", "api"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTags(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("parseTags(%q) = %v, expected %v", tt.input, result, tt.expected)
				return
			}

			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("parseTags(%q)[%d] = %q, expected %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestWizardState_SaveAndResetCert(t *testing.T) {
	state := NewWizardState()

	// Set current cert
	state.CurrentCert = CertificateInput{
		Hostname: "api.example.com",
		PortStr:  "443",
		Tags:     "production",
		Notes:    "Test",
	}
	state.AddAnother = true

	// Save it
	state.SaveCurrentCert()

	if len(state.Certificates) != 1 {
		t.Errorf("expected 1 certificate after save, got %d", len(state.Certificates))
	}

	if state.Certificates[0].Hostname != "api.example.com" {
		t.Errorf("expected saved hostname 'api.example.com', got %q", state.Certificates[0].Hostname)
	}

	// Reset for next
	state.ResetCurrentCert()

	if state.CurrentCert.Hostname != "" {
		t.Errorf("expected empty hostname after reset, got %q", state.CurrentCert.Hostname)
	}

	if state.CurrentCert.PortStr != "443" {
		t.Errorf("expected default port after reset, got %q", state.CurrentCert.PortStr)
	}

	if state.AddAnother {
		t.Error("expected AddAnother to be false after reset")
	}
}

func TestWizardState_ToConfig_InvalidDuration(t *testing.T) {
	state := &WizardState{
		APIKey:       "cw_test_key",
		APIEndpoint:  "https://api.certwatch.app",
		APITimeout:   "30s",
		AgentName:    "test-agent",
		SyncInterval: "invalid",
		ScanInterval: "1m",
		LogLevel:     "info",
		Concurrency:  10,
		Certificates: []CertificateInput{
			{Hostname: "example.com", PortStr: "443"},
		},
	}

	_, err := state.ToConfig()
	if err == nil {
		t.Error("expected error for invalid sync interval")
	}
}
