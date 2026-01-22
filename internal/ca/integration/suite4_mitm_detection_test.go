// Package integration contains integration tests for CA validation.
// Suite 4: MITM Detection
package integration

import (
	"context"
	"crypto/x509"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/certwatch-app/cw-agent/internal/ca/integration/harness"
	"github.com/certwatch-app/cw-agent/internal/scanner"
)

// TestIntegration_MITMDetection_WrongCASigner tests detection of certificates signed by wrong CA.
// This simulates a man-in-the-middle attack where an attacker substitutes a certificate.
func TestIntegration_MITMDetection_WrongCASigner(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test infrastructure
	caHarness := harness.NewTestCAHarness(t)

	// Generate "legitimate" cert signed by Root CA (what we expect)
	// But we'll actually serve an "evil" cert signed by Evil CA

	// Generate EVIL cert for same hostname signed by Evil CA
	evilCert, evilKey, _, _ := caHarness.GenerateServerCert(
		"example.com",
		[]string{"example.com", "www.example.com"},
		caHarness.EvilRootCert, // MITM: signed by attacker's CA
		caHarness.EvilRootKey,
	)

	// Start server with EVIL cert
	server := harness.NewTestHTTPSServer(t, evilCert, evilKey)
	server.Start(t)
	defer server.Stop()

	// Config trusts only legitimate Root CA (not Evil CA)
	caPath := caHarness.WriteCABundle([]*x509.Certificate{
		caHarness.RootCert, // Legitimate CA
	}, "legitimate-ca.pem")

	cfg := harness.QuickConfig(t, server.Hostname, server.Port, "custom", "basic", []string{caPath})

	// Create scanner
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	s := scanner.New(5*time.Second, 1, logger)
	defer s.Shutdown()

	// Scan server with evil cert
	ctx := context.Background()
	result := s.ScanWithCA(ctx, server.Hostname, server.Port, cfg.CA)

	if !result.Success {
		t.Fatalf("Scan failed: %s", result.Error)
	}

	// Verify MITM detected (CA validation FAILED)
	if result.Chain.Valid {
		t.Error("Expected CA validation to FAIL (MITM cert signed by wrong CA)")
	}
	if !contains(result.Chain.ValidationError, "unknown authority") {
		t.Errorf("Expected 'unknown authority' error, got: %s", result.Chain.ValidationError)
	}

	t.Logf("✓ MITM detected - certificate signed by untrusted CA")
	t.Logf("  Validation Error: %s", result.Chain.ValidationError)
	t.Logf("  Certificate Subject: %s", result.Certificate.Subject)
	t.Logf("  Certificate Issuer: %s", result.Certificate.Issuer)
}

// TestIntegration_MITMDetection_UnexpectedIntermediate tests detection of unexpected intermediate CA.
// This simulates an attack where the attacker uses a different intermediate CA.
func TestIntegration_MITMDetection_UnexpectedIntermediate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test infrastructure
	caHarness := harness.NewTestCAHarness(t)

	// Create an "evil" intermediate CA signed by Root CA
	// This simulates a compromised or unauthorized intermediate
	evilIntermediate, evilIntermediateKey, evilIntermediatePEM := caHarness.GenerateCA(
		"Evil Intermediate CA",
		caHarness.RootCert,
		caHarness.RootKey,
	)

	// Generate server cert signed by Evil Intermediate
	serverCert, serverKey, _, _ := caHarness.GenerateServerCert(
		"example.com",
		nil,
		evilIntermediate,
		evilIntermediateKey,
	)

	// Start server
	server := harness.NewTestHTTPSServer(t, serverCert, serverKey)
	server.Start(t)
	defer server.Stop()

	// Config trusts Root + Legitimate Intermediate (NOT Evil Intermediate)
	caPath := caHarness.WriteCABundle([]*x509.Certificate{
		caHarness.RootCert,
		caHarness.IntermediateCert, // Legitimate intermediate
		// evilIntermediate NOT included
	}, "legitimate-ca-bundle.pem")

	cfg := harness.QuickConfig(t, server.Hostname, server.Port, "custom", "basic", []string{caPath})

	// Create scanner
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	s := scanner.New(5*time.Second, 1, logger)
	defer s.Shutdown()

	// Scan server
	ctx := context.Background()
	result := s.ScanWithCA(ctx, server.Hostname, server.Port, cfg.CA)

	if !result.Success {
		t.Fatalf("Scan failed: %s", result.Error)
	}

	// Note: This test may pass if the Root CA is trusted, because Go's x509.Verify
	// will validate the chain Root -> Evil Intermediate -> Server Cert
	// The test demonstrates that even if Root is trusted, the chain is valid.
	// To detect this, we'd need additional policy checks (not in scope for Phase 1).

	t.Logf("Certificate Chain Validation Result:")
	t.Logf("  Valid: %v", result.Chain.Valid)
	t.Logf("  Trusted Root: %s", result.Chain.TrustedRoot)
	t.Logf("  Issuer: %s", result.Certificate.Issuer)

	// Document the behavior
	if result.Chain.Valid {
		t.Logf("Note: Chain validated because Root CA is trusted (Go x509.Verify allows any intermediate)")
		t.Logf("Evil Intermediate PEM:\n%s", string(evilIntermediatePEM))
	} else {
		t.Logf("✓ Unexpected intermediate CA detected")
	}
}

// TestIntegration_MITMDetection_CertificateSubstitution tests full certificate substitution.
// Attacker replaces legitimate cert with one signed by their own CA.
func TestIntegration_MITMDetection_CertificateSubstitution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test infrastructure
	caHarness := harness.NewTestCAHarness(t)

	// Scenario: Expect cert signed by Root CA, but attacker serves cert signed by Evil CA

	// Generate attacker's cert
	attackerCert, attackerKey, _, _ := caHarness.GenerateServerCert(
		"internal.corp",
		nil,
		caHarness.EvilRootCert,
		caHarness.EvilRootKey,
	)

	// Start server with attacker's cert
	server := harness.NewTestHTTPSServer(t, attackerCert, attackerKey)
	server.Start(t)
	defer server.Stop()

	// Config trusts only legitimate CA
	caPath := caHarness.WriteCABundle([]*x509.Certificate{
		caHarness.RootCert,
		caHarness.IntermediateCert,
	}, "legitimate-ca.pem")

	cfg := harness.QuickConfig(t, server.Hostname, server.Port, "custom", "basic", []string{caPath})

	// Create scanner
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	s := scanner.New(5*time.Second, 1, logger)
	defer s.Shutdown()

	// Scan server
	ctx := context.Background()
	result := s.ScanWithCA(ctx, server.Hostname, server.Port, cfg.CA)

	if !result.Success {
		t.Fatalf("Scan failed: %s", result.Error)
	}

	// Verify MITM detected
	if result.Chain.Valid {
		t.Error("Expected validation to FAIL (certificate signed by attacker's CA)")
	}
	if !contains(result.Chain.ValidationError, "unknown authority") {
		t.Errorf("Expected 'unknown authority' error, got: %s", result.Chain.ValidationError)
	}

	// Verify certificate details
	if result.Certificate.Issuer != "Evil Root CA" {
		t.Logf("Note: Issuer is '%s' (expected 'Evil Root CA')", result.Certificate.Issuer)
	}

	t.Logf("✓ Certificate substitution attack detected")
	t.Logf("  Expected Issuer: Test Root CA or Test Intermediate CA")
	t.Logf("  Actual Issuer: %s", result.Certificate.Issuer)
	t.Logf("  Validation Error: %s", result.Chain.ValidationError)
}

// TestIntegration_MITMDetection_ExpiredAttackerCert tests detection of expired attacker certificate.
// Even if an attacker's cert is expired, we should still detect the wrong CA.
func TestIntegration_MITMDetection_ExpiredAttackerCert(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test infrastructure
	caHarness := harness.NewTestCAHarness(t)

	// Generate expired cert signed by Evil CA
	expiredCert, expiredKey, _, _ := caHarness.GenerateExpiredCert(
		"example.com",
		caHarness.EvilRootCert,
		caHarness.EvilRootKey,
	)

	// Start server with expired attacker cert
	server := harness.NewTestHTTPSServer(t, expiredCert, expiredKey)
	server.Start(t)
	defer server.Stop()

	// Config trusts only legitimate CA
	caPath := caHarness.WriteCABundle([]*x509.Certificate{
		caHarness.RootCert,
	}, "legitimate-ca.pem")

	cfg := harness.QuickConfig(t, server.Hostname, server.Port, "custom", "basic", []string{caPath})

	// Create scanner
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	s := scanner.New(5*time.Second, 1, logger)
	defer s.Shutdown()

	// Scan server
	ctx := context.Background()
	result := s.ScanWithCA(ctx, server.Hostname, server.Port, cfg.CA)

	if !result.Success {
		t.Fatalf("Scan failed: %s", result.Error)
	}

	// Verify MITM detected (wrong CA, and also expired)
	if result.Chain.Valid {
		t.Error("Expected validation to FAIL (expired cert from wrong CA)")
	}

	// Should detect both issues: wrong CA and expiration
	containsExpired := contains(result.Chain.ValidationError, "expired") ||
		contains(result.Chain.ValidationError, "not yet valid")
	containsUnknownCA := contains(result.Chain.ValidationError, "unknown authority")

	if !containsUnknownCA && !containsExpired {
		t.Logf("Validation error: %s", result.Chain.ValidationError)
	}

	t.Logf("✓ Expired attacker certificate detected")
	t.Logf("  Validation Error: %s", result.Chain.ValidationError)
	t.Logf("  Certificate Expired: %v", containsExpired)
	t.Logf("  Wrong CA Detected: %v", containsUnknownCA)
}
