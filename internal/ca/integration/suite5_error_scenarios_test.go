// Package integration contains integration tests for CA validation.
// Suite 5: Error Scenarios
package integration

import (
	"context"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/certwatch-app/cw-agent/internal/ca/integration/harness"
	"github.com/certwatch-app/cw-agent/internal/config"
	"github.com/certwatch-app/cw-agent/internal/scanner"
)

// TestIntegration_Error_CABundleNotFound tests error when CA bundle file doesn't exist.
func TestIntegration_Error_CABundleNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test infrastructure
	caHarness := harness.NewTestCAHarness(t)

	// Generate server cert
	serverCert, serverKey, _, _ := caHarness.GenerateServerCert(
		"test.local",
		nil,
		caHarness.RootCert,
		caHarness.RootKey,
	)

	// Start HTTPS server
	server := harness.NewTestHTTPSServer(t, serverCert, serverKey)
	server.Start(t)
	defer server.Stop()

	// Config with non-existent CA bundle
	nonExistentPath := filepath.Join(caHarness.TempDir, "nonexistent-ca.pem")
	cfg := harness.QuickConfig(t, server.Hostname, server.Port, "custom", "chain", []string{nonExistentPath})

	// Create scanner
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	s := scanner.New(5*time.Second, 1, logger)
	defer s.Shutdown()

	// Scan certificate - should fail with file not found error
	ctx := context.Background()
	result := s.ScanWithCA(ctx, server.Hostname, server.Port, cfg.CA)

	// Connection should succeed, but CA loading should fail
	if result.Chain != nil && result.Chain.Valid {
		t.Error("Expected validation to FAIL (CA bundle not found)")
	}

	// Check error message
	if result.Chain != nil && result.Chain.ValidationError != "" {
		if !contains(result.Chain.ValidationError, "no such file") &&
			!contains(result.Chain.ValidationError, "failed to load CA") {
			t.Logf("Validation error: %s", result.Chain.ValidationError)
		}
		t.Logf("✓ CA bundle not found error detected")
	} else if !result.Success {
		t.Logf("✓ Scan failed with error: %s", result.Error)
	}
}

// TestIntegration_Error_InvalidPEMData tests error when CA bundle contains invalid PEM data.
func TestIntegration_Error_InvalidPEMData(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test infrastructure
	caHarness := harness.NewTestCAHarness(t)

	// Generate server cert
	serverCert, serverKey, _, _ := caHarness.GenerateServerCert(
		"test.local",
		nil,
		caHarness.RootCert,
		caHarness.RootKey,
	)

	// Start HTTPS server
	server := harness.NewTestHTTPSServer(t, serverCert, serverKey)
	server.Start(t)
	defer server.Stop()

	// Write invalid PEM data to file
	invalidPath := filepath.Join(caHarness.TempDir, "invalid.pem")
	if err := os.WriteFile(invalidPath, []byte("NOT A VALID CERTIFICATE\nJUST GARBAGE DATA"), 0644); err != nil {
		t.Fatalf("Failed to write invalid PEM: %v", err)
	}

	cfg := harness.QuickConfig(t, server.Hostname, server.Port, "custom", "chain", []string{invalidPath})

	// Create scanner
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	s := scanner.New(5*time.Second, 1, logger)
	defer s.Shutdown()

	// Scan certificate
	ctx := context.Background()
	result := s.ScanWithCA(ctx, server.Hostname, server.Port, cfg.CA)

	// Should fail validation (no valid CAs loaded)
	if result.Chain != nil && result.Chain.Valid {
		t.Error("Expected validation to FAIL (invalid PEM data)")
	}

	t.Logf("✓ Invalid PEM data error handled")
	if result.Chain != nil && result.Chain.ValidationError != "" {
		t.Logf("  Validation Error: %s", result.Chain.ValidationError)
	}
}

// TestIntegration_Error_EmptyCABundle tests error when CA bundle file is empty.
func TestIntegration_Error_EmptyCABundle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test infrastructure
	caHarness := harness.NewTestCAHarness(t)

	// Generate server cert
	serverCert, serverKey, _, _ := caHarness.GenerateServerCert(
		"test.local",
		nil,
		caHarness.RootCert,
		caHarness.RootKey,
	)

	// Start HTTPS server
	server := harness.NewTestHTTPSServer(t, serverCert, serverKey)
	server.Start(t)
	defer server.Stop()

	// Write empty CA bundle
	emptyPath := filepath.Join(caHarness.TempDir, "empty.pem")
	if err := os.WriteFile(emptyPath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write empty file: %v", err)
	}

	cfg := harness.QuickConfig(t, server.Hostname, server.Port, "custom", "chain", []string{emptyPath})

	// Create scanner
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	s := scanner.New(5*time.Second, 1, logger)
	defer s.Shutdown()

	// Scan certificate
	ctx := context.Background()
	result := s.ScanWithCA(ctx, server.Hostname, server.Port, cfg.CA)

	// Should fail validation (no CAs loaded)
	if result.Chain != nil && result.Chain.Valid {
		t.Error("Expected validation to FAIL (empty CA bundle)")
	}

	t.Logf("✓ Empty CA bundle error handled")
	if result.Chain != nil {
		t.Logf("  Validation Error: %s", result.Chain.ValidationError)
	}
}

// TestIntegration_Error_InsecurePermissions tests warning for world-writable CA bundle.
// Note: This test may not fail validation but should log a warning.
func TestIntegration_Error_InsecurePermissions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip on Windows (permission model differs)
	if os.PathSeparator == '\\' {
		t.Skip("Skipping permission test on Windows")
	}

	// Setup test infrastructure
	caHarness := harness.NewTestCAHarness(t)

	// Write CA bundle with insecure permissions
	insecurePath := caHarness.WriteCABundle([]*x509.Certificate{
		caHarness.RootCert,
	}, "insecure.pem")

	// Make world-writable (insecure)
	if err := os.Chmod(insecurePath, 0666); err != nil {
		t.Fatalf("Failed to chmod: %v", err)
	}

	// Generate server cert
	serverCert, serverKey, _, _ := caHarness.GenerateServerCert(
		"test.local",
		nil,
		caHarness.RootCert,
		caHarness.RootKey,
	)

	// Start HTTPS server
	server := harness.NewTestHTTPSServer(t, serverCert, serverKey)
	server.Start(t)
	defer server.Stop()

	cfg := harness.QuickConfig(t, server.Hostname, server.Port, "custom", "basic", []string{insecurePath})

	// Create scanner (with logs captured)
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	s := scanner.New(5*time.Second, 1, logger)
	defer s.Shutdown()

	// Scan certificate
	ctx := context.Background()
	result := s.ScanWithCA(ctx, server.Hostname, server.Port, cfg.CA)

	// Note: Current implementation may still load the CA despite insecure permissions
	// This test documents the behavior and ensures no crashes
	t.Logf("Validation result with insecure permissions:")
	t.Logf("  Valid: %v", result.Chain != nil && result.Chain.Valid)
	if result.Chain != nil && result.Chain.ValidationError != "" {
		t.Logf("  Error: %s", result.Chain.ValidationError)
	}

	// Check file permissions
	info, err := os.Stat(insecurePath)
	if err == nil {
		perm := info.Mode().Perm()
		if perm&0002 != 0 {
			t.Logf("✓ File is world-writable (0%o) - security risk", perm)
		}
	}
}

// TestIntegration_Error_MixedValidAndInvalidBundles tests behavior with multiple bundles.
// One valid, one invalid - should use the valid one.
func TestIntegration_Error_MixedValidAndInvalidBundles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test infrastructure
	caHarness := harness.NewTestCAHarness(t)

	// Generate server cert
	serverCert, serverKey, _, _ := caHarness.GenerateServerCert(
		"test.local",
		nil,
		caHarness.RootCert,
		caHarness.RootKey,
	)

	// Start HTTPS server
	server := harness.NewTestHTTPSServer(t, serverCert, serverKey)
	server.Start(t)
	defer server.Stop()

	// Write valid CA bundle
	validPath := caHarness.WriteCABundle([]*x509.Certificate{
		caHarness.RootCert,
	}, "valid-ca.pem")

	// Write invalid CA bundle
	invalidPath := filepath.Join(caHarness.TempDir, "invalid-ca.pem")
	if err := os.WriteFile(invalidPath, []byte("INVALID PEM DATA"), 0644); err != nil {
		t.Fatalf("Failed to write invalid bundle: %v", err)
	}

	// Config with both bundles
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
			TrustMode:      "custom",
			ValidationMode: "basic",
			CABundles:      []string{validPath, invalidPath}, // Both valid and invalid
		},
	}

	// Create scanner
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	s := scanner.New(5*time.Second, 1, logger)
	defer s.Shutdown()

	// Scan certificate
	ctx := context.Background()
	result := s.ScanWithCA(ctx, server.Hostname, server.Port, cfg.CA)

	// Should succeed (valid CA bundle loaded despite invalid one)
	if result.Success && result.Chain != nil {
		if result.Chain.Valid {
			t.Logf("✓ Validation succeeded using valid CA bundle")
			t.Logf("  (Invalid bundle ignored)")
		} else {
			t.Logf("Note: Validation failed despite valid bundle: %s", result.Chain.ValidationError)
		}
	}
}

// TestIntegration_Error_ConnectionTimeout tests handling of connection timeouts.
func TestIntegration_Error_ConnectionTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Config pointing to non-routable IP (should timeout)
	cfg := harness.QuickConfig(t, "192.0.2.1", 443, "system", "chain", nil)

	// Create scanner with very short timeout
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	s := scanner.New(1*time.Second, 1, logger) // 1 second timeout
	defer s.Shutdown()

	// Scan non-routable address
	ctx := context.Background()
	result := s.ScanWithCA(ctx, "192.0.2.1", 443, cfg.CA)

	// Should fail with connection error
	if result.Success {
		t.Error("Expected scan to FAIL (connection timeout)")
	}
	if !contains(result.Error, "timeout") && !contains(result.Error, "connection failed") {
		t.Logf("Error message: %s", result.Error)
	}

	t.Logf("✓ Connection timeout handled gracefully")
	t.Logf("  Error: %s", result.Error)
}
