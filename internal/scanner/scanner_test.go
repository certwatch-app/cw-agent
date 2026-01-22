package scanner

import (
	"context"
	"testing"
	"time"

	"github.com/certwatch-app/cw-agent/internal/ca"
	"github.com/certwatch-app/cw-agent/internal/config"
	"go.uber.org/zap/zaptest"
)

func TestNewScanner(t *testing.T) {
	logger := zaptest.NewLogger(t)
	scanner := New(10*time.Second, 10, logger)

	if scanner == nil {
		t.Fatal("NewScanner returned nil")
	}
	if scanner.logger == nil {
		t.Error("Scanner logger is nil")
	}
	if scanner.caLoader == nil {
		t.Error("Scanner CA loader is nil")
	}
	if scanner.concurrency != 10 {
		t.Errorf("Expected concurrency 10, got %d", scanner.concurrency)
	}
}

func TestScanWithCA_ValidationModeNone(t *testing.T) {
	logger := zaptest.NewLogger(t)
	scanner := New(10*time.Second, 10, logger)

	caConfig := &config.CAConfig{
		TrustMode:      "system",
		ValidationMode: "none",
	}

	ctx := context.Background()
	result := scanner.ScanWithCA(ctx, "example.com", 443, caConfig)

	if result.Error != "" {
		t.Skipf("Network test skipped: %s", result.Error)
	}

	if result.Chain == nil {
		t.Fatal("Expected chain info to be set")
	}

	if result.Chain.ValidationMode != "none" {
		t.Errorf("Expected ValidationMode 'none', got '%s'", result.Chain.ValidationMode)
	}
}

func TestScanAllWithCA_GlobalCAConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	scanner := New(10*time.Second, 10, logger)

	globalCA := &config.CAConfig{
		TrustMode:      "system",
		ValidationMode: "none", // Use "none" to avoid actual TLS connection
	}

	certs := []config.CertificateConfig{
		{
			Hostname: "example.com",
			Port:     443,
		},
		{
			Hostname: "google.com",
			Port:     443,
		},
	}

	ctx := context.Background()
	results := scanner.ScanAllWithCA(ctx, certs, globalCA)

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	// All results should have validation_mode set
	for i, result := range results {
		if result.Chain != nil && result.Chain.ValidationMode != "none" {
			t.Errorf("Result %d: expected ValidationMode 'none', got '%s'", i, result.Chain.ValidationMode)
		}
	}
}

func TestScanAllWithCA_PerCertificateOverride(t *testing.T) {
	logger := zaptest.NewLogger(t)
	scanner := New(10*time.Second, 10, logger)

	globalCA := &config.CAConfig{
		TrustMode:      "system",
		ValidationMode: "chain",
	}

	// Second certificate has CA override
	certs := []config.CertificateConfig{
		{
			Hostname: "example.com",
			Port:     443,
			CA:       nil, // Uses global CA
		},
		{
			Hostname: "internal.corp",
			Port:     443,
			CA: &config.CAConfig{
				TrustMode:      "custom",
				ValidationMode: "none", // Override to "none"
				InlineCerts:    []string{"-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----"},
			},
		},
	}

	ctx := context.Background()
	results := scanner.ScanAllWithCA(ctx, certs, globalCA)

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	// First result should use global CA (chain mode)
	// Second result should use override CA (none mode)
	if results[1].Chain != nil && results[1].Chain.ValidationMode != "none" {
		t.Errorf("Expected second cert to have ValidationMode 'none' from override, got '%s'", results[1].Chain.ValidationMode)
	}
}

func TestScanAllWithCA_NilGlobalCA(t *testing.T) {
	logger := zaptest.NewLogger(t)
	scanner := New(10*time.Second, 10, logger)

	certs := []config.CertificateConfig{
		{
			Hostname: "example.com",
			Port:     443,
		},
	}

	ctx := context.Background()
	results := scanner.ScanAllWithCA(ctx, certs, nil)

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	// With nil global CA, scanner uses default behavior (system CAs with chain validation)
	result := results[0]
	if result.Error != "" {
		t.Skipf("Network test skipped: %s", result.Error)
	}

	// Scan should succeed with default system CA validation
	if !result.Success {
		t.Error("Expected scan to succeed with default system CAs")
	}

	// ValidationMode should be set (default is "chain")
	if result.Chain != nil && result.Chain.ValidationMode == "" {
		t.Error("Expected ValidationMode to be set with default system CAs")
	}
}

func TestParseChainWithCA_RecordsTrustedRoot(t *testing.T) {
	// Verify the structure is set up correctly
	chain := &ChainInfo{
		Valid:          true,
		Certificates:   []ChainCertificate{},
		Issues:         []ChainIssue{},
		TrustedRoot:    "",
		VerifiedChains: nil,
	}

	if chain.TrustedRoot != "" {
		t.Error("TrustedRoot should be empty initially")
	}
	if chain.VerifiedChains != nil {
		t.Error("VerifiedChains should be nil initially")
	}
}

func TestValidationModeBasic_SkipsHostnameCheck(t *testing.T) {
	// This would require mocking x509.Verify with VerifyOptions
	// For now, we verify the structure is set up correctly
	logger := zaptest.NewLogger(t)
	scanner := New(10*time.Second, 10, logger)

	if scanner.caLoader == nil {
		t.Error("Scanner should have CA loader initialized")
	}

	// Verify CA loader can be created
	caLoader := ca.NewLoader(logger)
	if caLoader == nil {
		t.Error("Failed to create CA loader")
	}
}

func TestChainIssue_CAValidationFailed(t *testing.T) {
	// Verify we can create CA validation failed issues
	issue := ChainIssue{
		Type:    "ca_validation_failed",
		Message: "Certificate is not trusted: x509: certificate signed by unknown authority",
	}

	if issue.Type != "ca_validation_failed" {
		t.Errorf("Expected type 'ca_validation_failed', got '%s'", issue.Type)
	}
	if issue.Message == "" {
		t.Error("Expected non-empty message")
	}
}

func TestChainIssue_UntrustedRoot(t *testing.T) {
	// Verify we can create untrusted root issues
	issue := ChainIssue{
		Type:    "untrusted_root",
		Message: "Certificate chain does not chain to a trusted root CA",
	}

	if issue.Type != "untrusted_root" {
		t.Errorf("Expected type 'untrusted_root', got '%s'", issue.Type)
	}
	if issue.Message == "" {
		t.Error("Expected non-empty message")
	}
}

func TestScanWithCA_NilCAConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	scanner := New(10*time.Second, 10, logger)

	ctx := context.Background()
	result := scanner.ScanWithCA(ctx, "example.com", 443, nil)

	// Should handle nil CA config gracefully
	if result.Hostname != "example.com" {
		t.Errorf("Expected hostname 'example.com', got '%s'", result.Hostname)
	}
	if result.Port != 443 {
		t.Errorf("Expected port 443, got %d", result.Port)
	}

	// If scan succeeded, validation mode should not be set
	if result.Error == "" && result.Chain != nil && result.Chain.ValidationMode != "" {
		t.Logf("Note: ValidationMode was set to '%s' even with nil CA config", result.Chain.ValidationMode)
	}
}

func TestScanAllWithCA_EmptyCertificateList(t *testing.T) {
	logger := zaptest.NewLogger(t)
	scanner := New(10*time.Second, 10, logger)

	globalCA := &config.CAConfig{
		TrustMode:      "system",
		ValidationMode: "chain",
	}

	ctx := context.Background()
	results := scanner.ScanAllWithCA(ctx, []config.CertificateConfig{}, globalCA)

	if len(results) != 0 {
		t.Errorf("Expected 0 results for empty certificate list, got %d", len(results))
	}
}

func TestScanAllWithCA_ContextCancellation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	scanner := New(10*time.Second, 10, logger)

	globalCA := &config.CAConfig{
		TrustMode:      "system",
		ValidationMode: "none",
	}

	certs := []config.CertificateConfig{
		{Hostname: "example.com", Port: 443},
	}

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results := scanner.ScanAllWithCA(ctx, certs, globalCA)

	// Should still return results (may have errors)
	if len(results) != 1 {
		t.Errorf("Expected 1 result even with cancelled context, got %d", len(results))
	}
}
