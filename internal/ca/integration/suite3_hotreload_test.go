// Package integration contains integration tests for CA validation.
// Suite 3: Hot-Reload Scenarios
package integration

import (
	"context"
	"crypto/x509"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/certwatch-app/cw-agent/internal/ca/integration/harness"
	"github.com/certwatch-app/cw-agent/internal/config"
	"github.com/certwatch-app/cw-agent/internal/scanner"
)

// TestIntegration_HotReload_CABundleModified tests CA bundle hot-reload when file is modified.
// This is a key feature: no agent restart needed for CA updates.
func TestIntegration_HotReload_CABundleModified(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test infrastructure
	caHarness := harness.NewTestCAHarness(t)

	// Generate server cert signed by Root CA
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

	// Write initial CA bundle (only Intermediate, NOT Root - validation will fail)
	caPath := caHarness.WriteCABundle([]*x509.Certificate{
		caHarness.IntermediateCert,
	}, "ca-bundle.pem")

	// Enable auto-reload
	autoReload := true
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
			ValidationMode: "basic", // Use basic to avoid hostname issues
			CABundles:      []string{caPath},
			AutoReload:     &autoReload,
		},
	}

	// Create scanner
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	s := scanner.New(5*time.Second, 1, logger)
	defer s.Shutdown()

	// Build CA sources and start watching
	ctx := context.Background()
	sources, err := s.BuildCASources(ctx, cfg.CA)
	if err != nil {
		t.Fatalf("Failed to build CA sources: %v", err)
	}

	// Start watching file for changes
	for _, source := range sources {
		if err := s.StartWatching(ctx, source); err != nil {
			t.Fatalf("Failed to start watching: %v", err)
		}
	}

	// First scan - should FAIL (Root CA not in bundle)
	t.Log("Phase 1: Initial scan with incomplete CA bundle...")
	result1 := s.ScanWithCA(ctx, server.Hostname, server.Port, cfg.CA)
	if !result1.Success {
		t.Fatalf("Scan failed: %s", result1.Error)
	}
	if result1.Chain.Valid {
		t.Error("Expected validation to FAIL with incomplete CA bundle")
	}
	t.Logf("✓ Initial validation failed as expected: %s", result1.Chain.ValidationError)

	// Update CA bundle (add Root CA)
	t.Log("Phase 2: Updating CA bundle (adding Root CA)...")
	if err := os.WriteFile(caPath, append(
		caHarness.RootPEM,
		caHarness.IntermediatePEM...,
	), 0644); err != nil {
		t.Fatalf("Failed to update CA bundle: %v", err)
	}

	// Wait for file watcher debounce + cache invalidation
	t.Log("Waiting for hot-reload detection (3 seconds)...")
	time.Sleep(3 * time.Second)

	// Second scan - should SUCCEED (Root CA now trusted)
	t.Log("Phase 3: Scanning again after hot-reload...")
	result2 := s.ScanWithCA(ctx, server.Hostname, server.Port, cfg.CA)
	if !result2.Success {
		t.Fatalf("Scan failed: %s", result2.Error)
	}
	if !result2.Chain.Valid {
		t.Errorf("Expected validation to SUCCEED after hot-reload, got: %s", result2.Chain.ValidationError)
	}
	// Trusted root should be set after successful validation
	if result2.Chain.TrustedRoot == "" {
		t.Error("Expected trusted root to be set after hot-reload")
	}

	t.Logf("✓ Hot-reload succeeded - validation now passes")
	t.Logf("  Trusted Root: %s", result2.Chain.TrustedRoot)
}

// TestIntegration_HotReload_NewCAAdded tests adding a new CA to the bundle.
// Demonstrates dynamic trust expansion without restart.
func TestIntegration_HotReload_NewCAAdded(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test infrastructure
	caHarness := harness.NewTestCAHarness(t)

	// Generate two server certs with different CAs
	serverA_Cert, serverA_Key, _, _ := caHarness.GenerateServerCert(
		"server-a.test",
		nil,
		caHarness.RootCert,
		caHarness.RootKey,
	)
	serverB_Cert, serverB_Key, _, _ := caHarness.GenerateServerCert(
		"server-b.test",
		nil,
		caHarness.EvilRootCert, // Different CA
		caHarness.EvilRootKey,
	)

	// Start both servers
	serverA := harness.NewTestHTTPSServer(t, serverA_Cert, serverA_Key)
	serverA.Start(t)
	defer serverA.Stop()

	serverB := harness.NewTestHTTPSServer(t, serverB_Cert, serverB_Key)
	serverB.Start(t)
	defer serverB.Stop()

	// Initial CA bundle: only Root CA (not Evil CA)
	caPath := caHarness.WriteCABundle([]*x509.Certificate{
		caHarness.RootCert,
	}, "ca-bundle.pem")

	// Enable auto-reload
	autoReload := true
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
			ValidationMode: "basic", // Use basic to avoid hostname issues
			CABundles:      []string{caPath},
			AutoReload:     &autoReload,
		},
	}

	// Create scanner
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	s := scanner.New(5*time.Second, 1, logger)
	defer s.Shutdown()

	// Build CA sources and start watching
	ctx := context.Background()
	sources, err := s.BuildCASources(ctx, cfg.CA)
	if err != nil {
		t.Fatalf("Failed to build CA sources: %v", err)
	}
	for _, source := range sources {
		if err := s.StartWatching(ctx, source); err != nil {
			t.Fatalf("Failed to start watching: %v", err)
		}
	}

	// First scans: Server A succeeds, Server B fails
	t.Log("Phase 1: Initial scans...")
	resultA1 := s.ScanWithCA(ctx, serverA.Hostname, serverA.Port, cfg.CA)
	resultB1 := s.ScanWithCA(ctx, serverB.Hostname, serverB.Port, cfg.CA)

	if !resultA1.Chain.Valid {
		t.Errorf("Server A should validate (Root CA trusted): %s", resultA1.Chain.ValidationError)
	}
	if resultB1.Chain.Valid {
		t.Error("Server B should NOT validate (Evil CA not trusted)")
	}
	t.Log("✓ Server A: valid, Server B: invalid (as expected)")

	// Add Evil CA to bundle
	t.Log("Phase 2: Adding Evil CA to bundle...")
	if err := os.WriteFile(caPath, append(
		caHarness.RootPEM,
		caHarness.EvilRootPEM...,
	), 0644); err != nil {
		t.Fatalf("Failed to update CA bundle: %v", err)
	}

	// Wait for hot-reload
	t.Log("Waiting for hot-reload...")
	time.Sleep(3 * time.Second)

	// Second scans: both should succeed
	t.Log("Phase 3: Scanning both servers after hot-reload...")
	resultA2 := s.ScanWithCA(ctx, serverA.Hostname, serverA.Port, cfg.CA)
	resultB2 := s.ScanWithCA(ctx, serverB.Hostname, serverB.Port, cfg.CA)

	if !resultA2.Chain.Valid {
		t.Errorf("Server A validation failed after reload: %s", resultA2.Chain.ValidationError)
	}
	if !resultB2.Chain.Valid {
		t.Errorf("Server B should validate after adding Evil CA: %s", resultB2.Chain.ValidationError)
	}

	t.Log("✓ Hot-reload succeeded - both servers now validate")
}

// TestIntegration_HotReload_CARemovedFromBundle tests removing a CA from the bundle.
// Demonstrates dynamic trust revocation without restart.
func TestIntegration_HotReload_CARemovedFromBundle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test infrastructure
	caHarness := harness.NewTestCAHarness(t)

	// Generate server cert signed by Root CA (use 127.0.0.1 for local testing)
	serverCert, serverKey, _, _ := caHarness.GenerateServerCert(
		"127.0.0.1",
		[]string{"test.local"},
		caHarness.RootCert,
		caHarness.RootKey,
	)

	// Start HTTPS server
	server := harness.NewTestHTTPSServer(t, serverCert, serverKey)
	server.Start(t)
	defer server.Stop()

	// Initial CA bundle: Root CA + Evil CA (both trusted)
	caPath := caHarness.WriteCABundle([]*x509.Certificate{
		caHarness.RootCert,
		caHarness.EvilRootCert,
	}, "ca-bundle.pem")

	// Enable auto-reload
	autoReload := true
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
			CABundles:      []string{caPath},
			AutoReload:     &autoReload,
		},
	}

	// Create scanner
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	s := scanner.New(5*time.Second, 1, logger)
	defer s.Shutdown()

	// Build CA sources and start watching
	ctx := context.Background()
	sources, err := s.BuildCASources(ctx, cfg.CA)
	if err != nil {
		t.Fatalf("Failed to build CA sources: %v", err)
	}
	for _, source := range sources {
		if err := s.StartWatching(ctx, source); err != nil {
			t.Fatalf("Failed to start watching: %v", err)
		}
	}

	// First scan - should succeed (Root CA trusted)
	t.Log("Phase 1: Initial scan with Root CA trusted...")
	result1 := s.ScanWithCA(ctx, server.Hostname, server.Port, cfg.CA)
	if !result1.Chain.Valid {
		t.Errorf("Expected validation to succeed: %s", result1.Chain.ValidationError)
	}
	t.Log("✓ Initial validation succeeded")

	// Remove Root CA from bundle (keep only Evil CA)
	t.Log("Phase 2: Removing Root CA from bundle...")
	if err := os.WriteFile(caPath, caHarness.EvilRootPEM, 0644); err != nil {
		t.Fatalf("Failed to update CA bundle: %v", err)
	}

	// Wait for hot-reload
	t.Log("Waiting for hot-reload...")
	time.Sleep(3 * time.Second)

	// Second scan - should FAIL (Root CA no longer trusted)
	t.Log("Phase 3: Scanning after CA removal...")
	result2 := s.ScanWithCA(ctx, server.Hostname, server.Port, cfg.CA)
	if result2.Chain.Valid {
		t.Error("Expected validation to FAIL after removing Root CA")
	}
	if !contains(result2.Chain.ValidationError, "unknown authority") {
		t.Logf("Validation error: %s", result2.Chain.ValidationError)
	}

	t.Log("✓ Hot-reload succeeded - CA trust revoked")
}
