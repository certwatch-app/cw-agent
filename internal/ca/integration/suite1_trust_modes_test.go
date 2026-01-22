// Package integration contains integration tests for CA validation.
// Suite 1: Trust Mode Scenarios
package integration

import (
	"context"
	"crypto/x509"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/certwatch-app/cw-agent/internal/ca/integration/harness"
	"github.com/certwatch-app/cw-agent/internal/config"
	"github.com/certwatch-app/cw-agent/internal/scanner"
)

// TestIntegration_TrustMode_CustomInternal tests custom trust mode with internal CA.
// This is the primary use case: monitoring internal services with a custom CA bundle.
func TestIntegration_TrustMode_CustomInternal(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test infrastructure
	caHarness := harness.NewTestCAHarness(t)

	// Generate internal server cert signed by Intermediate CA (which is signed by Root CA)
	// Use 127.0.0.1 as CN since we're testing locally
	serverCert, serverKey, _, _ := caHarness.GenerateServerCert(
		"127.0.0.1",
		[]string{"internal.corp", "*.internal.corp"},
		caHarness.IntermediateCert,
		caHarness.IntermediateKey,
	)

	// Start HTTPS server with internal cert
	server := harness.NewTestHTTPSServer(t, serverCert, serverKey)
	server.Start(t)
	defer server.Stop()

	// Write CA bundle (Root + Intermediate)
	caPath := caHarness.WriteCABundle([]*x509.Certificate{
		caHarness.RootCert,
		caHarness.IntermediateCert,
	}, "internal-ca-bundle.pem")

	// Create config with trust_mode: custom, use basic validation to avoid hostname issues
	cfg := harness.QuickConfig(t, server.Hostname, server.Port, "custom", "basic", []string{caPath})

	// Create scanner with debug logging
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	s := scanner.New(5*time.Second, 1, logger)
	defer s.Shutdown()

	// Scan internal server
	ctx := context.Background()
	result := s.ScanWithCA(ctx, server.Hostname, server.Port, cfg.CA)

	// Verify scan succeeded
	if !result.Success {
		t.Fatalf("Scan failed: %s", result.Error)
	}

	// Verify certificate info
	if result.Certificate == nil {
		t.Fatal("No certificate returned")
	}
	if result.Certificate.Subject != "127.0.0.1" {
		t.Errorf("Expected subject '127.0.0.1', got '%s'", result.Certificate.Subject)
	}

	// Verify CA validation succeeded
	if result.Chain == nil {
		t.Fatal("No chain info returned")
	}
	if !result.Chain.Valid {
		t.Errorf("Chain validation failed: %s", result.Chain.ValidationError)
	}
	// Trusted root can be either Root CA or Intermediate CA depending on chain building
	if result.Chain.TrustedRoot == "" {
		t.Error("Expected trusted root to be set")
	}
	if result.Chain.ValidationMode != "basic" {
		t.Errorf("Expected validation mode 'basic', got '%s'", result.Chain.ValidationMode)
	}

	t.Logf("✓ Custom trust mode with internal CA succeeded")
	t.Logf("  Trusted Root: %s", result.Chain.TrustedRoot)
	t.Logf("  Validation Mode: %s", result.Chain.ValidationMode)
}

// TestIntegration_TrustMode_SystemDefault tests system trust mode (default OS CA bundle).
// This demonstrates CertWatch works with public certificates out-of-the-box.
//
// Note: This test uses google.com as a known-good public server with valid cert.
// If network is unavailable, the test will be skipped.
func TestIntegration_TrustMode_SystemDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create config with trust_mode: system (default)
	cfg := &config.Config{
		API: config.APIConfig{
			Endpoint: "https://api.certwatch.app",
			Key:      "cw_test_key",
			Timeout:  30 * time.Second,
		},
		Agent: config.AgentConfig{
			Name:        "test-agent",
			LogLevel:    "debug",
			Concurrency: 1,
		},
		CA: &config.CAConfig{
			TrustMode:      "system",
			ValidationMode: "chain",
		},
	}

	// Create scanner
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	s := scanner.New(10*time.Second, 1, logger)
	defer s.Shutdown()

	// Scan google.com (known public CA)
	ctx := context.Background()
	result := s.ScanWithCA(ctx, "google.com", 443, cfg.CA)

	// Check if connection failed (network issue - skip test)
	if !result.Success {
		if contains(result.Error, "connection failed") || contains(result.Error, "timeout") {
			t.Skipf("Network unavailable, skipping test: %s", result.Error)
		}
		t.Fatalf("Scan failed: %s", result.Error)
	}

	// Verify CA validation with system CAs
	if result.Chain == nil {
		t.Fatal("No chain info returned")
	}
	if !result.Chain.Valid {
		t.Errorf("Chain validation failed: %s", result.Chain.ValidationError)
	}
	// Trusted root will be a well-known public CA (e.g., DigiCert, Let's Encrypt)
	if result.Chain.TrustedRoot == "" {
		t.Error("Expected trusted root to be set")
	}

	t.Logf("✓ System trust mode with public CA succeeded")
	t.Logf("  Trusted Root: %s", result.Chain.TrustedRoot)
	t.Logf("  Subject: %s", result.Certificate.Subject)
}

// TestIntegration_TrustMode_CombinedPublicAndInternal tests combined trust mode.
// This shows CertWatch can validate both public and internal certificates.
func TestIntegration_TrustMode_CombinedPublicAndInternal(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test infrastructure
	caHarness := harness.NewTestCAHarness(t)

	// Generate internal server cert (use 127.0.0.1 for local testing)
	internalCert, internalKey, _, _ := caHarness.GenerateServerCert(
		"127.0.0.1",
		[]string{"internal.corp"},
		caHarness.IntermediateCert,
		caHarness.IntermediateKey,
	)

	// Start internal HTTPS server
	internalServer := harness.NewTestHTTPSServer(t, internalCert, internalKey)
	internalServer.Start(t)
	defer internalServer.Stop()

	// Write internal CA bundle
	caPath := caHarness.WriteCABundle([]*x509.Certificate{
		caHarness.RootCert,
		caHarness.IntermediateCert,
	}, "internal-ca.pem")

	// Create config with trust_mode: combined (use basic to avoid hostname issues)
	cfg := harness.QuickConfig(t, internalServer.Hostname, internalServer.Port, "combined", "basic", []string{caPath})

	// Create scanner
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	s := scanner.New(10*time.Second, 1, logger)
	defer s.Shutdown()

	ctx := context.Background()

	// Test 1: Scan internal server (should validate with custom CA)
	t.Run("InternalService", func(t *testing.T) {
		result := s.ScanWithCA(ctx, internalServer.Hostname, internalServer.Port, cfg.CA)

		if !result.Success {
			t.Fatalf("Internal scan failed: %s", result.Error)
		}
		if !result.Chain.Valid {
			t.Errorf("Internal chain validation failed: %s", result.Chain.ValidationError)
		}
		// Trusted root can be either Root or Intermediate depending on chain building
		if result.Chain.TrustedRoot == "" {
			t.Error("Expected trusted root to be set")
		}

		t.Logf("✓ Internal service validated with custom CA")
	})

	// Test 2: Scan public server (should validate with system CAs)
	t.Run("PublicService", func(t *testing.T) {
		result := s.ScanWithCA(ctx, "google.com", 443, cfg.CA)

		// Skip if network unavailable
		if !result.Success {
			if contains(result.Error, "connection failed") || contains(result.Error, "timeout") {
				t.Skipf("Network unavailable: %s", result.Error)
			}
			t.Fatalf("Public scan failed: %s", result.Error)
		}

		if !result.Chain.Valid {
			t.Errorf("Public chain validation failed: %s", result.Chain.ValidationError)
		}
		// Public CA root should differ from internal
		if result.Chain.TrustedRoot == "Test Root CA" {
			t.Error("Expected public CA root, got internal CA root")
		}

		t.Logf("✓ Public service validated with system CA")
		t.Logf("  Trusted Root: %s", result.Chain.TrustedRoot)
	})
}

// TestIntegration_TrustMode_CustomRejectsPublic tests that custom mode rejects public CAs.
// This demonstrates security: custom mode doesn't trust unexpected CAs.
func TestIntegration_TrustMode_CustomRejectsPublic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test infrastructure with internal CA only
	caHarness := harness.NewTestCAHarness(t)
	caPath := caHarness.WriteCABundle([]*x509.Certificate{
		caHarness.RootCert,
	}, "internal-only-ca.pem")

	// Create config with trust_mode: custom (only internal CA)
	cfg := harness.QuickConfig(t, "google.com", 443, "custom", "chain", []string{caPath})

	// Create scanner
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	s := scanner.New(10*time.Second, 1, logger)
	defer s.Shutdown()

	// Scan public server with only internal CA trusted
	ctx := context.Background()
	result := s.ScanWithCA(ctx, "google.com", 443, cfg.CA)

	// Connection should succeed, but CA validation should FAIL
	if !result.Success {
		// Network issue - skip test
		if contains(result.Error, "connection failed") || contains(result.Error, "timeout") {
			t.Skipf("Network unavailable: %s", result.Error)
		}
		t.Fatalf("Scan failed: %s", result.Error)
	}

	// Verify CA validation FAILED (public CA not trusted)
	if result.Chain.Valid {
		t.Error("Expected CA validation to FAIL for public cert with custom-only trust")
	}
	if !contains(result.Chain.ValidationError, "unknown authority") {
		t.Errorf("Expected 'unknown authority' error, got: %s", result.Chain.ValidationError)
	}

	t.Logf("✓ Custom trust mode correctly rejected public CA")
	t.Logf("  Validation Error: %s", result.Chain.ValidationError)
}

// Helper functions

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
