package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/certwatch-app/cw-agent/internal/config"
	"github.com/certwatch-app/cw-agent/internal/scanner"
	"github.com/certwatch-app/cw-agent/internal/state"
)

// setupTestClient creates a properly initialized test client
func setupTestClient(t *testing.T) (*Client, *state.Manager) {
	t.Helper()

	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "certwatch-state.json")

	stateManager := state.NewManager(statePath)
	if err := stateManager.Load(); err != nil {
		// OK if file doesn't exist (first run)
		if !os.IsNotExist(err) {
			t.Fatalf("Failed to load state manager: %v", err)
		}
	}

	client := &Client{
		agentName:    "test-agent",
		stateManager: stateManager,
	}

	return client, stateManager
}

func TestBuildSyncRequest_PopulatesValidationFields(t *testing.T) {
	client, _ := setupTestClient(t)

	certs := []config.CertificateConfig{
		{
			Hostname: "example.com",
			Port:     443,
		},
	}

	now := time.Now()
	results := []scanner.ScanResult{
		{
			Hostname:  "example.com",
			Port:      443,
			Success:   true,
			ScannedAt: now,
			Certificate: &scanner.CertificateInfo{
				Subject:           "CN=example.com",
				Issuer:            "CN=Test CA",
				SerialNumber:      "123456",
				FingerprintSHA256: "abc123",
				NotBefore:         now.Add(-24 * time.Hour),
				NotAfter:          now.Add(90 * 24 * time.Hour),
			},
			Chain: &scanner.ChainInfo{
				Valid:           true,
				ValidationMode:  "chain",
				ValidationError: "",
				TrustedRoot:     "CN=Test Root CA",
			},
		},
	}

	req := client.buildSyncRequest(certs, results)

	if len(req.Certificates) != 1 {
		t.Fatalf("Expected 1 certificate in sync request, got %d", len(req.Certificates))
	}

	certData := req.Certificates[0]

	// Verify validation_mode is populated
	if certData.ValidationMode != "chain" {
		t.Errorf("Expected ValidationMode 'chain', got '%s'", certData.ValidationMode)
	}

	// Verify validation_error is empty (validation succeeded)
	if certData.ValidationError != "" {
		t.Errorf("Expected empty ValidationError, got '%s'", certData.ValidationError)
	}

	// Verify trusted_root is populated
	if certData.TrustedRoot != "CN=Test Root CA" {
		t.Errorf("Expected TrustedRoot 'CN=Test Root CA', got '%s'", certData.TrustedRoot)
	}
}

func TestBuildSyncRequest_PopulatesValidationError(t *testing.T) {
	client, _ := setupTestClient(t)

	certs := []config.CertificateConfig{
		{
			Hostname: "internal.corp",
			Port:     443,
		},
	}

	now := time.Now()
	results := []scanner.ScanResult{
		{
			Hostname:  "internal.corp",
			Port:      443,
			Success:   true,
			ScannedAt: now,
			Certificate: &scanner.CertificateInfo{
				Subject:   "CN=internal.corp",
				NotBefore: now.Add(-24 * time.Hour),
				NotAfter:  now.Add(90 * 24 * time.Hour),
			},
			Chain: &scanner.ChainInfo{
				Valid:           false,
				ValidationMode:  "chain",
				ValidationError: "x509: certificate signed by unknown authority",
				TrustedRoot:     "",
			},
		},
	}

	req := client.buildSyncRequest(certs, results)

	certData := req.Certificates[0]

	// Verify validation_mode is populated
	if certData.ValidationMode != "chain" {
		t.Errorf("Expected ValidationMode 'chain', got '%s'", certData.ValidationMode)
	}

	// Verify validation_error is populated
	expectedError := "x509: certificate signed by unknown authority"
	if certData.ValidationError != expectedError {
		t.Errorf("Expected ValidationError '%s', got '%s'", expectedError, certData.ValidationError)
	}

	// Verify trusted_root is empty (validation failed)
	if certData.TrustedRoot != "" {
		t.Errorf("Expected empty TrustedRoot, got '%s'", certData.TrustedRoot)
	}
}

func TestBuildSyncRequest_HandlesNilChain(t *testing.T) {
	client, _ := setupTestClient(t)

	certs := []config.CertificateConfig{
		{
			Hostname: "example.com",
			Port:     443,
		},
	}

	now := time.Now()
	results := []scanner.ScanResult{
		{
			Hostname:  "example.com",
			Port:      443,
			Success:   true,
			ScannedAt: now,
			Certificate: &scanner.CertificateInfo{
				Subject:   "CN=example.com",
				NotBefore: now.Add(-24 * time.Hour),
				NotAfter:  now.Add(90 * 24 * time.Hour),
			},
			Chain: nil, // Chain is nil
		},
	}

	// Should not panic
	req := client.buildSyncRequest(certs, results)

	certData := req.Certificates[0]

	// Validation fields should be empty
	if certData.ValidationMode != "" {
		t.Errorf("Expected empty ValidationMode with nil chain, got '%s'", certData.ValidationMode)
	}
	if certData.ValidationError != "" {
		t.Errorf("Expected empty ValidationError with nil chain, got '%s'", certData.ValidationError)
	}
	if certData.TrustedRoot != "" {
		t.Errorf("Expected empty TrustedRoot with nil chain, got '%s'", certData.TrustedRoot)
	}
}

func TestBuildSyncRequest_HandlesEmptyValidationFields(t *testing.T) {
	client, _ := setupTestClient(t)

	certs := []config.CertificateConfig{
		{
			Hostname: "example.com",
			Port:     443,
		},
	}

	now := time.Now()
	results := []scanner.ScanResult{
		{
			Hostname:  "example.com",
			Port:      443,
			Success:   true,
			ScannedAt: now,
			Certificate: &scanner.CertificateInfo{
				Subject:   "CN=example.com",
				NotBefore: now.Add(-24 * time.Hour),
				NotAfter:  now.Add(90 * 24 * time.Hour),
			},
			Chain: &scanner.ChainInfo{
				Valid:           true,
				ValidationMode:  "", // Empty validation mode
				ValidationError: "",
				TrustedRoot:     "",
			},
		},
	}

	req := client.buildSyncRequest(certs, results)

	certData := req.Certificates[0]

	// Should handle empty fields gracefully
	if certData.ValidationMode != "" {
		t.Errorf("Expected empty ValidationMode, got '%s'", certData.ValidationMode)
	}
	if certData.ValidationError != "" {
		t.Errorf("Expected empty ValidationError, got '%s'", certData.ValidationError)
	}
	if certData.TrustedRoot != "" {
		t.Errorf("Expected empty TrustedRoot, got '%s'", certData.TrustedRoot)
	}
}

func TestBuildSyncRequest_ValidationModeNone(t *testing.T) {
	client, _ := setupTestClient(t)

	certs := []config.CertificateConfig{
		{
			Hostname: "dev.local",
			Port:     8443,
		},
	}

	now := time.Now()
	results := []scanner.ScanResult{
		{
			Hostname:  "dev.local",
			Port:      8443,
			Success:   true,
			ScannedAt: now,
			Certificate: &scanner.CertificateInfo{
				Subject:   "CN=dev.local",
				NotBefore: now.Add(-24 * time.Hour),
				NotAfter:  now.Add(90 * 24 * time.Hour),
			},
			Chain: &scanner.ChainInfo{
				Valid:           true,
				ValidationMode:  "none", // Validation mode is "none"
				ValidationError: "",
				TrustedRoot:     "", // No trusted root since validation was skipped
			},
		},
	}

	req := client.buildSyncRequest(certs, results)

	certData := req.Certificates[0]

	// Verify validation_mode is "none"
	if certData.ValidationMode != "none" {
		t.Errorf("Expected ValidationMode 'none', got '%s'", certData.ValidationMode)
	}

	// No error since validation was skipped
	if certData.ValidationError != "" {
		t.Errorf("Expected empty ValidationError with mode 'none', got '%s'", certData.ValidationError)
	}

	// No trusted root since validation was skipped
	if certData.TrustedRoot != "" {
		t.Errorf("Expected empty TrustedRoot with mode 'none', got '%s'", certData.TrustedRoot)
	}
}

func TestBuildSyncRequest_ValidationModeBasic(t *testing.T) {
	client, _ := setupTestClient(t)

	certs := []config.CertificateConfig{
		{
			Hostname: "*.example.com",
			Port:     443,
		},
	}

	now := time.Now()
	results := []scanner.ScanResult{
		{
			Hostname:  "*.example.com",
			Port:      443,
			Success:   true,
			ScannedAt: now,
			Certificate: &scanner.CertificateInfo{
				Subject:   "CN=*.example.com",
				NotBefore: now.Add(-24 * time.Hour),
				NotAfter:  now.Add(90 * 24 * time.Hour),
			},
			Chain: &scanner.ChainInfo{
				Valid:           true,
				ValidationMode:  "basic", // Basic validation (no hostname check)
				ValidationError: "",
				TrustedRoot:     "CN=Test Root CA",
			},
		},
	}

	req := client.buildSyncRequest(certs, results)

	certData := req.Certificates[0]

	// Verify validation_mode is "basic"
	if certData.ValidationMode != "basic" {
		t.Errorf("Expected ValidationMode 'basic', got '%s'", certData.ValidationMode)
	}

	// Validation succeeded
	if certData.ValidationError != "" {
		t.Errorf("Expected empty ValidationError, got '%s'", certData.ValidationError)
	}

	// Trusted root should be populated
	if certData.TrustedRoot != "CN=Test Root CA" {
		t.Errorf("Expected TrustedRoot 'CN=Test Root CA', got '%s'", certData.TrustedRoot)
	}
}

func TestBuildSyncRequest_MultipleCertificates(t *testing.T) {
	client, _ := setupTestClient(t)

	certs := []config.CertificateConfig{
		{Hostname: "example.com", Port: 443},
		{Hostname: "internal.corp", Port: 443},
		{Hostname: "dev.local", Port: 8443},
	}

	now := time.Now()
	results := []scanner.ScanResult{
		{
			Hostname:  "example.com",
			Port:      443,
			Success:   true,
			ScannedAt: now,
			Certificate: &scanner.CertificateInfo{
				Subject:   "CN=example.com",
				NotBefore: now.Add(-24 * time.Hour),
				NotAfter:  now.Add(90 * 24 * time.Hour),
			},
			Chain: &scanner.ChainInfo{
				Valid:           true,
				ValidationMode:  "chain",
				ValidationError: "",
				TrustedRoot:     "CN=Public Root CA",
			},
		},
		{
			Hostname:  "internal.corp",
			Port:      443,
			Success:   true,
			ScannedAt: now,
			Certificate: &scanner.CertificateInfo{
				Subject:   "CN=internal.corp",
				NotBefore: now.Add(-24 * time.Hour),
				NotAfter:  now.Add(90 * 24 * time.Hour),
			},
			Chain: &scanner.ChainInfo{
				Valid:           true,
				ValidationMode:  "custom",
				ValidationError: "",
				TrustedRoot:     "CN=Internal Root CA",
			},
		},
		{
			Hostname:  "dev.local",
			Port:      8443,
			Success:   true,
			ScannedAt: now,
			Certificate: &scanner.CertificateInfo{
				Subject:   "CN=dev.local",
				NotBefore: now.Add(-24 * time.Hour),
				NotAfter:  now.Add(90 * 24 * time.Hour),
			},
			Chain: &scanner.ChainInfo{
				Valid:           true,
				ValidationMode:  "none",
				ValidationError: "",
				TrustedRoot:     "",
			},
		},
	}

	req := client.buildSyncRequest(certs, results)

	if len(req.Certificates) != 3 {
		t.Fatalf("Expected 3 certificates in sync request, got %d", len(req.Certificates))
	}

	// Verify each certificate has correct validation data
	tests := []struct {
		index          int
		hostname       string
		validationMode string
		trustedRoot    string
	}{
		{0, "example.com", "chain", "CN=Public Root CA"},
		{1, "internal.corp", "custom", "CN=Internal Root CA"},
		{2, "dev.local", "none", ""},
	}

	for _, tt := range tests {
		certData := req.Certificates[tt.index]
		if certData.Hostname != tt.hostname {
			t.Errorf("Certificate %d: expected hostname '%s', got '%s'", tt.index, tt.hostname, certData.Hostname)
		}
		if certData.ValidationMode != tt.validationMode {
			t.Errorf("Certificate %d: expected ValidationMode '%s', got '%s'", tt.index, tt.validationMode, certData.ValidationMode)
		}
		if certData.TrustedRoot != tt.trustedRoot {
			t.Errorf("Certificate %d: expected TrustedRoot '%s', got '%s'", tt.index, tt.trustedRoot, certData.TrustedRoot)
		}
	}
}

func TestBuildSyncRequest_NoResults(t *testing.T) {
	client, _ := setupTestClient(t)

	certs := []config.CertificateConfig{
		{
			Hostname: "example.com",
			Port:     443,
		},
	}

	// Empty results slice
	results := []scanner.ScanResult{}

	req := client.buildSyncRequest(certs, results)

	if len(req.Certificates) != 1 {
		t.Fatalf("Expected 1 certificate in sync request, got %d", len(req.Certificates))
	}

	certData := req.Certificates[0]

	// Should have hostname and port, but no scan data
	if certData.Hostname != "example.com" {
		t.Errorf("Expected hostname 'example.com', got '%s'", certData.Hostname)
	}

	// Validation fields should be empty (no scan result)
	if certData.ValidationMode != "" {
		t.Errorf("Expected empty ValidationMode with no scan results, got '%s'", certData.ValidationMode)
	}
}
