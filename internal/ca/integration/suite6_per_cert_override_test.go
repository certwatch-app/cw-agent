// Package integration contains integration tests for CA validation.
// Suite 6: Per-Certificate Override Scenarios
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

// TestIntegration_PerCertOverride_ValidationModes tests different validation modes per certificate.
// Demonstrates flexibility: prod (strict), staging (basic), dev (none).
func TestIntegration_PerCertOverride_ValidationModes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test infrastructure
	caHarness := harness.NewTestCAHarness(t)

	// Generate certificates for 3 environments
	prodCert, prodKey, _, _ := caHarness.GenerateServerCert(
		"prod.example.com",
		nil,
		caHarness.IntermediateCert,
		caHarness.IntermediateKey,
	)
	stagingCert, stagingKey, _, _ := caHarness.GenerateServerCert(
		"staging.example.com",
		nil,
		caHarness.IntermediateCert,
		caHarness.IntermediateKey,
	)
	devCert, devKey, _, _ := caHarness.GenerateServerCert(
		"dev.example.com",
		nil,
		caHarness.EvilRootCert, // Dev uses different CA
		caHarness.EvilRootKey,
	)

	// Start servers
	prodServer := harness.NewTestHTTPSServer(t, prodCert, prodKey)
	prodServer.Start(t)
	defer prodServer.Stop()

	stagingServer := harness.NewTestHTTPSServer(t, stagingCert, stagingKey)
	stagingServer.Start(t)
	defer stagingServer.Stop()

	devServer := harness.NewTestHTTPSServer(t, devCert, devKey)
	devServer.Start(t)
	defer devServer.Stop()

	// Write CA bundle
	caPath := caHarness.WriteCABundle([]*x509.Certificate{
		caHarness.RootCert,
		caHarness.IntermediateCert,
	}, "ca-bundle.pem")

	// Create config with global default + per-cert overrides
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
			ValidationMode: "chain", // Global default
			CABundles:      []string{caPath},
			AutoReload:     &autoReload,
		},
		Certificates: []config.CertificateConfig{
			// Prod: Uses global (chain validation)
			{
				Hostname: prodServer.Hostname,
				Port:     prodServer.Port,
				Notes:    "Production - strict validation",
			},
			// Staging: Override to basic validation
			{
				Hostname: stagingServer.Hostname,
				Port:     stagingServer.Port,
				Notes:    "Staging - basic validation",
				CA: &config.CAConfig{
					ValidationMode: "basic", // Override
				},
			},
			// Dev: Override to no validation
			{
				Hostname: devServer.Hostname,
				Port:     devServer.Port,
				Notes:    "Development - no validation",
				CA: &config.CAConfig{
					ValidationMode: "none", // Override
				},
			},
		},
	}

	// Create scanner
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	s := scanner.New(5*time.Second, 1, logger)
	defer s.Shutdown()

	ctx := context.Background()

	// Test Prod (chain validation)
	t.Run("Production_ChainValidation", func(t *testing.T) {
		result := s.ScanWithCA(ctx, prodServer.Hostname, prodServer.Port, cfg.CA)
		if !result.Success {
			t.Fatalf("Prod scan failed: %s", result.Error)
		}
		if result.Chain.ValidationMode != "chain" {
			t.Errorf("Expected validation mode 'chain' for prod, got '%s'", result.Chain.ValidationMode)
		}
		t.Logf("✓ Prod uses chain validation")
	})

	// Test Staging (basic validation)
	t.Run("Staging_BasicValidation", func(t *testing.T) {
		// Use per-cert override
		stagingCA := cfg.Certificates[1].CA
		result := s.ScanWithCA(ctx, stagingServer.Hostname, stagingServer.Port, stagingCA)
		if !result.Success {
			t.Fatalf("Staging scan failed: %s", result.Error)
		}
		if result.Chain.ValidationMode != "basic" {
			t.Errorf("Expected validation mode 'basic' for staging, got '%s'", result.Chain.ValidationMode)
		}
		t.Logf("✓ Staging uses basic validation")
	})

	// Test Dev (no validation)
	t.Run("Development_NoValidation", func(t *testing.T) {
		// Use per-cert override
		devCA := cfg.Certificates[2].CA
		result := s.ScanWithCA(ctx, devServer.Hostname, devServer.Port, devCA)
		if !result.Success {
			t.Fatalf("Dev scan failed: %s", result.Error)
		}
		if result.Chain.ValidationMode != "none" {
			t.Errorf("Expected validation mode 'none' for dev, got '%s'", result.Chain.ValidationMode)
		}
		// Dev cert is signed by Evil CA, but validation is skipped
		if !result.Chain.Valid {
			t.Logf("Note: validation_mode=none should skip validation")
		}
		t.Logf("✓ Dev uses no validation (validation skipped)")
	})
}

// TestIntegration_PerCertOverride_TrustModes tests different trust modes per certificate.
// Global uses system CAs, specific cert uses custom CA.
func TestIntegration_PerCertOverride_TrustModes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test infrastructure
	caHarness := harness.NewTestCAHarness(t)

	// Generate internal service cert (use 127.0.0.1 for local testing)
	internalCert, internalKey, _, _ := caHarness.GenerateServerCert(
		"127.0.0.1",
		[]string{"internal.corp"},
		caHarness.IntermediateCert,
		caHarness.IntermediateKey,
	)

	// Start internal server
	internalServer := harness.NewTestHTTPSServer(t, internalCert, internalKey)
	internalServer.Start(t)
	defer internalServer.Stop()

	// Write internal CA bundle
	internalCAPath := caHarness.WriteCABundle([]*x509.Certificate{
		caHarness.RootCert,
		caHarness.IntermediateCert,
	}, "internal-ca.pem")

	// Create config: global=system, internal service=custom
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
			TrustMode:      "system", // Global: trust system CAs
			ValidationMode: "chain",
		},
		Certificates: []config.CertificateConfig{
			// Public service: uses global (system CAs)
			{
				Hostname: "google.com",
				Port:     443,
				Notes:    "Public service - system CAs",
			},
			// Internal service: override to custom CA
			{
				Hostname: internalServer.Hostname,
				Port:     internalServer.Port,
				Notes:    "Internal service - custom CA",
				CA: &config.CAConfig{
					TrustMode:      "custom", // Override
					ValidationMode: "basic",
					CABundles:      []string{internalCAPath},
				},
			},
		},
	}

	// Create scanner
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	s := scanner.New(10*time.Second, 1, logger)
	defer s.Shutdown()

	ctx := context.Background()

	// Test public service (system CAs)
	t.Run("PublicService_SystemCAs", func(t *testing.T) {
		result := s.ScanWithCA(ctx, "google.com", 443, cfg.CA)
		if !result.Success {
			if contains(result.Error, "timeout") || contains(result.Error, "connection failed") {
				t.Skipf("Network unavailable: %s", result.Error)
			}
			t.Fatalf("Public scan failed: %s", result.Error)
		}
		if !result.Chain.Valid {
			t.Errorf("Public service validation failed: %s", result.Chain.ValidationError)
		}
		t.Logf("✓ Public service validates with system CAs")
		t.Logf("  Trusted Root: %s", result.Chain.TrustedRoot)
	})

	// Test internal service (custom CA)
	t.Run("InternalService_CustomCA", func(t *testing.T) {
		internalCA := cfg.Certificates[1].CA
		result := s.ScanWithCA(ctx, internalServer.Hostname, internalServer.Port, internalCA)
		if !result.Success {
			t.Fatalf("Internal scan failed: %s", result.Error)
		}
		if !result.Chain.Valid {
			t.Errorf("Internal service validation failed: %s", result.Chain.ValidationError)
		}
		// Trusted root can be either Root or Intermediate depending on chain building
		if result.Chain.TrustedRoot == "" {
			t.Error("Expected trusted root to be set")
		}
		t.Logf("✓ Internal service validates with custom CA")
		t.Logf("  Trusted Root: %s", result.Chain.TrustedRoot)
	})
}

// TestIntegration_PerCertOverride_DifferentCABundles tests per-certificate CA bundle overrides.
// Different services trust different CA bundles.
func TestIntegration_PerCertOverride_DifferentCABundles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test infrastructure
	caHarness := harness.NewTestCAHarness(t)

	// Generate certificates signed by different CAs
	serviceA_Cert, serviceA_Key, _, _ := caHarness.GenerateServerCert(
		"service-a.local",
		nil,
		caHarness.RootCert,
		caHarness.RootKey,
	)
	serviceB_Cert, serviceB_Key, _, _ := caHarness.GenerateServerCert(
		"service-b.local",
		nil,
		caHarness.EvilRootCert, // Different CA
		caHarness.EvilRootKey,
	)

	// Start servers
	serverA := harness.NewTestHTTPSServer(t, serviceA_Cert, serviceA_Key)
	serverA.Start(t)
	defer serverA.Stop()

	serverB := harness.NewTestHTTPSServer(t, serviceB_Cert, serviceB_Key)
	serverB.Start(t)
	defer serverB.Stop()

	// Write separate CA bundles
	caPathA := caHarness.WriteCABundle([]*x509.Certificate{
		caHarness.RootCert,
	}, "ca-bundle-a.pem")
	caPathB := caHarness.WriteCABundle([]*x509.Certificate{
		caHarness.EvilRootCert,
	}, "ca-bundle-b.pem")

	// Create config with different CA bundles per cert
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
			CABundles:      []string{caPathA}, // Global uses CA A
			AutoReload:     &autoReload,
		},
		Certificates: []config.CertificateConfig{
			// Service A: uses global CA bundle (CA A)
			{
				Hostname: serverA.Hostname,
				Port:     serverA.Port,
			},
			// Service B: overrides with CA bundle B
			{
				Hostname: serverB.Hostname,
				Port:     serverB.Port,
				CA: &config.CAConfig{
					TrustMode:      "custom",
					ValidationMode: "basic",
					CABundles:      []string{caPathB}, // Override with CA B
					AutoReload:     &autoReload,
				},
			},
		},
	}

	// Create scanner
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	s := scanner.New(5*time.Second, 1, logger)
	defer s.Shutdown()

	ctx := context.Background()

	// Test Service A (trusts CA A)
	t.Run("ServiceA_TrustsCAA", func(t *testing.T) {
		result := s.ScanWithCA(ctx, serverA.Hostname, serverA.Port, cfg.CA)
		if !result.Success {
			t.Fatalf("Service A scan failed: %s", result.Error)
		}
		if !result.Chain.Valid {
			t.Errorf("Service A validation failed: %s", result.Chain.ValidationError)
		}
		if result.Chain.TrustedRoot != "Test Root CA" {
			t.Errorf("Expected root 'Test Root CA', got '%s'", result.Chain.TrustedRoot)
		}
		t.Logf("✓ Service A validates with CA Bundle A")
	})

	// Test Service B (trusts CA B)
	t.Run("ServiceB_TrustsCAB", func(t *testing.T) {
		serviceB_CA := cfg.Certificates[1].CA
		result := s.ScanWithCA(ctx, serverB.Hostname, serverB.Port, serviceB_CA)
		if !result.Success {
			t.Fatalf("Service B scan failed: %s", result.Error)
		}
		if !result.Chain.Valid {
			t.Errorf("Service B validation failed: %s", result.Chain.ValidationError)
		}
		if result.Chain.TrustedRoot != "Evil Root CA" {
			t.Errorf("Expected root 'Evil Root CA', got '%s'", result.Chain.TrustedRoot)
		}
		t.Logf("✓ Service B validates with CA Bundle B")
	})
}
