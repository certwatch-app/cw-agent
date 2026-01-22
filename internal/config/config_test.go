package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestCAConfigDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "minimal.yaml")

	configContent := `
api:
  key: "cw_test_key"
  endpoint: "https://api.test.com"
agent:
  name: "test-agent"
  sync_interval: 60s
certificates:
  - hostname: "example.com"
    port: 443
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	cfg, err := Load(v)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.CA == nil {
		t.Fatal("Expected CA config to be initialized with defaults")
	}

	if cfg.CA.TrustMode != "system" {
		t.Errorf("Expected default trust_mode 'system', got '%s'", cfg.CA.TrustMode)
	}

	if cfg.CA.ValidationMode != "chain" {
		t.Errorf("Expected default validation_mode 'chain', got '%s'", cfg.CA.ValidationMode)
	}
}

func TestValidateCA_ValidConfigs(t *testing.T) {
	tests := []struct {
		name   string
		config *CAConfig
	}{
		{
			name: "system trust mode",
			config: &CAConfig{
				TrustMode:      "system",
				ValidationMode: "chain",
			},
		},
		{
			name: "custom trust mode with ca_bundles",
			config: &CAConfig{
				TrustMode:      "custom",
				ValidationMode: "chain",
				CABundles:      []string{"/etc/ssl/ca.pem"},
			},
		},
		{
			name: "custom trust mode with inline_certs",
			config: &CAConfig{
				TrustMode:      "custom",
				ValidationMode: "basic",
				InlineCerts:    []string{"-----BEGIN CERTIFICATE-----\n..."},
			},
		},
		{
			name: "custom trust mode with both",
			config: &CAConfig{
				TrustMode:      "custom",
				ValidationMode: "none",
				CABundles:      []string{"/etc/ssl/ca.pem"},
				InlineCerts:    []string{"-----BEGIN CERTIFICATE-----\n..."},
			},
		},
		{
			name: "combined trust mode",
			config: &CAConfig{
				TrustMode:      "combined",
				ValidationMode: "chain",
				CABundles:      []string{"/etc/ssl/ca.pem"},
			},
		},
		{
			name: "system mode with validation none",
			config: &CAConfig{
				TrustMode:      "system",
				ValidationMode: "none",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{CA: tt.config}
			err := cfg.validateCA(tt.config)
			if err != nil {
				t.Errorf("Expected no error for valid config, got: %v", err)
			}
		})
	}
}

func TestValidateCA_InvalidTrustMode(t *testing.T) {
	tests := []struct {
		name      string
		trustMode string
	}{
		{"empty trust mode", ""},
		{"invalid trust mode", "invalid"},
		{"unknown trust mode", "local"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				CA: &CAConfig{
					TrustMode:      tt.trustMode,
					ValidationMode: "chain",
				},
			}
			err := cfg.validateCA(cfg.CA)
			if err == nil {
				t.Error("Expected error for invalid trust_mode")
			}
		})
	}
}

func TestValidateCA_InvalidValidationMode(t *testing.T) {
	tests := []struct {
		name           string
		validationMode string
	}{
		{"empty validation mode", ""},
		{"invalid validation mode", "invalid"},
		{"unknown validation mode", "full"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				CA: &CAConfig{
					TrustMode:      "system",
					ValidationMode: tt.validationMode,
				},
			}
			err := cfg.validateCA(cfg.CA)
			if err == nil {
				t.Error("Expected error for invalid validation_mode")
			}
		})
	}
}

func TestValidateCA_CustomModeRequiresCAs(t *testing.T) {
	tests := []struct {
		name        string
		caBundles   []string
		inlineCerts []string
		expectError bool
	}{
		{
			name:        "no CAs provided",
			caBundles:   nil,
			inlineCerts: nil,
			expectError: true,
		},
		{
			name:        "empty ca_bundles and inline_certs",
			caBundles:   []string{},
			inlineCerts: []string{},
			expectError: true,
		},
		{
			name:        "only ca_bundles",
			caBundles:   []string{"/etc/ssl/ca.pem"},
			inlineCerts: nil,
			expectError: false,
		},
		{
			name:        "only inline_certs",
			caBundles:   nil,
			inlineCerts: []string{"-----BEGIN CERTIFICATE-----"},
			expectError: false,
		},
		{
			name:        "both provided",
			caBundles:   []string{"/etc/ssl/ca.pem"},
			inlineCerts: []string{"-----BEGIN CERTIFICATE-----"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				CA: &CAConfig{
					TrustMode:      "custom",
					ValidationMode: "chain",
					CABundles:      tt.caBundles,
					InlineCerts:    tt.inlineCerts,
				},
			}
			err := cfg.validateCA(cfg.CA)
			if tt.expectError && err == nil {
				t.Error("Expected error for custom mode without CAs")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestValidateCA_CombinedModeRequiresCAs(t *testing.T) {
	tests := []struct {
		name        string
		caBundles   []string
		inlineCerts []string
		expectError bool
	}{
		{
			name:        "no CAs provided",
			caBundles:   nil,
			inlineCerts: nil,
			expectError: true,
		},
		{
			name:        "ca_bundles provided",
			caBundles:   []string{"/etc/ssl/ca.pem"},
			inlineCerts: nil,
			expectError: false,
		},
		{
			name:        "inline_certs provided",
			caBundles:   nil,
			inlineCerts: []string{"-----BEGIN CERTIFICATE-----"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				CA: &CAConfig{
					TrustMode:      "combined",
					ValidationMode: "chain",
					CABundles:      tt.caBundles,
					InlineCerts:    tt.inlineCerts,
				},
			}
			err := cfg.validateCA(cfg.CA)
			if tt.expectError && err == nil {
				t.Error("Expected error for combined mode without custom CAs")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestPerCertificateCAOverride(t *testing.T) {
	// Create a temporary test config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `
api:
  key: "test-key"
  endpoint: "https://api.test.com"

agent:
  name: "test-agent"
  sync_interval: 60s

# Global CA config
ca:
  trust_mode: "system"
  validation_mode: "chain"

certificates:
  # Uses global CA config
  - hostname: "example.com"
    port: 443

  # Overrides with custom CA
  - hostname: "internal.corp"
    port: 443
    ca:
      trust_mode: "custom"
      validation_mode: "basic"
      ca_bundles:
        - "/etc/ssl/internal-ca.pem"
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	cfg, err := Load(v)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify global CA config
	if cfg.CA == nil {
		t.Fatal("Expected global CA config to be loaded")
	}
	if cfg.CA.TrustMode != "system" {
		t.Errorf("Expected global trust_mode 'system', got '%s'", cfg.CA.TrustMode)
	}

	// Verify first certificate uses global CA (no override)
	if cfg.Certificates[0].CA != nil {
		t.Error("Expected first certificate to have nil CA (uses global)")
	}

	// Verify second certificate has CA override
	if cfg.Certificates[1].CA == nil {
		t.Fatal("Expected second certificate to have CA override")
	}
	if cfg.Certificates[1].CA.TrustMode != "custom" {
		t.Errorf("Expected override trust_mode 'custom', got '%s'", cfg.Certificates[1].CA.TrustMode)
	}
	if cfg.Certificates[1].CA.ValidationMode != "basic" {
		t.Errorf("Expected override validation_mode 'basic', got '%s'", cfg.Certificates[1].CA.ValidationMode)
	}
}

func TestBackwardCompatibility_NoCAConfig(t *testing.T) {
	// Create config without CA section (old format)
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "old-config.yaml")

	configContent := `
api:
  key: "test-key"
  endpoint: "https://api.test.com"

agent:
  name: "test-agent"
  sync_interval: 60s

certificates:
  - hostname: "example.com"
    port: 443
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	cfg, err := Load(v)
	if err != nil {
		t.Fatalf("Failed to load old config format: %v", err)
	}

	// Should have default CA config after setDefaults()
	if cfg.CA == nil {
		t.Fatal("Expected default CA config to be set")
	}
	if cfg.CA.TrustMode != "system" {
		t.Errorf("Expected default trust_mode 'system', got '%s'", cfg.CA.TrustMode)
	}
	if cfg.CA.ValidationMode != "chain" {
		t.Errorf("Expected default validation_mode 'chain', got '%s'", cfg.CA.ValidationMode)
	}
}

func TestValidate_IncludesCAValidation(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid-ca.yaml")

	// Config with invalid CA trust mode
	configContent := `
api:
  key: "test-key"
  endpoint: "https://api.test.com"

agent:
  name: "test-agent"
  sync_interval: 60s

ca:
  trust_mode: "invalid-mode"
  validation_mode: "chain"

certificates:
  - hostname: "example.com"
    port: 443
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	cfg, err := Load(v)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validate should catch the invalid CA trust_mode
	err = cfg.Validate()
	if err == nil {
		t.Error("Expected error for invalid CA trust_mode")
	}
}

func TestValidate_PerCertCAValidation(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid-per-cert-ca.yaml")

	// Config with invalid per-certificate CA
	configContent := `
api:
  key: "test-key"
  endpoint: "https://api.test.com"

agent:
  name: "test-agent"
  sync_interval: 60s

certificates:
  - hostname: "example.com"
    port: 443
    ca:
      trust_mode: "custom"
      validation_mode: "chain"
      # Missing ca_bundles and inline_certs
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	cfg, err := Load(v)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validate should catch the custom mode without CAs
	err = cfg.Validate()
	if err == nil {
		t.Error("Expected error for custom mode without CAs in per-cert config")
	}
}
